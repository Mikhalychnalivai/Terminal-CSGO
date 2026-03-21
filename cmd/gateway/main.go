package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	gossh "github.com/gliderlabs/ssh"

	"hack2026mart/internal/game/protocol"
	"hack2026mart/internal/game/render"
)

func main() {
	listenAddr := getenv("SSH_LISTEN_ADDR", ":2222")
	managerAddr := getenv("ROOM_MANAGER_ADDR", "http://room-manager:8080")

	server := &gossh.Server{
		Addr: listenAddr,
		Handler: func(s gossh.Session) {
			handleSession(s, managerAddr)
		},
	}

	log.Printf("gateway ssh listening at %s", listenAddr)
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("gateway failed: %v", err)
	}
}

func handleSession(s gossh.Session, managerAddr string) {
	input := bufio.NewReader(s)
	pty, winCh, hasPty := s.Pty()
	viewW := atomic.Int32{}
	viewH := atomic.Int32{}
	if hasPty {
		viewW.Store(int32(pty.Window.Width))
		viewH.Store(int32(pty.Window.Height))
	} else {
		viewW.Store(120)
		viewH.Store(42)
	}
	io.WriteString(s, "\x1b[2J\x1b[H")
	io.WriteString(s, "DOOM SSH Arena\n")
	io.WriteString(s, "Enter your nickname: ")
	name := readLine(input)
	if name == "" {
		name = "marine"
	}
	io.WriteString(s, "\n1) Create room\n2) Join room\nSelect [1/2]: ")
	mode := readLine(input)
	if mode == "" {
		mode = "1"
	}
	io.WriteString(s, "Room ID (default: arena): ")
	roomID := readLine(input)
	if roomID == "" {
		roomID = "arena"
	}

	mapID := ""
	if mode == "1" {
		io.WriteString(s, "Карта: 1 = map1.json, 2 = map2.json, wad = DOOM.WAD [1]: ")
		mapID = strings.TrimSpace(readLine(input))
		if mapID == "" {
			mapID = "1"
		}
	}

	var roomAddr string
	var err error
	if mode == "1" {
		roomAddr, err = managerRequest(managerAddr+"/rooms/create", roomID, mapID)
	} else {
		roomAddr, err = managerRequest(managerAddr+"/rooms/get", roomID, "")
	}
	if err != nil {
		io.WriteString(s, "\n"+err.Error()+"\n")
		return
	}

	conn, err := net.Dial("tcp", roomAddr)
	if err != nil {
		io.WriteString(s, "\nRoom service unavailable.\n")
		return
	}
	defer conn.Close()
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}

	hello := protocol.ClientMessage{
		Type:   "join",
		RoomID: roomID,
		Name:   name,
	}
	if err := writeJSON(conn, hello); err != nil {
		io.WriteString(s, "\nFailed to join room.\n")
		return
	}

	reader := bufio.NewReader(conn)
	first, err := readServerMessage(reader)
	if err != nil {
		io.WriteString(s, "\nNo response from room server.\n")
		return
	}
	if first.Type == "error" {
		io.WriteString(s, "\n"+first.Error+"\n")
		return
	}
	if first.Type != "welcome" {
		io.WriteString(s, "\nUnexpected response from room server.\n")
		return
	}

	playerID := first.PlayerID
	if mode == "1" {
		io.WriteString(s, "\nRoom created and joined. Press Enter to spawn...\n")
	} else {
		io.WriteString(s, "\nConnected to room. Press Enter to spawn...\n")
	}
	_ = readLine(input)

	var (
		done      = make(chan struct{})
		closeOnce sync.Once
		fireNano  atomic.Int64 // 0 = нет анимации выстрела; время начала (UnixNano)
		gfxMu     sync.Mutex
		gfx       *render.WadGraphics
		gfxKey    string
		// Кэш карты: room шлёт walls/weapons только в первом полном state.
		cachedWalls    []protocol.GridPoint
		cachedWeapons  []protocol.WeaponSpawn
		// Для bob при ходьбе: последняя позиция и время последнего шага.
		lastPX, lastPY int = -1, -1
		lastMoveNano   int64
	)
	wadPath := getenv("WAD_PATH", "/assets/DOOM.WAD")
	stop := func() {
		closeOnce.Do(func() { close(done) })
	}

	go func() {
		for {
			msg, err := readServerMessage(reader)
			if err != nil {
				stop()
				return
			}
			if msg.Type == "state" && msg.State != nil {
				st := msg.State
				if len(st.Walls) > 0 {
					cachedWalls = append([]protocol.GridPoint(nil), st.Walls...)
					cachedWeapons = append([]protocol.WeaponSpawn(nil), st.Weapons...)
				} else if len(cachedWalls) > 0 {
					merged := *st
					merged.Walls = cachedWalls
					merged.Weapons = cachedWeapons
					st = &merged
				}
				gfxMu.Lock()
				key := wadPath + "|" + st.WallTexture + "|" + st.CeilingFlat + "|" + st.FloorFlat
				if gfx == nil || gfxKey != key {
					gfx = render.LoadWadGraphics(wadPath, st.WallTexture, st.CeilingFlat, st.FloorFlat)
					gfxKey = key
				}
				g := gfx
				gfxMu.Unlock()
				now := time.Now()
				nan := now.UnixNano()
				walking := false
				for _, p := range st.Players {
					if p.ID != playerID {
						continue
					}
					if lastPX >= 0 {
						if p.X != lastPX || p.Y != lastPY {
							lastMoveNano = nan
						}
					}
					lastPX, lastPY = p.X, p.Y
					walking = lastMoveNano > 0 && (nan-lastMoveNano) < 180_000_000
					break
				}
				hud := render.GunHUDState{
					FireStartUnixNano: fireNano.Load(),
					NowUnixNano:       nan,
					Walking:           walking,
				}
				frame := render.Frame(
					playerID,
					st,
					hud,
					int(viewW.Load()),
					int(viewH.Load()),
					g,
				)
				if _, err := io.WriteString(s, frame); err != nil {
					stop()
					return
				}
			}
		}
	}()
	if hasPty {
		go func() {
			for w := range winCh {
				if w.Width > 0 {
					viewW.Store(int32(w.Width))
				}
				if w.Height > 0 {
					viewH.Store(int32(w.Height))
				}
			}
		}()
	}

	buf := make([]byte, 1)
	for {
		select {
		case <-done:
			return
		default:
		}
		n, err := input.Read(buf)
		if err != nil || n == 0 {
			return
		}
		key := strings.ToLower(string(buf[0]))
		if key == "q" {
			return
		}
		if key == " " {
			fireNano.Store(time.Now().UnixNano())
			_ = writeJSON(conn, protocol.ClientMessage{Type: "input", Key: "fire"})
			continue
		}
		if key == "w" || key == "a" || key == "s" || key == "d" {
			_ = writeJSON(conn, protocol.ClientMessage{Type: "input", Key: key})
		}
	}
}

func readLine(r *bufio.Reader) string {
	var b strings.Builder
	for {
		ch, err := r.ReadByte()
		if err != nil {
			break
		}
		if ch == '\n' || ch == '\r' {
			break
		}
		b.WriteByte(ch)
	}
	return strings.TrimSpace(b.String())
}

func writeJSON(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "%s\n", b)
	return err
}

func readServerMessage(r *bufio.Reader) (protocol.ServerMessage, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return protocol.ServerMessage{}, err
	}
	var msg protocol.ServerMessage
	if err := json.Unmarshal(line, &msg); err != nil {
		return protocol.ServerMessage{}, err
	}
	return msg, nil
}

// mapID передаётся только при создании комнаты (JSON map1/map2 или wad).
func managerRequest(url string, roomID string, mapID string) (string, error) {
	reqBody := map[string]string{"room_id": roomID}
	if strings.TrimSpace(mapID) != "" {
		reqBody["map_id"] = strings.TrimSpace(mapID)
	}
	b, _ := json.Marshal(reqBody)
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		return "", fmt.Errorf("room-manager unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf(strings.TrimSpace(string(body)))
	}
	var out struct {
		Addr string `json:"addr"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("bad room-manager response")
	}
	if out.Addr == "" {
		return "", fmt.Errorf("room-manager returned empty room addr")
	}
	return out.Addr, nil
}

func getenv(k, fallback string) string {
	v := os.Getenv(k)
	if v == "" {
		return fallback
	}
	return v
}
