package room

import (
	"bufio"
	"context"
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

	"hack2026mart/internal/game/jsonmap"
	"hack2026mart/internal/game/nav"
	"hack2026mart/internal/game/navmesh"
	"hack2026mart/internal/game/protocol"
	"hack2026mart/internal/game/render"
	"hack2026mart/internal/game/stats"
	"hack2026mart/internal/game/wad"
)

type Server struct {
	addr      string
	room      *Room
	collector stats.Collector
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
	blocked map[uint64]struct{}
	// navMesh — только для A* (FindNavPath), ленивая сборка (navMeshOnce); ходьба — nav+blocked.
	navMesh     *navmesh.Mesh
	navMeshOnce sync.Once
	players     map[string]*Player
	// killFeed — последние события для килчата (только имена).
	killFeed []protocol.KillFeedEntry
	server   *Server // ссылка на сервер для доступа к collector
	mu       sync.RWMutex
}

type Player struct {
	id     string
	name   string
	x      float64
	y      float64
	angle  float64
	hp     int
	armor  int
	dead   bool
	// ticksSinceMove: после каждого broadcast +1; при успешном шаге = 0. Большое значение = стоит на месте.
	ticksSinceMove int
	walkPhase      int // 0..7, +1 за каждый успешный шаг
	lastFireNano int64
	// lastHitConfirmNano — время последнего попадания по врагу (для клиентского «попал»).
	lastHitConfirmNano int64
	// killedBy — имя игрока, нанёсшего смертельный урон (для UI жертвы).
	killedBy string
	// Пистолет: только обойма (запас бесконечен). Автомат: магазин и запас.
	pistolMag    int
	rifleMag     int
	rifleReserve int
	money        int
	kills        int
	deaths       int
	// echoPingNano — отдать в следующем state клиенту для измерения RTT (после отправки обнуляется).
	echoPingNano int64
	// reportedPingMs — последний RTT (мс), прислан клиентом для таблицы у всех.
	reportedPingMs int
	conn         net.Conn
	send   chan []byte // готовая JSON-строка с \n; coalesce — только последний state при переполнении
	// sendFullMap: следующий state с полными walls/weapons; потом только компактные обновления.
	sendFullMap bool
}

func NewServer(addr string, roomID string, wadPath string, mapName string) (*Server, error) {
	baseRoom, err := newRoomFromWAD(roomID, wadPath, mapName)
	if err != nil {
		return nil, err
	}
	srv := &Server{
		addr: addr,
		room: baseRoom,
	}
	baseRoom.server = srv
	return srv, nil
}

// NewServerFromJSON загружает карту из JSON (каталог map/ в репозитории), а не из WAD.
func NewServerFromJSON(addr string, roomID string, jsonPath string) (*Server, error) {
	baseRoom, err := newRoomFromJSON(roomID, jsonPath)
	if err != nil {
		return nil, err
	}
	srv := &Server{
		addr: addr,
		room: baseRoom,
	}
	baseRoom.server = srv
	return srv, nil
}

// SetStatsCollector устанавливает коллектор статистики для сервера.
func (s *Server) SetStatsCollector(collector stats.Collector) {
	s.collector = collector
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
	br := bufio.NewReader(conn)

	b0, err := br.Peek(1)
	if err != nil {
		return
	}
	var pName string
	if b0[0] == protocol.MsgTypeJoin {
		if _, err := br.ReadByte(); err != nil {
			return
		}
		_, n, err := protocol.DecodeJoinAfterType(br)
		if err != nil {
			writeMsg(conn, protocol.ServerMessage{Type: "error", Error: "bad join message"})
			return
		}
		pName = strings.TrimSpace(n)
	} else {
		line, err := br.ReadBytes('\n')
		if err != nil {
			return
		}
		var hello protocol.ClientMessage
		if err := json.Unmarshal(line, &hello); err != nil || hello.Type != "join" {
			writeMsg(conn, protocol.ServerMessage{Type: "error", Error: "expected join message"})
			return
		}
		pName = strings.TrimSpace(hello.Name)
	}
	if pName == "" {
		pName = "marine"
	}

	r := s.room
	playerID := fmt.Sprintf("%s-%06d", r.id, rand.Intn(999999))
	p := &Player{
		id:          playerID,
		name:        pName,
		conn:        conn,
		send:        make(chan []byte, 8),
		sendFullMap: true,
	}

	// Record session start
	sessionStart := time.Now()
	if s.collector != nil {
		go s.collector.RecordSession(context.Background(), stats.SessionEvent{
			PlayerID:  playerID,
			RoomID:    r.id,
			MapID:     r.mapTitle,
			EventType: "start",
			Timestamp: sessionStart,
		})
	}

	r.addPlayer(p)
	defer func() {
		r.removePlayer(playerID)
		// Record session end
		if s.collector != nil {
			go s.collector.RecordSession(context.Background(), stats.SessionEvent{
				PlayerID:  playerID,
				RoomID:    r.id,
				MapID:     r.mapTitle,
				EventType: "end",
				Timestamp: time.Now(),
			})
		}
	}()

	wb := protocol.EncodeWelcome(protocol.WelcomePayload{
		PlayerID:  playerID,
		RoomID:    r.id,
		Width:     r.width,
		Height:    r.height,
		LobbyText: "WASD to move, q to quit",
	})
	if _, err := conn.Write(wb); err != nil {
		return
	}

	go p.writePump()
	r.broadcastSnapshot()

	for {
		line, err := br.ReadBytes('\n')
		if err != nil {
			return
		}
		var msg protocol.ClientMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		if msg.Type == "input" {
			r.applyInput(playerID, &msg)
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
		blocked: blocked,
		players: map[string]*Player{},
	}, nil
}

func newRoomFromJSON(roomID string, jsonPath string) (*Room, error) {
	layout, err := jsonmap.Load(jsonPath)
	if err != nil {
		return nil, err
	}
	width := layout.Width
	height := layout.Height
	weapons := []protocol.WeaponSpawn{{Name: "SHOTGUN", X: width / 2, Y: height / 2}}
	spawns := append([]protocol.GridPoint(nil), layout.Spawns...)

	// Разброс спавнов для JSON включается только явно (не через общий SPAWN_MODE=scatter из compose для WAD).
	useScatter := strings.EqualFold(strings.TrimSpace(os.Getenv("JSON_USE_SCATTER")), "1")
	if useScatter {
		spawnCount := getenvInt("SPAWN_COUNT", 10, 1, 64)
		spawnMinDist := getenvInt("SPAWN_MIN_DIST", 7, 1, 50)
		symmetry := strings.ToLower(strings.TrimSpace(os.Getenv("SPAWN_SYMMETRY")))
		if symmetry == "" {
			symmetry = "4"
		}
		seed := roomSeed(roomID)
		scattered := scatterSpawns(layout.Blocked, width, height, spawnCount, spawnMinDist, seed, symmetry)
		if len(scattered) > 0 {
			spawns = scattered
		}
	}

	if len(spawns) == 0 {
		spawns = []protocol.GridPoint{{X: width / 2, Y: height / 2}}
	}

	walls := append([]protocol.GridPoint(nil), layout.Walls...)

	log.Printf("json map loaded room=%s file=%s title=%s size=%dx%d wallCells=%d spawns=%d json_scatter=%v",
		roomID, jsonPath, layout.Title, width, height, len(walls), len(spawns), useScatter)

	return &Room{
		id:        roomID,
		width:     width,
		height:    height,
		mapTitle:  layout.Title,
		wallTex:   layout.WallTex,
		ceilFlat:  layout.Ceiling,
		floorFlat: layout.Floor,
		weapons:   weapons,
		walls:     walls,
		spawns:    spawns,
		blocked: layout.Blocked,
		players: map[string]*Player{},
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

func scatterSpawns(blocked map[uint64]struct{}, w, h, count, minDist int, seed int64, symmetry string) []protocol.GridPoint {
	openMinX, openMaxX := 1, w-2
	openMinY, openMaxY := 1, h-2
	if openMaxX < openMinX || openMaxY < openMinY {
		return nil
	}

	isOpen := func(x, y int) bool {
		if x < openMinX || x > openMaxX || y < openMinY || y > openMaxY {
			return false
		}
		_, ok := blocked[nav.CellKey(x, y)]
		return !ok
	}

	minD2 := minDist * minDist

	mirrorX := func(x int) int { return (w - 1) - x }
	mirrorY := func(y int) int { return (h - 1) - y }

	rnd := rand.New(rand.NewSource(seed))
	selected := make([]protocol.GridPoint, 0, count)
	selectedSet := map[uint64]struct{}{}

	tryAddPoint := func(x, y int) bool {
		if !isOpen(x, y) {
			return false
		}
		k := nav.CellKey(x, y)
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
		unique := make(map[uint64][2]int, len(points))
		for _, p := range points {
			x, y := p[0], p[1]
			if !isOpen(x, y) {
				return 0
			}
			unique[nav.CellKey(x, y)] = [2]int{x, y}
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

func mapWalls(md *wad.MapData, minX, minY, maxX, maxY, w, h int) ([]protocol.GridPoint, map[uint64]struct{}) {
	blocked := map[uint64]struct{}{}
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
			blocked[nav.CellKey(p.X, p.Y)] = struct{}{}
		}
	}
	walls := make([]protocol.GridPoint, 0, len(blocked))
	for k := range blocked {
		x, y := nav.CellUnpack(k)
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

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// angleTowardMapCenter — направление «вперёд» к центру карты (для дуэльных спавнов смотрят друг на друга).
func angleTowardMapCenter(px, py float64, w, h int) float64 {
	cx := float64(w) / 2.0
	cy := float64(h) / 2.0
	return math.Atan2(cy-py, cx-px)
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
	spawnX, spawnY := r.pickSpawnNearPlayersExcludingLocked("")
	p.x = spawnX
	p.y = spawnY
	p.angle = angleTowardMapCenter(spawnX, spawnY, r.width, r.height)
	p.hp = 100
	p.armor = 0
	p.dead = false
	p.lastFireNano = 0
	p.lastHitConfirmNano = 0
	p.killedBy = ""
	p.ticksSinceMove = 1000
	p.walkPhase = 0
	resetPlayerAmmo(p)
	p.money = render.StartingMoney
	r.players[p.id] = p
}

func resetPlayerAmmo(p *Player) {
	p.pistolMag = render.HUDPistolMagMax
	p.rifleMag = render.HUDRifleMagMax
	p.rifleReserve = render.HUDRifleReserveSpawn
}

func cellCenter(cx, cy int) (float64, float64) {
	return float64(cx) + 0.5, float64(cy) + 0.5
}

// pickSpawnNearPlayersExcludingLocked — рядом с другими игроками; exceptID не учитывается как «занятая» клетка (респавн).
func (r *Room) pickSpawnNearPlayersExcludingLocked(exceptID string) (float64, float64) {
	if len(r.spawns) == 0 {
		return cellCenter(r.width/2, r.height/2)
	}
	hasOther := false
	for id := range r.players {
		if id != exceptID {
			hasOther = true
			break
		}
	}
	if !hasOther {
		s := r.spawns[rand.Intn(len(r.spawns))]
		return cellCenter(s.X, s.Y)
	}
	// Несколько точек из карты (Start/End): выбираем свободную, максимально далёкую от уже стоящих игроков
	// (дуэль / test_arena_3x3 — второй игрок напротив первого).
	if len(r.spawns) >= 2 {
		bestX, bestY := -1, -1
		bestScore := -1
		for _, s := range r.spawns {
			if r.isCellOccupiedLockedExcept(s.X, s.Y, exceptID) {
				continue
			}
			minDist := int(^uint(0) >> 1)
			for id, pl := range r.players {
				if id == exceptID {
					continue
				}
				d := absInt(int(math.Floor(pl.x))-s.X) + absInt(int(math.Floor(pl.y))-s.Y)
				if d < minDist {
					minDist = d
				}
			}
			if minDist > bestScore {
				bestScore = minDist
				bestX, bestY = s.X, s.Y
			}
		}
		if bestX >= 0 {
			return cellCenter(bestX, bestY)
		}
	}
	offs := []struct{ dx, dy int }{
		{1, 0}, {-1, 0}, {0, 1}, {0, -1},
		{1, 1}, {1, -1}, {-1, 1}, {-1, -1},
		{2, 0}, {-2, 0}, {0, 2}, {0, -2},
	}
	for id, pl := range r.players {
		if id == exceptID {
			continue
		}
		for _, o := range offs {
			cx := int(math.Floor(pl.x)) + o.dx
			cy := int(math.Floor(pl.y)) + o.dy
			if cx < 1 || cx > r.width-2 || cy < 1 || cy > r.height-2 {
				continue
			}
			if _, ok := r.blocked[nav.CellKey(cx, cy)]; ok {
				continue
			}
			if r.isCellOccupiedLockedExcept(cx, cy, exceptID) {
				continue
			}
			return cellCenter(cx, cy)
		}
	}
	s := r.spawns[rand.Intn(len(r.spawns))]
	return cellCenter(s.X, s.Y)
}

func (r *Room) isCellOccupiedLockedExcept(x, y int, exceptID string) bool {
	for id, pl := range r.players {
		if id == exceptID {
			continue
		}
		if int(math.Floor(pl.x)) == x && int(math.Floor(pl.y)) == y {
			return true
		}
	}
	return false
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

func (r *Room) applyInput(playerID string, msg *protocol.ClientMessage) {
	r.mu.Lock()
	p := r.players[playerID]
	if p == nil {
		r.mu.Unlock()
		return
	}
	key := strings.ToLower(strings.TrimSpace(msg.Key))

	// Пинг обрабатываем и мёртвым (таблица / измерение RTT на экране смерти не рисуется, но соединение живо).
	if key == "ping" && msg.PingNano != 0 {
		p.echoPingNano = msg.PingNano
		if msg.PingRTTMs > 0 && msg.PingRTTMs <= 2000 {
			p.reportedPingMs = msg.PingRTTMs
		}
		r.mu.Unlock()
		return
	}

	if p.dead {
		if key == "r" {
			r.respawnPlayerLocked(p)
		}
		r.mu.Unlock()
		return
	}

	switch key {
	case "r":
		if msg.Weapon == render.HUDWeaponRifle {
			r.tryReloadRifleLocked(p)
		} else {
			r.tryReloadPistolLocked(p)
		}
	case "fire":
		r.tryFireLocked(p, msg.Weapon)
	case "w":
		stepMove(r, p, 1.0)
	case "s":
		stepMove(r, p, -1.0)
	case "a":
		p.angle -= 0.20
	case "d":
		p.angle += 0.20
	case "buy":
		r.tryPurchaseLocked(p, msg.Buy)
	}
	if p.angle < -math.Pi {
		p.angle += 2 * math.Pi
	}
	if p.angle > math.Pi {
		p.angle -= 2 * math.Pi
	}
	r.mu.Unlock()
	// State рассылает broadcastTicker; не вызывать broadcastSnapshot здесь.
}

func (r *Room) respawnPlayerLocked(p *Player) {
	if len(r.spawns) == 0 {
		return
	}
	p.x, p.y = r.pickSpawnNearPlayersExcludingLocked(p.id)
	p.angle = angleTowardMapCenter(p.x, p.y, r.width, r.height)
	p.hp = 100
	p.armor = 0
	p.dead = false
	p.killedBy = ""
	p.ticksSinceMove = 1000
	p.walkPhase = 0
	// Патроны автомата не сбрасываем: после смерти столько же, сколько было в момент смерти.
}

const fireCooldownNanos = int64(380e6)

// ~200 ms при ROOM_TICK_MS=4; при другом тике длительность «ходьбы» для анимации масштабируется.
const moveAnimTicks = 50

func (r *Room) tryReloadRifleLocked(p *Player) {
	if p.dead {
		return
	}
	if p.rifleMag >= render.HUDRifleMagMax {
		return
	}
	if p.rifleReserve <= 0 {
		return
	}
	need := render.HUDRifleMagMax - p.rifleMag
	if need > p.rifleReserve {
		need = p.rifleReserve
	}
	p.rifleMag += need
	p.rifleReserve -= need
}

func (r *Room) tryReloadPistolLocked(p *Player) {
	if p.dead {
		return
	}
	if p.pistolMag >= render.HUDPistolMagMax {
		return
	}
	p.pistolMag = render.HUDPistolMagMax
}

func (r *Room) tryFireLocked(shooter *Player, weapon int) {
	if shooter.dead {
		return
	}
	now := time.Now().UnixNano()
	if now-shooter.lastFireNano < fireCooldownNanos {
		return
	}
	isRifle := weapon == render.HUDWeaponRifle
	if isRifle {
		if shooter.rifleMag <= 0 {
			return
		}
	} else {
		if shooter.pistolMag <= 0 {
			return
		}
	}

	shooter.lastFireNano = now
	if isRifle {
		shooter.rifleMag--
	} else {
		shooter.pistolMag--
	}

	weaponType := render.HUDWeaponPistol
	if isRifle {
		weaponType = render.HUDWeaponRifle
	}

	target := r.traceHitPlayer(shooter)
	hit := target != nil

	// Record shot event
	if r.server != nil && r.server.collector != nil {
		go r.server.collector.RecordShot(context.Background(), stats.ShotEvent{
			PlayerID:   shooter.id,
			RoomID:     r.id,
			WeaponType: weaponType,
			Hit:        hit,
			Timestamp:  time.Now(),
		})
	}

	if target == nil {
		return
	}
	shooter.lastHitConfirmNano = now
	dmg := render.HUDPistolDamage
	if isRifle {
		dmg = render.HUDRifleDamage
	}
	wasAlive := !target.dead
	r.applyDamageLocked(target, dmg)
	if wasAlive && target.dead {
		target.killedBy = shooter.name
		r.pushKillFeedLocked(shooter.name, target.name)
		shooter.money += render.KillRewardMoney
		shooter.kills++
		target.deaths++

		// Record kill event
		if r.server != nil && r.server.collector != nil {
			go r.server.collector.RecordKill(context.Background(), stats.KillEvent{
				KillerID:   shooter.id,
				VictimID:   target.id,
				RoomID:     r.id,
				WeaponType: weaponType,
				Timestamp:  time.Now(),
			})
		}
	}
}

const maxKillFeedEntries = 8

func (r *Room) pushKillFeedLocked(killerName, victimName string) {
	r.killFeed = append(r.killFeed, protocol.KillFeedEntry{Killer: killerName, Victim: victimName})
	if len(r.killFeed) > maxKillFeedEntries {
		r.killFeed = r.killFeed[len(r.killFeed)-maxKillFeedEntries:]
	}
}

func (r *Room) tryPurchaseLocked(p *Player, item string) {
	if p.dead {
		return
	}
	item = strings.ToLower(strings.TrimSpace(item))
	switch item {
	case "ammo", "rifle_ammo", "rifle":
		if p.money < render.ShopAmmoPrice {
			return
		}
		if p.rifleReserve >= render.ShopMaxRifleReserve {
			return
		}
		p.money -= render.ShopAmmoPrice
		p.rifleReserve += render.ShopAmmoRounds
		if p.rifleReserve > render.ShopMaxRifleReserve {
			p.rifleReserve = render.ShopMaxRifleReserve
		}
	case "armor", "vest":
		if p.money < render.ShopArmorPrice {
			return
		}
		if p.armor >= render.ShopMaxArmor {
			return
		}
		p.money -= render.ShopArmorPrice
		p.armor += render.ShopArmorAdd
		if p.armor > render.ShopMaxArmor {
			p.armor = render.ShopMaxArmor
		}
	}
}

func (r *Room) applyDamageLocked(target *Player, dmg int) {
	if target.armor > render.ShopMaxArmor {
		target.armor = render.ShopMaxArmor
	}
	if !target.dead && dmg > 0 {
		rest := dmg
		if target.armor > 0 {
			abs := rest
			if rest > target.armor {
				abs = target.armor
			}
			target.armor -= abs
			rest -= abs
		}
		target.hp -= rest
		if target.hp <= 0 {
			target.hp = 0
			target.dead = true
		}
	}
}

// traceHitPlayer — hitscan по лучу из центра клетки стрелка. Клетка цели — AABB [px,px+1]×[py,py+1];
// пошаговый floor(x/y) пропускает клетки по диагонали, поэтому используем пересечение луча с клеткой + LOS по стенам.
func (r *Room) traceHitPlayer(shooter *Player) *Player {
	const maxDist = 40.0
	ox := shooter.x
	oy := shooter.y
	dx := math.Cos(shooter.angle)
	dy := math.Sin(shooter.angle)

	type cand struct {
		pl *Player
		t  float64
	}
	var hits []cand
	for id, pl := range r.players {
		if id == shooter.id || pl.dead {
			continue
		}
		px := int(math.Floor(pl.x))
		py := int(math.Floor(pl.y))
		t, ok := rayHitGridCell2D(ox, oy, dx, dy, px, py, maxDist)
		if !ok {
			continue
		}
		if r.rayBlockedByWalls(ox, oy, dx, dy, t) {
			continue
		}
		hits = append(hits, cand{pl: pl, t: t})
	}
	if len(hits) == 0 {
		return nil
	}
	best := hits[0]
	for i := 1; i < len(hits); i++ {
		if hits[i].t < best.t {
			best = hits[i]
		}
	}
	return best.pl
}

// rayHitGridCell2D — расстояние до входа луча O+t*D в клетку [px,px+1]×[py,py+1], t∈[0,maxDist].
func rayHitGridCell2D(ox, oy, dx, dy float64, px, py int, maxDist float64) (float64, bool) {
	minx := float64(px)
	maxx := float64(px + 1)
	miny := float64(py)
	maxy := float64(py + 1)

	const eps = 1e-9
	var tx0, tx1 float64
	if math.Abs(dx) < eps {
		if ox <= minx || ox >= maxx {
			return 0, false
		}
		tx0 = -math.MaxFloat64
		tx1 = math.MaxFloat64
	} else {
		inv := 1.0 / dx
		t1 := (minx - ox) * inv
		t2 := (maxx - ox) * inv
		tx0 = math.Min(t1, t2)
		tx1 = math.Max(t1, t2)
	}
	var ty0, ty1 float64
	if math.Abs(dy) < eps {
		if oy <= miny || oy >= maxy {
			return 0, false
		}
		ty0 = -math.MaxFloat64
		ty1 = math.MaxFloat64
	} else {
		inv := 1.0 / dy
		t1 := (miny - oy) * inv
		t2 := (maxy - oy) * inv
		ty0 = math.Min(t1, t2)
		ty1 = math.Max(t1, t2)
	}
	tEnter := math.Max(tx0, ty0)
	tExit := math.Min(tx1, ty1)
	if tExit < tEnter {
		return 0, false
	}
	var tHit float64
	switch {
	case tEnter >= 0:
		tHit = tEnter
	case tExit >= 0:
		// луч начинает внутри клетки (редко)
		tHit = 0
	default:
		return 0, false
	}
	if tHit > maxDist {
		return 0, false
	}
	return tHit, true
}

// rayBlockedByWalls — есть ли стена на отрезке (0, limitT) вдоль луча (мелкий шаг, без пропуска клеток по диагонали).
func (r *Room) rayBlockedByWalls(ox, oy, dx, dy, limitT float64) bool {
	const step = 0.06
	if limitT <= step {
		return false
	}
	for t := step; t < limitT; t += step {
		x := ox + dx*t
		y := oy + dy*t
		gx := int(math.Floor(x))
		gy := int(math.Floor(y))
		if gx < 0 || gy < 0 || gx >= r.width || gy >= r.height {
			return true
		}
		if _, ok := r.blocked[nav.CellKey(gx, gy)]; ok {
			return true
		}
	}
	return false
}

const moveStepPerTick = 0.26

func stepMove(r *Room, p *Player, dir float64) {
	nx := p.x + math.Cos(p.angle)*dir*moveStepPerTick
	ny := p.y + math.Sin(p.angle)*dir*moveStepPerTick
	tryMove(r, p, nx, ny)
}

func tryMove(r *Room, p *Player, nx, ny float64) {
	fx, fy, ok := nav.TryMoveSlide(r.blocked, r.width, r.height, p.x, p.y, nx, ny)
	if !ok {
		return
	}
	p.x = fx
	p.y = fy
	p.ticksSinceMove = 0
	p.walkPhase = (p.walkPhase + 1) % 8
}

// FindNavPath — A* по NavMesh (полигоны из той же сетки, что blocked). Меш строится один раз при первом вызове;
// при загрузке комнаты навмеш не считается — движение только через nav.TryMoveSlide(blocked).
func (r *Room) FindNavPath(ax, ay, bx, by float64) ([]navmesh.Vec2, bool) {
	r.navMeshOnce.Do(func() {
		r.navMesh = navmesh.BuildFromBlocked(r.width, r.height, r.blocked)
	})
	m := r.navMesh
	if m == nil || len(m.Polys) == 0 {
		return nil, false
	}
	return m.FindPath(ax, ay, bx, by)
}

func roomTickInterval() time.Duration {
	v := strings.TrimSpace(os.Getenv("ROOM_TICK_MS"))
	if v == "" {
		return 4 * time.Millisecond
	}
	ms, err := strconv.Atoi(v)
	if err != nil || ms < 4 || ms > 100 {
		return 4 * time.Millisecond
	}
	return time.Duration(ms) * time.Millisecond
}

// broadcastTicker шлёт state с заданным интервалом (по умолчанию 4 ms ≈250 Гц; ROOM_TICK_MS).
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

	nowNano := time.Now().UnixNano()
	pstates := make([]protocol.PlayerState, 0, len(players))
	for _, p := range players {
		fireAge := 0
		if p.lastFireNano > 0 {
			d := (nowNano - p.lastFireNano) / 1e6
			if d >= 0 && d <= 1000 {
				fireAge = int(d)
				// 0 мс «только что выстрелил» съедалось json omitempty — клиент не видел вспышку.
				if fireAge == 0 {
					fireAge = 1
				}
			}
		}
		hitAge := 0
		if p.lastHitConfirmNano > 0 {
			d := (nowNano - p.lastHitConfirmNano) / 1e6
			if d >= 0 && d <= 600 {
				hitAge = int(d)
				if hitAge == 0 {
					hitAge = 1
				}
			}
		}
		kb := ""
		if p.dead {
			kb = p.killedBy
		}
		echoPing := int64(0)
		if p.echoPingNano != 0 {
			echoPing = p.echoPingNano
			p.echoPingNano = 0
		}
		pstates = append(pstates, protocol.PlayerState{
			ID: p.id, Name: p.name, X: p.x, Y: p.y, Angle: p.angle,
			HP: p.hp, Armor: p.armor, Dead: p.dead,
			Moving:          p.ticksSinceMove < moveAnimTicks,
			WalkPhase:       p.walkPhase,
			FireAgeMs:       fireAge,
			HitConfirmAgeMs: hitAge,
			KilledBy:        kb,
			PistolMag:       p.pistolMag,
			RifleMag:        p.rifleMag,
			RifleReserve:    p.rifleReserve,
			Money:           p.money,
			Kills:           p.kills,
			Deaths:          p.deaths,
			EchoPingNano:    echoPing,
			PingMs:          p.reportedPingMs,
		})
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

	killCopy := make([]protocol.KillFeedEntry, len(r.killFeed))
	copy(killCopy, r.killFeed)
	compactSnap := protocol.RoomSnapshot{
		RoomID:      r.id,
		Width:       r.width,
		Height:      r.height,
		MapTitle:    r.mapTitle,
		WallTexture: r.wallTex,
		CeilingFlat: r.ceilFlat,
		FloorFlat:   r.floorFlat,
		Players:     pstates,
		KillFeed:    killCopy,
	}
	lineCompact, err := protocol.MarshalServerLine(&protocol.ServerMessage{Type: "state", State: &compactSnap})
	if err != nil {
		return
	}
	lineCompact = protocol.MaybeGzipServerLine(lineCompact)
	var lineFull []byte
	if needFull {
		fullSnap := compactSnap
		fullSnap.Walls = wallsCopy
		fullSnap.Weapons = weaponsCopy
		lineFull, err = protocol.MarshalServerLine(&protocol.ServerMessage{Type: "state", State: &fullSnap})
		if err != nil {
			return
		}
		lineFull = protocol.MaybeGzipServerLine(lineFull)
	}

	for _, p := range players {
		if p.sendFullMap && len(lineFull) > 0 {
			p.sendFullMap = false
			coalesceSendLine(p.send, append([]byte(nil), lineFull...))
		} else {
			coalesceSendLine(p.send, append([]byte(nil), lineCompact...))
		}
	}
	for _, p := range players {
		p.ticksSinceMove++
	}
}

func (p *Player) writePump() {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	w := bufio.NewWriterSize(p.conn, 32*1024)
	for {
		select {
		case line, ok := <-p.send:
			if !ok {
				_ = w.Flush()
				return
			}
			if _, err := w.Write(line); err != nil {
				return
			}
		drain:
			for {
				select {
				case more, ok := <-p.send:
					if !ok {
						_ = w.Flush()
						return
					}
					if _, err := w.Write(more); err != nil {
						return
					}
				default:
					if err := w.Flush(); err != nil {
						return
					}
					break drain
				}
			}
		case <-ticker.C:
			pl := pingLineBytes()
			if _, err := w.Write(pl); err != nil {
				return
			}
			if err := w.Flush(); err != nil {
				return
			}
		}
	}
}

func setTCPNoDelay(c net.Conn) {
	if tc, ok := c.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}
}

var (
	pingLineBuf []byte
	pingOnce    sync.Once
)

func pingLineBytes() []byte {
	pingOnce.Do(func() {
		var err error
		pingLineBuf, err = protocol.MarshalServerLine(&protocol.ServerMessage{Type: "ping"})
		if err != nil {
			pingLineBuf = []byte("{\"type\":\"ping\"}\n")
		}
	})
	return pingLineBuf
}

// coalesceSendLine кладёт строку в канал; при переполнении сбрасывает старый кадр — остаётся последний state.
func coalesceSendLine(ch chan []byte, line []byte) {
	select {
	case ch <- line:
	default:
		select {
		case <-ch:
		default:
		}
		select {
		case ch <- line:
		default:
		}
	}
}

func writeMsg(conn net.Conn, m protocol.ServerMessage) error {
	b, err := protocol.MarshalServerLine(&m)
	if err != nil {
		return err
	}
	_, err = conn.Write(b)
	return err
}
