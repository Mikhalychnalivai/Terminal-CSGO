package protocol

import (
	"encoding/binary"
	"errors"
	"io"
)

const (
	MsgTypeJoin    = 0x05
	MsgTypeWelcome = 0x06
)

const (
	maxJoinStr  = 2048
	maxNameStr  = 128
	maxWelcome  = 4096
	maxLobbyStr = 512
)

// WelcomePayload — бинарный welcome после join (без JSON).
type WelcomePayload struct {
	PlayerID  string
	RoomID    string
	Width     int
	Height    int
	LobbyText string
}

// EncodeJoin — [type][u16 len room][room][u16 len name][name].
func EncodeJoin(roomID, name string) []byte {
	ri := []byte(roomID)
	ni := []byte(name)
	if len(ri) > maxJoinStr {
		ri = ri[:maxJoinStr]
	}
	if len(ni) > maxNameStr {
		ni = ni[:maxNameStr]
	}
	out := make([]byte, 1+2+len(ri)+2+len(ni))
	out[0] = MsgTypeJoin
	binary.BigEndian.PutUint16(out[1:3], uint16(len(ri)))
	copy(out[3:], ri)
	o := 3 + len(ri)
	binary.BigEndian.PutUint16(out[o:o+2], uint16(len(ni)))
	copy(out[o+2:], ni)
	return out
}

// DecodeJoin читает одно сообщение join с r (первый байт уже прочитан как MsgTypeJoin).
func DecodeJoinAfterType(r io.Reader) (roomID, name string, err error) {
	var l16 [2]byte
	if _, err := io.ReadFull(r, l16[:]); err != nil {
		return "", "", err
	}
	n := binary.BigEndian.Uint16(l16[:])
	if n == 0 || int(n) > maxJoinStr {
		return "", "", errors.New("bad join room length")
	}
	rb := make([]byte, n)
	if _, err := io.ReadFull(r, rb); err != nil {
		return "", "", err
	}
	if _, err := io.ReadFull(r, l16[:]); err != nil {
		return "", "", err
	}
	n2 := binary.BigEndian.Uint16(l16[:])
	if int(n2) > maxNameStr {
		return "", "", errors.New("bad join name length")
	}
	nb := make([]byte, n2)
	if _, err := io.ReadFull(r, nb); err != nil {
		return "", "", err
	}
	return string(rb), string(nb), nil
}

// EncodeWelcome кодирует welcome (с ведущим байтом типа).
func EncodeWelcome(w WelcomePayload) []byte {
	pid := []byte(w.PlayerID)
	rid := []byte(w.RoomID)
	lob := []byte(w.LobbyText)
	if len(pid) > maxWelcome {
		pid = pid[:maxWelcome]
	}
	if len(rid) > maxWelcome {
		rid = rid[:maxWelcome]
	}
	if len(lob) > maxLobbyStr {
		lob = lob[:maxLobbyStr]
	}
	// [type][u16 lp][pid][u16 lr][rid][u32 w][u32 h][u16 ll][lobby]
	n := 1 + 2 + len(pid) + 2 + len(rid) + 4 + 4 + 2 + len(lob)
	out := make([]byte, n)
	out[0] = MsgTypeWelcome
	binary.BigEndian.PutUint16(out[1:3], uint16(len(pid)))
	copy(out[3:], pid)
	o := 3 + len(pid)
	binary.BigEndian.PutUint16(out[o:o+2], uint16(len(rid)))
	copy(out[o+2:], rid)
	o += 2 + len(rid)
	binary.BigEndian.PutUint32(out[o:o+4], uint32(w.Width))
	binary.BigEndian.PutUint32(out[o+4:o+8], uint32(w.Height))
	o += 8
	binary.BigEndian.PutUint16(out[o:o+2], uint16(len(lob)))
	copy(out[o+2:], lob)
	return out
}

// DecodeWelcomeAfterType читает welcome после байта MsgTypeWelcome.
func DecodeWelcomeAfterType(r io.Reader) (WelcomePayload, error) {
	var w WelcomePayload
	var l16 [2]byte
	if _, err := io.ReadFull(r, l16[:]); err != nil {
		return w, err
	}
	n := binary.BigEndian.Uint16(l16[:])
	if int(n) > maxWelcome {
		return w, errors.New("bad welcome player id length")
	}
	pid := make([]byte, n)
	if _, err := io.ReadFull(r, pid); err != nil {
		return w, err
	}
	if _, err := io.ReadFull(r, l16[:]); err != nil {
		return w, err
	}
	n2 := binary.BigEndian.Uint16(l16[:])
	if int(n2) > maxWelcome {
		return w, errors.New("bad welcome room id length")
	}
	rid := make([]byte, n2)
	if _, err := io.ReadFull(r, rid); err != nil {
		return w, err
	}
	var wh [8]byte
	if _, err := io.ReadFull(r, wh[:]); err != nil {
		return w, err
	}
	w.Width = int(binary.BigEndian.Uint32(wh[:4]))
	w.Height = int(binary.BigEndian.Uint32(wh[4:]))
	if _, err := io.ReadFull(r, l16[:]); err != nil {
		return w, err
	}
	nl := binary.BigEndian.Uint16(l16[:])
	if int(nl) > maxLobbyStr {
		return w, errors.New("bad welcome lobby length")
	}
	lob := make([]byte, nl)
	if _, err := io.ReadFull(r, lob); err != nil {
		return w, err
	}
	w.PlayerID = string(pid)
	w.RoomID = string(rid)
	w.LobbyText = string(lob)
	return w, nil
}
