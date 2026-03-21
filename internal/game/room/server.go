package room

import (
	"bufio"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log"
	"math"
	"math/rand"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"hack2026mart/internal/game/protocol"
	"hack2026mart/internal/game/wad"
)

// demoPlayerID — фиктивный игрок для превью спрайта (не в таблице players).
const demoPlayerID = "__demo_marine__"

type Server struct {
	addr string
	room *Room
}

type Room struct {
	id       string
	width    int
	height   int
	mapTitle string
	wallTex  string
	ceilFlat string
	floorFlat string
	weapons  []protocol.WeaponSpawn
	walls    []protocol.GridPoint
	spawns   []protocol.GridPoint
	blocked  map[string]struct{}
	players  map[string]*Player
	mu       sync.RWMutex
}

type Player struct {
	id   string
	name string
	x    int
	y    int
	angle float64
	conn net.Conn
	send chan protocol.ServerMessage
	// sendFullMap: следующий state с полными walls/weapons; потом только компактные обновления.
	sendFullMap bool
}

func NewServer(addr string, roomID string, wadPath string, mapName string) (*Server, error) {
	baseRoom, err := newRoomFromWAD(roomID, wadPath, mapName)
	if err != nil {
		return nil, err
	}
	return &Server{
		addr: addr,
		room: baseRoom,
	}, nil
}

func (s *Server) Run() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listen room server: %w", err)
	}
	log.Printf("room server listening at %s", s.addr)
	go s.room.broadcastTicker()
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("accept error: %v", err)
			continue
		}
		setTCPNoDelay(conn)
		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)

	line, err := reader.ReadBytes('\n')
	if err != nil {
		return
	}
	var hello protocol.ClientMessage
	if err := json.Unmarshal(line, &hello); err != nil || hello.Type != "join" {
		writeMsg(conn, protocol.ServerMessage{Type: "error", Error: "expected join message"})
		return
	}

	pName := strings.TrimSpace(hello.Name)
	if pName == "" {
		pName = "marine"
	}

	r := s.room
	playerID := fmt.Sprintf("%s-%06d", r.id, rand.Intn(999999))
	p := &Player{
		id:          playerID,
		name:        pName,
		conn:        conn,
		send:        make(chan protocol.ServerMessage, 128),
		sendFullMap: true,
	}

	r.addPlayer(p)
	defer r.removePlayer(playerID)

	writeMsg(conn, protocol.ServerMessage{
		Type:     "welcome",
		PlayerID: playerID,
		RoomID:   r.id,
		Width:    r.width,
		Height:   r.height,
		LobbyText: "WASD to move, q to quit",
	})

	go p.writePump()
	r.broadcastSnapshot()

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return
		}
		var msg protocol.ClientMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		if msg.Type == "input" {
			r.applyInput(playerID, msg.Key)
		}
	}
}

func newRoomFromWAD(roomID string, wadPath string, mapName string) (*Room, error) {
	md, err := wad.LoadMap(wadPath, mapName)
	if err != nil {
		return nil, err
	}
	width := 80
	height := 28
	minX, minY, maxX, maxY := bounds(md.Vertices)
	weapons := mapWeapons(md.Things, minX, minY, maxX, maxY, width, height)
	walls, blocked := mapWalls(md, minX, minY, maxX, maxY, width, height)
	spawns := mapPlayerSpawns(md.Things, minX, minY, maxX, maxY, width, height)
	if strings.ToLower(strings.TrimSpace(os.Getenv("SPAWN_MODE"))) == "scatter" {
		spawnCount := getenvInt("SPAWN_COUNT", 10, 1, 64)
		spawnMinDist := getenvInt("SPAWN_MIN_DIST", 7, 1, 50)
		symmetry := strings.ToLower(strings.TrimSpace(os.Getenv("SPAWN_SYMMETRY")))
		if symmetry == "" {
			symmetry = "4"
		}

		seed := roomSeed(roomID)
		scattered := scatterSpawns(blocked, width, height, spawnCount, spawnMinDist, seed, symmetry)
		// If scatter can't place enough points, keep the original WAD spawns.
		if len(scattered) > 0 {
			spawns = scattered
		}
	}
	if len(weapons) == 0 {
		weapons = []protocol.WeaponSpawn{{Name: "SHOTGUN", X: width / 2, Y: height / 2}}
	}
	if len(spawns) == 0 {
		spawns = []protocol.GridPoint{{X: width / 2, Y: height / 2}}
	}
	return &Room{
		id:        roomID,
		width:     width,
		height:    height,
		mapTitle:  md.MapName,
		wallTex:   md.WallTexture,
		ceilFlat:  md.CeilingFlat,
		floorFlat: md.FloorFlat,
		weapons:   weapons,
		walls:     walls,
		spawns:    spawns,
		blocked:   blocked,
		players:   map[string]*Player{},
	}, nil
}

func bounds(v []wad.Vertex) (int, int, int, int) {
	if len(v) == 0 {
		return -1024, -1024, 1024, 1024
	}
	minX, maxX := int(v[0].X), int(v[0].X)
	minY, maxY := int(v[0].Y), int(v[0].Y)
	for _, it := range v[1:] {
		x := int(it.X)
		y := int(it.Y)
		if x < minX {
			minX = x
		}
		if x > maxX {
			maxX = x
		}
		if y < minY {
			minY = y
		}
		if y > maxY {
			maxY = y
		}
	}
	return minX, minY, maxX, maxY
}

func mapWeapons(things []wad.Thing, minX, minY, maxX, maxY, w, h int) []protocol.WeaponSpawn {
	typeNames := map[int16]string{
		2001: "SHOTGUN",
		2002: "CHAINGUN",
		2003: "ROCKET",
		2004: "PLASMA",
		2005: "CHAINSAW",
		2006: "BFG9000",
	}
	out := make([]protocol.WeaponSpawn, 0, 16)
	for _, t := range things {
		name, ok := typeNames[t.Type]
		if !ok {
			continue
		}
		out = append(out, protocol.WeaponSpawn{
			Name: name,
			X:    project(int(t.X), minX, maxX, 1, w-2),
			Y:    project(int(t.Y), minY, maxY, 1, h-2),
		})
	}
	return out
}

func mapPlayerSpawns(things []wad.Thing, minX, minY, maxX, maxY, w, h int) []protocol.GridPoint {
	out := make([]protocol.GridPoint, 0, 4)
	for _, t := range things {
		if t.Type < 1 || t.Type > 4 {
			continue
		}
		out = append(out, protocol.GridPoint{
			X: project(int(t.X), minX, maxX, 1, w-2),
			Y: project(int(t.Y), minY, maxY, 1, h-2),
		})
	}
	return out
}

func getenvInt(k string, fallback, min, max int) int {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}

func roomSeed(roomID string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(strings.TrimSpace(roomID)))
	return int64(h.Sum64())
}

func scatterSpawns(blocked map[string]struct{}, w, h, count, minDist int, seed int64, symmetry string) []protocol.GridPoint {
	openMinX, openMaxX := 1, w-2
	openMinY, openMaxY := 1, h-2
	if openMaxX < openMinX || openMaxY < openMinY {
		return nil
	}

	key := func(x, y int) string { return fmt.Sprintf("%d:%d", x, y) }
	isOpen := func(x, y int) bool {
		if x < openMinX || x > openMaxX || y < openMinY || y > openMaxY {
			return false
		}
		_, ok := blocked[key(x, y)]
		return !ok
	}

	minD2 := minDist * minDist

	mirrorX := func(x int) int { return (w - 1) - x }
	mirrorY := func(y int) int { return (h - 1) - y }

	rnd := rand.New(rand.NewSource(seed))
	selected := make([]protocol.GridPoint, 0, count)
	selectedSet := map[string]struct{}{}

	tryAddPoint := func(x, y int) bool {
		if !isOpen(x, y) {
			return false
		}
		k := key(x, y)
		if _, ok := selectedSet[k]; ok {
			return false
		}
		// Keep points apart so they look "scattered" instead of clumped.
		for _, p := range selected {
			dx := p.X - x
			dy := p.Y - y
			if dx*dx+dy*dy < minD2 {
				return false
			}
		}
		selected = append(selected, protocol.GridPoint{X: x, Y: y})
		selectedSet[k] = struct{}{}
		return true
	}

	addSymmetricGroup := func(baseX, baseY int) int {
		// Build group points based on symmetry mode.
		points := [][2]int{{baseX, baseY}}
		switch symmetry {
		case "none":
			// no extra mirrors
		case "x":
			points = append(points, [2]int{mirrorX(baseX), baseY})
		case "y":
			points = append(points, [2]int{baseX, mirrorY(baseY)})
		default:
			// 4-way mirror (vertical + horizontal)
			points = append(points, [2]int{mirrorX(baseX), baseY})
			points = append(points, [2]int{baseX, mirrorY(baseY)})
			points = append(points, [2]int{mirrorX(baseX), mirrorY(baseY)})
		}

		// Add unique points; reject the whole group if any point is blocked.
		unique := make(map[string][2]int, len(points))
		for _, p := range points {
			x, y := p[0], p[1]
			if !isOpen(x, y) {
				return 0
			}
			unique[key(x, y)] = [2]int{x, y}
		}
		added := 0
		for _, p := range unique {
			if len(selected) >= count {
				break
			}
			if tryAddPoint(p[0], p[1]) {
				added++
			}
		}
		return added
	}

	// Candidate generation.
	if strings.ToLower(symmetry) == "none" {
		candidates := make([][2]int, 0, (openMaxX-openMinX+1)*(openMaxY-openMinY+1))
		for x := openMinX; x <= openMaxX; x++ {
			for y := openMinY; y <= openMaxY; y++ {
				if isOpen(x, y) {
					candidates = append(candidates, [2]int{x, y})
				}
			}
		}
		rnd.Shuffle(len(candidates), func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })
		for _, c := range candidates {
			if len(selected) >= count {
				break
			}
			_ = tryAddPoint(c[0], c[1])
		}
	} else {
		// For symmetric modes, generate only in one quadrant to avoid duplicate base selection.
		baseMaxX := (w - 1) / 2
		baseMaxY := (h - 1) / 2
		candidates := make([][2]int, 0, 512)
		for x := openMinX; x <= baseMaxX; x++ {
			for y := openMinY; y <= baseMaxY; y++ {
				if isOpen(x, y) {
					candidates = append(candidates, [2]int{x, y})
				}
			}
		}
		rnd.Shuffle(len(candidates), func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })
		for _, c := range candidates {
			if len(selected) >= count {
				break
			}
			addSymmetricGroup(c[0], c[1])
		}
	}

	// As a fallback, if we still don't have enough, try filling remaining slots without symmetry.
	if len(selected) < count {
		candidates := make([][2]int, 0, 512)
		for x := openMinX; x <= openMaxX; x++ {
			for y := openMinY; y <= openMaxY; y++ {
				if isOpen(x, y) {
					candidates = append(candidates, [2]int{x, y})
				}
			}
		}
		rnd.Shuffle(len(candidates), func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })
		for _, c := range candidates {
			if len(selected) >= count {
				break
			}
			_ = tryAddPoint(c[0], c[1])
		}
	}

	return selected
}

func mapWalls(md *wad.MapData, minX, minY, maxX, maxY, w, h int) ([]protocol.GridPoint, map[string]struct{}) {
	blocked := map[string]struct{}{}
	for _, ln := range md.LineDefs {
		if int(ln.StartVertex) >= len(md.Vertices) || int(ln.EndVertex) >= len(md.Vertices) {
			continue
		}
		v1 := md.Vertices[int(ln.StartVertex)]
		v2 := md.Vertices[int(ln.EndVertex)]
		x1 := project(int(v1.X), minX, maxX, 1, w-2)
		y1 := project(int(v1.Y), minY, maxY, 1, h-2)
		x2 := project(int(v2.X), minX, maxX, 1, w-2)
		y2 := project(int(v2.Y), minY, maxY, 1, h-2)
		for _, p := range rasterLine(x1, y1, x2, y2) {
			blocked[key(p.X, p.Y)] = struct{}{}
		}
	}
	walls := make([]protocol.GridPoint, 0, len(blocked))
	for k := range blocked {
		var x, y int
		_, _ = fmt.Sscanf(k, "%d:%d", &x, &y)
		walls = append(walls, protocol.GridPoint{X: x, Y: y})
	}
	return walls, blocked
}

func rasterLine(x1, y1, x2, y2 int) []protocol.GridPoint {
	out := make([]protocol.GridPoint, 0, 32)
	dx := abs(x2 - x1)
	sx := -1
	if x1 < x2 {
		sx = 1
	}
	dy := -abs(y2 - y1)
	sy := -1
	if y1 < y2 {
		sy = 1
	}
	err := dx + dy
	for {
		out = append(out, protocol.GridPoint{X: x1, Y: y1})
		if x1 == x2 && y1 == y2 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x1 += sx
		}
		if e2 <= dx {
			err += dx
			y1 += sy
		}
	}
	return out
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func key(x, y int) string {
	return fmt.Sprintf("%d:%d", x, y)
}

func project(val, srcMin, srcMax, dstMin, dstMax int) int {
	if srcMax <= srcMin {
		return (dstMin + dstMax) / 2
	}
	ratio := float64(val-srcMin) / float64(srcMax-srcMin)
	out := float64(dstMin) + ratio*float64(dstMax-dstMin)
	if out < float64(dstMin) {
		return dstMin
	}
	if out > float64(dstMax) {
		return dstMax
	}
	return int(out)
}

func (r *Room) addPlayer(p *Player) {
	r.mu.Lock()
	defer r.mu.Unlock()
	spawn := r.spawns[rand.Intn(len(r.spawns))]
	spawnX, spawnY := spawn.X, spawn.Y
	p.x = spawnX
	p.y = spawnY
	p.angle = -math.Pi / 2
	r.players[p.id] = p
}

func (r *Room) removePlayer(playerID string) {
	r.mu.Lock()
	p := r.players[playerID]
	delete(r.players, playerID)
	r.mu.Unlock()
	if p != nil {
		close(p.send)
	}
	r.broadcastSnapshot()
}

func (r *Room) applyInput(playerID string, key string) {
	r.mu.Lock()
	p := r.players[playerID]
	if p == nil {
		r.mu.Unlock()
		return
	}
	switch strings.ToLower(key) {
	case "w":
		stepMove(r, p, 1.0)
	case "s":
		stepMove(r, p, -1.0)
	case "a":
		p.angle -= 0.20
	case "d":
		p.angle += 0.20
	}
	if p.angle < -math.Pi {
		p.angle += 2 * math.Pi
	}
	if p.angle > math.Pi {
		p.angle -= 2 * math.Pi
	}
	r.mu.Unlock()
	r.broadcastSnapshot()
}

func stepMove(r *Room, p *Player, dir float64) {
	nx := int(math.Round(float64(p.x) + math.Cos(p.angle)*dir))
	ny := int(math.Round(float64(p.y) + math.Sin(p.angle)*dir))
	tryMove(r, p, nx, ny)
}

func tryMove(r *Room, p *Player, nx, ny int) {
	if nx < 1 || nx > r.width-2 || ny < 1 || ny > r.height-2 {
		return
	}
	if _, ok := r.blocked[key(nx, ny)]; ok {
		return
	}
	p.x = nx
	p.y = ny
}

func roomTickInterval() time.Duration {
	v := strings.TrimSpace(os.Getenv("ROOM_TICK_MS"))
	if v == "" {
		return 16 * time.Millisecond
	}
	ms, err := strconv.Atoi(v)
	if err != nil || ms < 8 || ms > 100 {
		return 16 * time.Millisecond
	}
	return time.Duration(ms) * time.Millisecond
}

// broadcastTicker шлёт state с заданным интервалом (по умолчанию ~62 Гц).
func (r *Room) broadcastTicker() {
	ticker := time.NewTicker(roomTickInterval())
	defer ticker.Stop()
	for range ticker.C {
		r.mu.RLock()
		empty := len(r.players) == 0
		r.mu.RUnlock()
		if empty {
			continue
		}
		r.broadcastSnapshot()
	}
}

func (r *Room) broadcastSnapshot() {
	r.mu.Lock()
	defer r.mu.Unlock()

	players := make([]*Player, 0, len(r.players))
	for _, p := range r.players {
		players = append(players, p)
	}
	if len(players) == 0 {
		return
	}

	pstates := make([]protocol.PlayerState, 0, len(players)+1)
	for _, p := range players {
		pstates = append(pstates, protocol.PlayerState{
			ID: p.id, Name: p.name, X: p.x, Y: p.y, Angle: p.angle,
		})
	}
	if len(players) == 1 {
		pstates = append(pstates, r.demoMarineNear(players[0]))
	}

	needFull := false
	for _, p := range players {
		if p.sendFullMap {
			needFull = true
			break
		}
	}
	var wallsCopy []protocol.GridPoint
	var weaponsCopy []protocol.WeaponSpawn
	if needFull {
		wallsCopy = append([]protocol.GridPoint(nil), r.walls...)
		weaponsCopy = append([]protocol.WeaponSpawn(nil), r.weapons...)
	}

	for _, p := range players {
		out := protocol.RoomSnapshot{
			RoomID:      r.id,
			Width:       r.width,
			Height:      r.height,
			MapTitle:    r.mapTitle,
			WallTexture: r.wallTex,
			CeilingFlat: r.ceilFlat,
			FloorFlat:   r.floorFlat,
			Players:     pstates,
		}
		if p.sendFullMap {
			out.Walls = wallsCopy
			out.Weapons = weaponsCopy
			p.sendFullMap = false
		}
		msg := protocol.ServerMessage{Type: "state", State: &out}
		select {
		case p.send <- msg:
		default:
		}
	}
}

func (p *Player) writePump() {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case msg, ok := <-p.send:
			if !ok {
				return
			}
			if err := writeMsg(p.conn, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = writeMsg(p.conn, protocol.ServerMessage{Type: "ping"})
		}
	}
}

// demoMarineNear — несколько клеток перед игроком, лицом к нему (проверка PLAYA*).
func (r *Room) demoMarineNear(p *Player) protocol.PlayerState {
	for _, step := range []int{4, 3, 5, 2, 6, 1} {
		nx := int(math.Round(float64(p.x) + float64(step)*math.Cos(p.angle)))
		ny := int(math.Round(float64(p.y) + float64(step)*math.Sin(p.angle)))
		if nx < 1 || nx > r.width-2 || ny < 1 || ny > r.height-2 {
			continue
		}
		if nx == p.x && ny == p.y {
			continue
		}
		if _, ok := r.blocked[key(nx, ny)]; ok {
			continue
		}
		ang := math.Atan2(float64(p.y-ny), float64(p.x-nx))
		return protocol.PlayerState{
			ID:    demoPlayerID,
			Name:  "MARINE",
			X:     nx,
			Y:     ny,
			Angle: ang,
		}
	}
	x, y, ang := r.pickDemoMarineCell()
	return protocol.PlayerState{
		ID:    demoPlayerID,
		Name:  "MARINE",
		X:     x,
		Y:     y,
		Angle: ang,
	}
}

func (r *Room) pickDemoMarineCell() (int, int, float64) {
	if len(r.spawns) == 0 {
		cx := r.width / 2
		cy := r.height / 2
		return cx, cy, -math.Pi / 2
	}
	sx, sy := r.spawns[0].X, r.spawns[0].Y
	tries := []struct{ dx, dy int }{
		{0, -3}, {0, -4}, {0, -2}, {0, -1},
		{1, -2}, {-1, -2}, {2, -2}, {-2, -2},
		{2, 0}, {-2, 0}, {0, 2},
	}
	for _, t := range tries {
		nx, ny := sx+t.dx, sy+t.dy
		if nx < 1 || nx > r.width-2 || ny < 1 || ny > r.height-2 {
			continue
		}
		if _, ok := r.blocked[key(nx, ny)]; ok {
			continue
		}
		ang := math.Atan2(float64(sy-ny), float64(sx-nx))
		return nx, ny, ang
	}
	return sx, sy, -math.Pi / 2
}

func setTCPNoDelay(c net.Conn) {
	if tc, ok := c.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}
}

func writeMsg(conn net.Conn, m protocol.ServerMessage) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = conn.Write(b)
	return err
}
