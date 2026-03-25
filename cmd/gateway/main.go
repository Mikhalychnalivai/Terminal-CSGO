package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	gossh "github.com/gliderlabs/ssh"
	xssh "golang.org/x/crypto/ssh"

	"hack2026mart/internal/game/nav"
	"hack2026mart/internal/game/protocol"
	"hack2026mart/internal/game/render"
	"hack2026mart/internal/mapedit"
)

const (
	predMoveStep     = 0.26
	predAngleStep    = 0.20
	predSnapDistance = 4.0
	predLerp           = 0.2
	interpBufferMs     = int64(50 * 1e6)
	maxInterpStates    = 4
)

var (
	inputLineW = []byte(`{"type":"input","key":"w"}` + "\n")
	inputLineS = []byte(`{"type":"input","key":"s"}` + "\n")
	inputLineA = []byte(`{"type":"input","key":"a"}` + "\n")
	inputLineD = []byte(`{"type":"input","key":"d"}` + "\n")
	// R без weapon — респавн (мёртвый); с weapon — перезарядка выбранного оружия.
	inputLineRRespawn     = []byte(`{"type":"input","key":"r"}` + "\n")
	inputLineReloadPistol = []byte(`{"type":"input","key":"r","weapon":1}` + "\n")
	inputLineReloadRifle  = []byte(`{"type":"input","key":"r","weapon":2}` + "\n")
	// weapon: 1 pistol, 2 rifle (render.HUDWeapon*)
	inputLineFirePistol = []byte(`{"type":"input","key":"fire","weapon":1}` + "\n")
	inputLineFireRifle  = []byte(`{"type":"input","key":"fire","weapon":2}` + "\n")
	inputLineBuyAmmo    = []byte(`{"type":"input","key":"buy","buy":"ammo"}` + "\n")
	inputLineBuyArmor   = []byte(`{"type":"input","key":"buy","buy":"armor"}` + "\n")
)

func normalizeAngleRad(a float64) float64 {
	for a > math.Pi {
		a -= 2 * math.Pi
	}
	for a < -math.Pi {
		a += 2 * math.Pi
	}
	return a
}

// predKey — тот же упакованный ключ клетки, что render.wallKey / nav.CellKey (без fmt).
func predKey(x, y int) uint64 {
	return nav.CellKey(x, y)
}

func main() {
	listenAddr := getenv("SSH_LISTEN_ADDR", ":2222")
	managerAddr := getenv("ROOM_MANAGER_ADDR", "http://room-manager:8080")

	server := &gossh.Server{
		Addr: listenAddr,
		Handler: func(s gossh.Session) {
			handleSession(s, managerAddr)
		},
		// Старые клиентские ключи ssh-rsa и rsa-sha2-*: явный список, иначе часть клиентов падает на handshake.
		ServerConfigCallback: func(_ gossh.Context) *xssh.ServerConfig {
			cfg := &xssh.ServerConfig{}
			cfg.PublicKeyAuthAlgorithms = []string{
				xssh.KeyAlgoRSA,
				xssh.KeyAlgoRSASHA256,
				xssh.KeyAlgoRSASHA512,
				xssh.KeyAlgoED25519,
				xssh.KeyAlgoECDSA256,
				xssh.KeyAlgoECDSA384,
				xssh.KeyAlgoECDSA521,
			}
			return cfg
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
	io.WriteString(s, "\x1b[2J\x1b[H\x1b[?25l")
	drawRedGradientBanner(s)
	drawNicknamePrompt(s)
	name := readLine(input, s)
	if name == "" {
		name = "marine"
	}
	io.WriteString(s, "\x1b[?25l")
	drawModeMenu(s)
	mode := readLine(input, s)
	if mode == "" {
		mode = "1"
	}
	io.WriteString(s, "\x1b[?25l")
	drawRoomIDPrompt(s)
	roomID := readLine(input, s)
	if roomID == "" {
		roomID = "arena"
	}
	io.WriteString(s, "\x1b[?25l")

	mapID := ""
	customMapJSON := ""
	if mode == "1" {
		drawMapChoicePrompt(s)
		choice := strings.TrimSpace(readLine(input, s))
		switch choice {
		case "3":
			io.WriteString(s, "\x1b[2J\x1b[H\x1b[?25h")
			drawMapEditorIntro(s)
			jsonBytes, err := mapedit.RunInteractive(input, s)
			if err != nil {
				if err == mapedit.ErrAborted {
					drawManagerError(s, "Создание карты отменено (quit).")
				} else {
					drawManagerError(s, err.Error())
				}
				return
			}
			customMapJSON = string(jsonBytes)
			mapID = "custom"
		case "2":
			drawCustomMapPrompt(s)
			mapID = strings.TrimSpace(readLine(input, s))
			if mapID == "" {
				mapID = "corridor5"
			}
		case "1", "":
			mapID = "corridor5"
		default:
			mapID = choice
		}
		io.WriteString(s, "\x1b[?25l")
	}

	var roomAddr string
	var err error
	drawConnectingBanner(s)
	runConnectingSpinner(s)
	if mode == "1" {
		roomAddr, err = managerRequest(managerAddr+"/rooms/create", roomID, mapID, customMapJSON)
	} else {
		roomAddr, err = managerRequest(managerAddr+"/rooms/get", roomID, "", "")
	}
	if err != nil {
		drawManagerError(s, err.Error())
		return
	}

	conn, err := net.Dial("tcp", roomAddr)
	if err != nil {
		drawWireError(s, "Комната недоступна (TCP)", err.Error())
		return
	}
	defer conn.Close()
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}

	br := bufio.NewReaderSize(conn, 256*1024)
	cv := &clientView{lastLocalHP: -1, lastMoney: -1}
	wadPath := getenv("WAD_PATH", "/assets/DOOM.WAD")

	if _, err := conn.Write(protocol.EncodeJoin(roomID, name)); err != nil {
		drawWireError(s, "Не удалось отправить join", err.Error())
		return
	}

	welcome, err := readClientWelcome(br)
	if err != nil {
		drawWireError(s, "Нет ответа от комнаты", err.Error())
		return
	}
	if welcome.Type == "error" {
		drawWireError(s, "Комната отклонила вход", welcome.Error)
		return
	}
	if welcome.Type != "welcome" {
		drawWireError(s, "Неожиданный ответ комнаты", welcome.Type)
		return
	}
	playerID := welcome.PlayerID

	firstState, err := protocol.ReadServerMessage(br)
	if err != nil {
		drawWireError(s, "Нет игрового state", err.Error())
		return
	}
	if firstState.Type != "state" || firstState.State == nil {
		drawWireError(s, "Ожидался первый state", firstState.Type)
		return
	}
	cv.ingestState(firstState.State, playerID, wadPath)

	drawSessionSuccess(s, mode == "1", name, roomID)
	_ = readLine(input, s)

	var (
		done      = make(chan struct{})
		closeOnce sync.Once
		fireNano        atomic.Int64
		reloadStartNano atomic.Int64
		weaponSel       atomic.Int32
	)
	weaponSel.Store(int32(render.HUDWeaponPistol))
	stop := func() {
		closeOnce.Do(func() { close(done) })
	}

	inputReady := make(chan struct{}, 1)
	var pendingMu sync.Mutex
	var pending [][]byte
	flushPending := func() {
		pendingMu.Lock()
		defer pendingMu.Unlock()
		for _, b := range pending {
			_, _ = conn.Write(b)
		}
		pending = pending[:0]
	}
	queueWrite := func(b []byte) {
		pendingMu.Lock()
		pending = append(pending, append([]byte(nil), b...))
		pendingMu.Unlock()
		select {
		case inputReady <- struct{}{}:
		default:
		}
	}
	go func() {
		tick := time.NewTicker(500 * time.Microsecond)
		defer tick.Stop()
		for {
			select {
			case <-done:
				flushPending()
				return
			case <-tick.C:
				pendingMu.Lock()
				n := len(pending)
				pendingMu.Unlock()
				if n > 0 {
					flushPending()
				}
			case <-inputReady:
				pendingMu.Lock()
				n := len(pending)
				pendingMu.Unlock()
				if n > 0 {
					flushPending()
				}
			}
		}
	}()

	// Остальные state с room (JSON строки).
	go func() {
		for {
			msg, err := protocol.ReadServerMessage(br)
			if err != nil {
				stop()
				return
			}
			if msg.Type == "state" && msg.State != nil {
				cv.ingestState(msg.State, playerID, wadPath)
			}
		}
	}()

	// Пинг: один запрос в полёте (иначе pending перезаписывался до эха — RTT не считался).
	go func() {
		tick := time.NewTicker(1100 * time.Millisecond)
		defer tick.Stop()
		stale := int64(3 * time.Second)
		for {
			select {
			case <-done:
				return
			case <-tick.C:
				now := time.Now().UnixNano()
				if cv.pendingPingNano.Load() != 0 {
					if now-cv.lastPingSendWall.Load() > stale {
						cv.pendingPingNano.Store(0)
					} else {
						continue
					}
				}
				sendNano := time.Now().UnixNano()
				cv.lastPingSendWall.Store(sendNano)
				cv.pendingPingNano.Store(sendNano)
				rtt := int(cv.pingRTTms.Load())
				if rtt > 0 {
					queueWrite([]byte(fmt.Sprintf(`{"type":"input","key":"ping","ping_nano":%d,"ping_rtt_ms":%d}`+"\n", sendNano, rtt)))
				} else {
					queueWrite([]byte(fmt.Sprintf(`{"type":"input","key":"ping","ping_nano":%d}`+"\n", sendNano)))
				}
			}
		}
	}()

	// Отрисовка: тикер GATEWAY_RENDER_MS + не чаще frameInterval (экономия трафика в SSH).
	const frameInterval = 33 * time.Millisecond
	lastFrame := time.Time{}
	go func() {
		paintOnce := func() bool {
			now := time.Now()
			if !lastFrame.IsZero() && now.Sub(lastFrame) < frameInterval {
				return true
			}
			frame := cv.paintFrame(playerID, &fireNano, &reloadStartNano, &weaponSel, viewW.Load(), viewH.Load(), now)
			if frame == "" {
				return true
			}
			if _, err := io.WriteString(s, frame); err != nil {
				stop()
				return false
			}
			lastFrame = now
			return true
		}
		if !paintOnce() {
			return
		}
		tick := time.NewTicker(gatewayRenderInterval())
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-tick.C:
				if !paintOnce() {
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

	buf := make([]byte, 64)
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
		// Tab (0x09) должен ловиться в любом месте буфера: при склейке с другими символами last не был бы '\t'.
		hasTab := false
		for i := 0; i < n; i++ {
			if buf[i] == '\t' {
				hasTab = true
				break
			}
		}
		last := buf[n-1]
		key := strings.ToLower(string(last))
		nan := time.Now().UnixNano()
		if key == "q" {
			return
		}
		if n == 1 && last == 27 { // одиночный Esc — закрыть оверлеи (не путать с \x1b[… стрелок)
			cv.scoreboardOpen.Store(false)
			cv.buyMenuOpen.Store(false)
			continue
		}
		if hasTab {
			cv.mu.Lock()
			dead := false
			if cv.snap != nil {
				for _, pl := range cv.snap.Players {
					if pl.ID == playerID {
						dead = pl.Dead
						break
					}
				}
			}
			cv.mu.Unlock()
			if !dead {
				cv.scoreboardOpen.Store(!cv.scoreboardOpen.Load())
				if cv.scoreboardOpen.Load() {
					cv.buyMenuOpen.Store(false)
				}
			}
			continue
		}
		if key == "b" {
			cv.mu.Lock()
			dead := false
			if cv.snap != nil {
				for _, pl := range cv.snap.Players {
					if pl.ID == playerID {
						dead = pl.Dead
						break
					}
				}
			}
			cv.mu.Unlock()
			if !dead {
				cv.buyMenuOpen.Store(!cv.buyMenuOpen.Load())
				if cv.buyMenuOpen.Load() {
					cv.scoreboardOpen.Store(false)
				}
			}
			continue
		}
		if cv.buyMenuOpen.Load() {
			if key == "1" {
				queueWrite(inputLineBuyAmmo)
				continue
			}
			if key == "2" {
				queueWrite(inputLineBuyArmor)
				continue
			}
			continue
		}
		// Таблица (Tab) не блокирует бой — иначе при открытом табло нельзя играть; закрыть: Tab или Esc.
		if key == " " {
			if rs := reloadStartNano.Load(); rs != 0 {
				el := nan - rs
				if weaponSel.Load() == int32(render.HUDWeaponRifle) {
					if el > 0 && el < render.ReloadAnimTotalNanos {
						continue
					}
				} else {
					if el > 0 && el < render.PistolReloadAnimTotalNanos {
						continue
					}
				}
			}
			if weaponSel.Load() == int32(render.HUDWeaponRifle) {
				if cv.localRifleMag.Load() <= 0 {
					continue
				}
				queueWrite(inputLineFireRifle)
			} else {
				if cv.localPistolMag.Load() <= 0 {
					continue
				}
				queueWrite(inputLineFirePistol)
			}
			fireNano.Store(nan)
			continue
		}
		if key == "r" {
			cv.mu.Lock()
			dead := false
			var mag, reserve, pistolMag int32
			if cv.snap != nil {
				for _, pl := range cv.snap.Players {
					if pl.ID == playerID {
						dead = pl.Dead
						mag = int32(pl.RifleMag)
						reserve = int32(pl.RifleReserve)
						pistolMag = int32(pl.PistolMag)
						break
					}
				}
			}
			cv.mu.Unlock()
			if dead {
				queueWrite(inputLineRRespawn)
			} else if weaponSel.Load() == int32(render.HUDWeaponRifle) {
				queueWrite(inputLineReloadRifle)
			} else {
				queueWrite(inputLineReloadPistol)
			}
			if !dead && weaponSel.Load() == int32(render.HUDWeaponRifle) &&
				mag < int32(render.HUDRifleMagMax) && reserve > 0 {
				reloadStartNano.Store(nan)
			}
			if !dead && weaponSel.Load() == int32(render.HUDWeaponPistol) &&
				pistolMag < int32(render.HUDPistolMagMax) {
				reloadStartNano.Store(nan)
			}
			continue
		}
		if key == "1" {
			weaponSel.Store(int32(render.HUDWeaponPistol))
			continue
		}
		if key == "2" {
			weaponSel.Store(int32(render.HUDWeaponRifle))
			continue
		}
		if key == "w" || key == "a" || key == "s" || key == "d" {
			cv.localApply(key, playerID, nan)
			switch key {
			case "w":
				queueWrite(inputLineW)
			case "s":
				queueWrite(inputLineS)
			case "a":
				queueWrite(inputLineA)
			case "d":
				queueWrite(inputLineD)
			}
		}
	}
}

// readLine читает строку до Enter; out — эхо символов (SSH-клиенты часто не показывают ввод без PTY-echo).
func readLine(r *bufio.Reader, out io.Writer) string {
	skipLeadingLineNoise(r)
	var buf []byte
	for {
		ch, err := r.ReadByte()
		if err != nil {
			break
		}
		if ch == '\n' {
			break
		}
		if ch == '\r' {
			// Enter часто только CR. Нельзя Peek без данных — bufio блокируется до следующего байта
			// (пользователь жмёт Enter второй раз). Съедаем LF только если он уже в буфере (CRLF одним пакетом).
			if r.Buffered() > 0 {
				if peek, err := r.Peek(1); err == nil && len(peek) > 0 && peek[0] == '\n' {
					_, _ = r.ReadByte()
				}
			}
			break
		}
		if ch == 8 || ch == 127 { // Backspace / Delete
			if len(buf) > 0 {
				buf = buf[:len(buf)-1]
				if out != nil {
					_, _ = io.WriteString(out, "\b \b")
				}
			}
			continue
		}
		// Эхо печатных ASCII и байт UTF-8 (>127); иначе кириллица в позывном не отображалась бы.
		if out != nil && (ch >= 32 || ch > 127) {
			_, _ = out.Write([]byte{ch})
		}
		buf = append(buf, ch)
	}
	return strings.TrimSpace(string(buf))
}

// skipLeadingLineNoise убирает «лишние» CR/LF в начале (например LF пришёл следом за CR-only Enter).
func skipLeadingLineNoise(r *bufio.Reader) {
	for r.Buffered() > 0 {
		peek, err := r.Peek(1)
		if err != nil || len(peek) == 0 {
			return
		}
		c := peek[0]
		if c != '\n' && c != '\r' {
			return
		}
		_, _ = r.ReadByte()
		if c == '\r' && r.Buffered() > 0 {
			p2, _ := r.Peek(1)
			if len(p2) > 0 && p2[0] == '\n' {
				_, _ = r.ReadByte()
			}
		}
	}
}

func writeJSON(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "%s\n", b)
	return err
}

func readClientWelcome(br *bufio.Reader) (protocol.ServerMessage, error) {
	b, err := br.Peek(1)
	if err != nil {
		return protocol.ServerMessage{}, err
	}
	if b[0] == protocol.MsgTypeWelcome {
		if _, err := br.ReadByte(); err != nil {
			return protocol.ServerMessage{}, err
		}
		wp, err := protocol.DecodeWelcomeAfterType(br)
		if err != nil {
			return protocol.ServerMessage{}, err
		}
		return protocol.ServerMessage{
			Type:      "welcome",
			PlayerID:  wp.PlayerID,
			RoomID:    wp.RoomID,
			Width:     wp.Width,
			Height:    wp.Height,
			LobbyText: wp.LobbyText,
		}, nil
	}
	line, err := br.ReadBytes('\n')
	if err != nil {
		return protocol.ServerMessage{}, err
	}
	var msg protocol.ServerMessage
	if err := json.Unmarshal(line, &msg); err != nil {
		return protocol.ServerMessage{}, err
	}
	return msg, nil
}

// mapID передаётся при создании комнаты (по умолчанию corridor5). customMapJSON — тело JSON-карты при map_id=custom.
func managerRequest(url string, roomID string, mapID string, customMapJSON string) (string, error) {
	reqBody := map[string]string{"room_id": roomID}
	if strings.TrimSpace(mapID) != "" {
		reqBody["map_id"] = strings.TrimSpace(mapID)
	}
	if strings.TrimSpace(customMapJSON) != "" {
		reqBody["map_json"] = customMapJSON
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

type interpSample struct {
	tNano      int64
	x, y, angle float64
}

// clientView — кэш снапшота и WAD; prediction (pred*) для локального игрока; интерполяция чужих позиций.
type clientView struct {
	mu sync.Mutex

	snap           *protocol.RoomSnapshot
	cachedWalls    []protocol.GridPoint
	cachedBlocked  map[uint64]struct{}
	cachedWeapons  []protocol.WeaponSpawn
	gfx            *render.WadGraphics
	gfxKey         string

	predX, predY, predAngle float64
	hasPred                 bool

	otherInterp map[string][]interpSample

	lastLocalMoveNano int64
	lastLocalTurnNano int64
	lastLocalTurnDir  int

	// lastLocalHP: -1 пока не получили первый state с нашим игроком.
	lastLocalHP          int
	damageFlashUntilNano int64
	// localRifleMag — последний rifle_mag из state (для блока SPACE без анимации при пустом магазине).
	localRifleMag atomic.Int32
	localPistolMag atomic.Int32
	buyMenuOpen       atomic.Bool
	scoreboardOpen    atomic.Bool
	pendingPingNano   atomic.Int64
	lastPingSendWall  atomic.Int64 // wall clock при отправке ping (сброс зависшего ожидания эха)
	pingRTTms         atomic.Int32
	lastStateIngestNano atomic.Int64

	// lastMoney: -1 до первого state; при росте money — вспышка «+N» над балансом.
	lastMoney              int
	moneyGainFlashUntilNano int64
	moneyGainAmount         int
}

func (cv *clientView) ingestState(st *protocol.RoomSnapshot, playerID string, wadPath string) {
	cv.mu.Lock()
	defer cv.mu.Unlock()
	cv.lastStateIngestNano.Store(time.Now().UnixNano())

	if len(st.Walls) > 0 {
		cv.cachedWalls = append([]protocol.GridPoint(nil), st.Walls...)
		cv.cachedWeapons = append([]protocol.WeaponSpawn(nil), st.Weapons...)
		cv.rebuildBlockedCache()
	}
	merged := *st
	if len(cv.cachedWalls) > 0 && len(st.Walls) == 0 {
		merged.Walls = cv.cachedWalls
		merged.Weapons = cv.cachedWeapons
		// Не подставляем cachedPlayers при пустом st.Players — иначе залипают позиции и fire_age_ms.
	}
	cv.snap = &merged

	key := wadPath + "|" + merged.WallTexture + "|" + merged.CeilingFlat + "|" + merged.FloorFlat
	if cv.gfx == nil || cv.gfxKey != key {
		cv.gfx = render.LoadWadGraphics(wadPath, merged.WallTexture, merged.CeilingFlat, merged.FloorFlat)
		cv.gfxKey = key
	}

	for _, p := range merged.Players {
		if p.ID != playerID {
			continue
		}
		if p.Dead {
			cv.buyMenuOpen.Store(false)
			cv.scoreboardOpen.Store(false)
		}
		cv.localRifleMag.Store(int32(p.RifleMag))
		cv.localPistolMag.Store(int32(p.PistolMag))
		if cv.lastLocalHP >= 0 && p.HP < cv.lastLocalHP {
			cv.damageFlashUntilNano = time.Now().UnixNano() + 350_000_000
		}
		cv.lastLocalHP = p.HP
		if cv.lastMoney >= 0 && p.Money > cv.lastMoney {
			cv.moneyGainAmount = p.Money - cv.lastMoney
			cv.moneyGainFlashUntilNano = time.Now().UnixNano() + render.MoneyGainFlashNanos
		}
		cv.lastMoney = p.Money
		if p.EchoPingNano != 0 && p.EchoPingNano == cv.pendingPingNano.Load() {
			delta := time.Now().UnixNano() - p.EchoPingNano
			if delta >= 0 && delta < 5e9 {
				cv.pingRTTms.Store(int32(delta / 1e6))
			}
			cv.pendingPingNano.Store(0)
		}
		if !cv.hasPred {
			cv.predX, cv.predY, cv.predAngle = p.X, p.Y, p.Angle
			cv.hasPred = true
			break
		}
		d := math.Hypot(p.X-cv.predX, p.Y-cv.predY)
		if d > predSnapDistance {
			cv.predX, cv.predY = p.X, p.Y
		} else {
			cv.predX += predLerp * (p.X - cv.predX)
			cv.predY += predLerp * (p.Y - cv.predY)
		}
		da := normalizeAngleRad(p.Angle - cv.predAngle)
		if math.Abs(da) > 0.5 {
			cv.predAngle = normalizeAngleRad(p.Angle)
		} else {
			cv.predAngle = normalizeAngleRad(cv.predAngle + predLerp*da)
		}
		break
	}

	nowNano := time.Now().UnixNano()
	seen := make(map[string]struct{}, len(merged.Players))
	for _, p := range merged.Players {
		if p.ID == playerID {
			continue
		}
		seen[p.ID] = struct{}{}
		cv.addOtherState(p.ID, p.X, p.Y, p.Angle, nowNano)
	}
	if cv.otherInterp != nil {
		for id := range cv.otherInterp {
			if _, ok := seen[id]; !ok {
				delete(cv.otherInterp, id)
			}
		}
	}
}

func (cv *clientView) addOtherState(id string, x, y, angle float64, tNano int64) {
	if cv.otherInterp == nil {
		cv.otherInterp = make(map[string][]interpSample)
	}
	sl := cv.otherInterp[id]
	s := interpSample{tNano: tNano, x: x, y: y, angle: angle}
	if len(sl) > 0 && sl[len(sl)-1].tNano == tNano {
		sl[len(sl)-1] = s
	} else {
		sl = append(sl, s)
	}
	if len(sl) > maxInterpStates {
		sl = sl[len(sl)-maxInterpStates:]
	}
	cv.otherInterp[id] = sl
}

func (cv *clientView) interpolateOther(id string, targetNano int64) (x, y, angle float64, ok bool) {
	sl := cv.otherInterp[id]
	if len(sl) == 0 {
		return 0, 0, 0, false
	}
	if len(sl) == 1 {
		s := sl[0]
		return s.x, s.y, s.angle, true
	}
	if targetNano <= sl[0].tNano {
		s := sl[0]
		return s.x, s.y, s.angle, true
	}
	last := sl[len(sl)-1]
	if targetNano >= last.tNano {
		return last.x, last.y, last.angle, true
	}
	for i := 0; i < len(sl)-1; i++ {
		a0, a1 := sl[i], sl[i+1]
		if a0.tNano <= targetNano && targetNano <= a1.tNano {
			span := float64(a1.tNano - a0.tNano)
			if span < 1 {
				span = 1
			}
			alpha := float64(targetNano-a0.tNano) / span
			x = a0.x + alpha*(a1.x-a0.x)
			y = a0.y + alpha*(a1.y-a0.y)
			da := normalizeAngleRad(a1.angle - a0.angle)
			angle = normalizeAngleRad(a0.angle + alpha*da)
			return x, y, angle, true
		}
	}
	return last.x, last.y, last.angle, true
}

func (cv *clientView) rebuildBlockedCache() {
	if len(cv.cachedWalls) == 0 {
		cv.cachedBlocked = nil
		return
	}
	m := make(map[uint64]struct{}, len(cv.cachedWalls))
	for _, w := range cv.cachedWalls {
		m[predKey(w.X, w.Y)] = struct{}{}
	}
	cv.cachedBlocked = m
}

func (cv *clientView) snapshotForPaintLocked(playerID string, renderNano int64) *protocol.RoomSnapshot {
	if cv.snap == nil {
		return nil
	}
	if !cv.hasPred {
		out := *cv.snap
		pls := append([]protocol.PlayerState(nil), cv.snap.Players...)
		target := renderNano - interpBufferMs
		for i := range pls {
			if pls[i].ID == playerID {
				continue
			}
			if x, y, a, ok := cv.interpolateOther(pls[i].ID, target); ok {
				pls[i].X, pls[i].Y, pls[i].Angle = x, y, a
			}
		}
		out.Players = pls
		return &out
	}
	out := *cv.snap
	pls := make([]protocol.PlayerState, len(cv.snap.Players))
	copy(pls, cv.snap.Players)
	target := renderNano - interpBufferMs
	for i := range pls {
		if pls[i].ID == playerID {
			pls[i].X = cv.predX
			pls[i].Y = cv.predY
			pls[i].Angle = cv.predAngle
			continue
		}
		if x, y, a, ok := cv.interpolateOther(pls[i].ID, target); ok {
			pls[i].X, pls[i].Y, pls[i].Angle = x, y, a
		}
	}
	out.Players = pls
	return &out
}

func (cv *clientView) tryMoveLocalLocked(nx, ny float64) bool {
	if cv.snap == nil {
		return false
	}
	w := cv.snap.Width
	h := cv.snap.Height
	walls := cv.snap.Walls
	if len(walls) == 0 {
		walls = cv.cachedWalls
	}
	blocked := cv.cachedBlocked
	if blocked == nil && len(walls) > 0 {
		blocked = make(map[uint64]struct{}, len(walls))
		for _, wp := range walls {
			blocked[predKey(wp.X, wp.Y)] = struct{}{}
		}
		cv.cachedBlocked = blocked
	}
	if blocked == nil {
		return false
	}
	fx, fy, ok := nav.TryMoveSlide(blocked, w, h, cv.predX, cv.predY, nx, ny)
	if !ok {
		return false
	}
	cv.predX, cv.predY = fx, fy
	return true
}

func (cv *clientView) localApply(key string, playerID string, nan int64) {
	cv.mu.Lock()
	defer cv.mu.Unlock()
	if cv.snap == nil {
		return
	}
	var me *protocol.PlayerState
	for i := range cv.snap.Players {
		if cv.snap.Players[i].ID == playerID {
			me = &cv.snap.Players[i]
			break
		}
	}
	if me == nil || me.Dead {
		return
	}
	if !cv.hasPred {
		cv.predX, cv.predY, cv.predAngle = me.X, me.Y, me.Angle
		cv.hasPred = true
	}
	switch key {
	case "w":
		nx := cv.predX + math.Cos(cv.predAngle)*predMoveStep
		ny := cv.predY + math.Sin(cv.predAngle)*predMoveStep
		if cv.tryMoveLocalLocked(nx, ny) {
			cv.lastLocalMoveNano = nan
		}
	case "s":
		nx := cv.predX - math.Cos(cv.predAngle)*predMoveStep
		ny := cv.predY - math.Sin(cv.predAngle)*predMoveStep
		if cv.tryMoveLocalLocked(nx, ny) {
			cv.lastLocalMoveNano = nan
		}
	case "a":
		cv.predAngle = normalizeAngleRad(cv.predAngle - predAngleStep)
		cv.lastLocalTurnNano = nan
		cv.lastLocalTurnDir = -1
	case "d":
		cv.predAngle = normalizeAngleRad(cv.predAngle + predAngleStep)
		cv.lastLocalTurnNano = nan
		cv.lastLocalTurnDir = 1
	}
}

func (cv *clientView) paintFrame(
	playerID string,
	fireNano *atomic.Int64,
	reloadStartNano *atomic.Int64,
	weaponSel *atomic.Int32,
	viewW, viewH int32,
	now time.Time,
) string {
	nan := now.UnixNano()
	rs := reloadStartNano.Load()
	if rs != 0 && nan-rs >= render.ReloadAnimTotalNanos {
		reloadStartNano.Store(0)
		rs = 0
	}
	cv.mu.Lock()
	st := cv.snapshotForPaintLocked(playerID, nan)
	g := cv.gfx
	lastMoveNano := cv.lastLocalMoveNano
	turnNano := cv.lastLocalTurnNano
	turnDir := cv.lastLocalTurnDir
	damageUntil := cv.damageFlashUntilNano
	moneyGainUntil := cv.moneyGainFlashUntilNano
	moneyGainAmt := cv.moneyGainAmount
	cv.mu.Unlock()

	if st == nil {
		return ""
	}
	lagMs := 0
	if last := cv.lastStateIngestNano.Load(); last > 0 {
		lagMs = int((nan - last) / 1e6)
		if lagMs < 0 {
			lagMs = 0
		}
		if lagMs > 999 {
			lagMs = 999
		}
	}
	walking := lastMoveNano > 0 && (nan-lastMoveNano) < 180_000_000

	hud := render.GunHUDState{
		FireStartUnixNano:   fireNano.Load(),
		ReloadStartUnixNano: rs,
		NowUnixNano:         nan,
		Walking:             walking,
		Weapon:              int(weaponSel.Load()),
		TurnLastUnixNano:    turnNano,
		TurnDir:             turnDir,
		DamageFlashUntilUnixNano: damageUntil,
		BuyMenuOpen:              cv.buyMenuOpen.Load(),
		ScoreboardOpen:           cv.scoreboardOpen.Load(),
		PingRTTMs:                int(cv.pingRTTms.Load()),
		StateLagMs:               lagMs,
		MoneyGainFlashUntilUnixNano: moneyGainUntil,
		MoneyGainAmount:             moneyGainAmt,
	}
	return render.Frame(playerID, st, hud, int(viewW), int(viewH), g)
}

func gatewayRenderInterval() time.Duration {
	v := strings.TrimSpace(os.Getenv("GATEWAY_RENDER_MS"))
	if v == "" {
		return 8 * time.Millisecond
	}
	ms, err := strconv.Atoi(v)
	if err != nil || ms < 8 || ms > 200 {
		return 8 * time.Millisecond
	}
	return time.Duration(ms) * time.Millisecond
}
