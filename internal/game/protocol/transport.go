package protocol

import (
	"bufio"
	"bytes"
	"compress/flate"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
)

// Настройки читаются один раз при init (room не меняет env на лету).
var (
	stateGzipOn   bool
	stateGzipMinB int
)

func init() {
	stateGzipOn = true
	v := strings.TrimSpace(os.Getenv("ROOM_STATE_GZIP"))
	if v == "0" || strings.EqualFold(v, "false") || strings.EqualFold(v, "no") || strings.EqualFold(v, "off") {
		stateGzipOn = false
	}
	stateGzipMinB = 128
	if s := strings.TrimSpace(os.Getenv("ROOM_GZIP_MIN_BYTES")); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			if n < 32 {
				n = 32
			}
			if n > 1_000_000 {
				n = 1_000_000
			}
			stateGzipMinB = n
		}
	}
}

// LineMagicGzip — кадр: [0xC1][uint32 BE длина gzip][gzip...] без \n (gzip может содержать байт 0x0A).
const LineMagicGzip = byte(0xC1)

// maxGzipFrameBytes — защита от OOM при битом кадре.
const maxGzipFrameBytes = 8 << 20

var gzipWriterPool = sync.Pool{
	New: func() any {
		w, err := gzip.NewWriterLevel(io.Discard, flate.BestSpeed)
		if err != nil {
			return gzip.NewWriter(io.Discard)
		}
		return w
	},
}

// MarshalServerLine кодирует одну строку протокола (JSON + '\n'), без HTML-экранирования.
func MarshalServerLine(msg *ServerMessage) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(msg); err != nil {
		return nil, err
	}
	out := bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
	return append(out, '\n'), nil
}

// MaybeGzipServerLine сжимает строку сервера gzip (BestSpeed), если включено и выгодно по размеру.
// Маленькие сообщения и случаи, когда сжатие не уменьшает объём, остаются без префикса (обычный JSON).
func MaybeGzipServerLine(line []byte) []byte {
	if !stateGzipOn {
		return line
	}
	body := bytes.TrimSuffix(append([]byte(nil), line...), []byte("\n"))
	if len(body) < stateGzipMinB {
		return line
	}
	var out bytes.Buffer
	zw := gzipWriterPool.Get().(*gzip.Writer)
	zw.Reset(&out)
	if _, err := zw.Write(body); err != nil {
		gzipWriterPool.Put(zw)
		return line
	}
	if err := zw.Close(); err != nil {
		gzipWriterPool.Put(zw)
		return line
	}
	gzipWriterPool.Put(zw)
	comp := out.Bytes()
	if len(comp) >= len(body) {
		return line
	}
	wrapped := make([]byte, 0, 1+4+len(comp))
	wrapped = append(wrapped, LineMagicGzip)
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(comp)))
	wrapped = append(wrapped, lenBuf[:]...)
	wrapped = append(wrapped, comp...)
	return wrapped
}

// ReadServerMessage читает одно сообщение с room: JSON до «\n» или gzip-кадр с префиксом длины.
func ReadServerMessage(r *bufio.Reader) (ServerMessage, error) {
	peek, err := r.Peek(1)
	if err != nil {
		return ServerMessage{}, err
	}
	if peek[0] == LineMagicGzip {
		if _, err := r.ReadByte(); err != nil {
			return ServerMessage{}, err
		}
		var lenBuf [4]byte
		if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
			return ServerMessage{}, err
		}
		n := binary.BigEndian.Uint32(lenBuf[:])
		if n == 0 || uint64(n) > maxGzipFrameBytes {
			return ServerMessage{}, fmt.Errorf("bad gzip frame length: %d", n)
		}
		gzipBlob := make([]byte, n)
		if _, err := io.ReadFull(r, gzipBlob); err != nil {
			return ServerMessage{}, err
		}
		return decodeGzipJSONMessage(gzipBlob)
	}
	line, err := r.ReadBytes('\n')
	if err != nil {
		return ServerMessage{}, err
	}
	return decodePlainJSONLine(line)
}

func decodeGzipJSONMessage(gzipBlob []byte) (ServerMessage, error) {
	gr, err := gzip.NewReader(bytes.NewReader(gzipBlob))
	if err != nil {
		return ServerMessage{}, err
	}
	var plain bytes.Buffer
	_, err = io.Copy(&plain, gr)
	_ = gr.Close()
	if err != nil {
		return ServerMessage{}, err
	}
	var msg ServerMessage
	if err := json.Unmarshal(plain.Bytes(), &msg); err != nil {
		return ServerMessage{}, err
	}
	return msg, nil
}

func decodePlainJSONLine(line []byte) (ServerMessage, error) {
	line = trimLineCRLF(line)
	if len(line) == 0 {
		return ServerMessage{}, errors.New("empty server line")
	}
	var msg ServerMessage
	if err := json.Unmarshal(line, &msg); err != nil {
		return ServerMessage{}, err
	}
	return msg, nil
}

// DecodeServerLine разбирает буфер (тесты): только JSON до CRLF; gzip-кадры — через ReadServerMessage.
func DecodeServerLine(line []byte) (ServerMessage, error) {
	return decodePlainJSONLine(line)
}

func trimLineCRLF(b []byte) []byte {
	b = bytes.TrimSuffix(b, []byte("\n"))
	b = bytes.TrimSuffix(b, []byte("\r"))
	return b
}
