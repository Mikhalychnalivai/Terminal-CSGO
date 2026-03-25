// Package jsonmap загружает карты из JSON (исходные карты лежат в каталоге map/ репозитория).
// Логика близка к Wolfenstein 3D (1993): сетка клеток, стены между клетками и цельные «кубы» стен;
// грани Left/Right/Up/Down задают видимые стены; NONE = нет стены на этой стороне.
package jsonmap

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"hack2026mart/internal/game/nav"
	"hack2026mart/internal/game/protocol"
)

// defaultScale — клеток движка на одну клетку редактора (Wolf3D-грани; для простого редактора ставьте JSON_MAP_SCALE=1).
const defaultScale = 2

// File — корень JSON-файла карты.
type File struct {
	Params  Params   `json:"Params"`
	Ceiling string   `json:"Ceiling"`
	Floor   string   `json:"Floor"`
	Walls   []Cell   `json:"Walls"`
	Sprites []Sprite `json:"Sprites"`
}

// SpawnCell — точка спавна в координатах клетки редактора (как Start), ось Y как в JSON.
type SpawnCell struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// Params — размеры и точки старта/финиша в координатах клеток JSON (0..Width-1).
type Params struct {
	Name   string `json:"Name"`
	Width  int    `json:"Width"`
	Height int    `json:"Height"`
	Start  struct {
		X int `json:"x"`
		Y int `json:"y"`
	} `json:"Start"`
	End struct {
		X int `json:"x"`
		Y int `json:"y"`
	} `json:"End"`
	// SpawnCells — явные точки спавна (если не пусто, используются они; иначе Start/End).
	SpawnCells []SpawnCell `json:"SpawnCells,omitempty"`
	// SimpleGrid — только кубы из Walls (как булева сетка редактора), без тонких Wolf3D-стенок между клетками.
	// Иначе между стеной и полом добавляются лишние blocked-клетки — на миникарте «лишние» # относительно плана.
	SimpleGrid bool `json:"SimpleGrid,omitempty"`
}

// Side — грань клетки (Colour NONE = нет стены).
type Side struct {
	Colour  string `json:"Colour"`
	Texture string `json:"Texture"`
}

// Cell — одна клетка сетки с четырьмя гранями.
type Cell struct {
	X      int  `json:"x"`
	Y      int  `json:"y"`
	Left   Side `json:"Left"`
	Right  Side `json:"Right"`
	Up     Side `json:"Up"`
	Down   Side `json:"Down"`
}

// Sprite — объект (бочка, колонна и т.д.).
type Sprite struct {
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Texture string  `json:"Texture"`
}

// Layout — готовые данные для комнаты.
type Layout struct {
	Title   string
	Width   int
	Height  int
	WallTex string
	Ceiling string
	Floor   string
	Walls   []protocol.GridPoint
	Blocked map[uint64]struct{}
	Spawns  []protocol.GridPoint
}

func sideSolid(s Side) bool {
	c := strings.TrimSpace(strings.ToUpper(s.Colour))
	if c == "NONE" || c == "" {
		return false
	}
	// hex в JSON редакторе: 0xFFFFFF и т.п.
	if strings.HasPrefix(c, "0X") {
		return true
	}
	return true
}

func allSidesSolid(c Cell) bool {
	return sideSolid(c.Left) && sideSolid(c.Right) && sideSolid(c.Up) && sideSolid(c.Down)
}

func emptyOpenCell() Cell {
	return Cell{
		Left:  Side{Colour: "NONE"},
		Right: Side{Colour: "NONE"},
		Up:    Side{Colour: "NONE"},
		Down:  Side{Colour: "NONE"},
	}
}

func getScale() int {
	v := strings.TrimSpace(os.Getenv("JSON_MAP_SCALE"))
	if v == "" {
		return defaultScale
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return defaultScale
	}
	if n > 8 {
		return 8
	}
	return n
}

func flipY() bool {
	return strings.TrimSpace(os.Getenv("JSON_MAP_FLIP_Y")) == "1"
}

// editorToGridY переводит координату Y из JSON редактора в индекс строки сетки (0 = верх экрана).
// Если JSON_MAP_FLIP_Y=1, ось Y как в «математике» (снизу вверх) переворачивается.
func editorToGridY(editorY, jh int) int {
	if flipY() {
		return jh - 1 - editorY
	}
	return editorY
}

// Load читает JSON и строит сетку коллизий в стиле Wolf3D: полная сетка клеток,
// общие грани между соседями учитываются один раз (вертикальные и горизонтальные линии).
func Load(path string) (*Layout, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read json map: %w", err)
	}
	var f File
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("parse json map: %w", err)
	}
	return LayoutFromParsedFile(&f, path)
}

// LayoutFromParsedFile строит Layout из распарсенного File (pathHint — basename для Title, если Params.Name пуст).
func LayoutFromParsedFile(f *File, pathHint string) (*Layout, error) {
	p := f.Params
	if p.Width < 1 || p.Height < 1 {
		return nil, fmt.Errorf("invalid Params.Width/Height")
	}

	jw, jh := p.Width, p.Height
	S := getScale()
	rw := jw*S + 2
	rh := jh*S + 2

	simpleGrid := p.SimpleGrid
	if strings.TrimSpace(os.Getenv("JSON_SIMPLE_GRID")) == "1" {
		simpleGrid = true
	}

	// Полная сетка: отсутствующие клетки = пустой пол (все NONE), как открытое пространство.
	grid := make([][]Cell, jh)
	for y := 0; y < jh; y++ {
		grid[y] = make([]Cell, jw)
		for x := 0; x < jw; x++ {
			grid[y][x] = emptyOpenCell()
		}
	}
	for _, c := range f.Walls {
		if c.X < 0 || c.Y < 0 || c.X >= jw || c.Y >= jh {
			continue
		}
		gy := editorToGridY(c.Y, jh)
		grid[gy][c.X] = c
	}

	var wallTex string
	pickTex := func(t string) {
		t = strings.TrimSpace(t)
		if t != "" && wallTex == "" {
			wallTex = strings.ToUpper(t)
		}
	}
	for y := 0; y < jh; y++ {
		for x := 0; x < jw; x++ {
			c := grid[y][x]
			pickTex(c.Left.Texture)
			pickTex(c.Right.Texture)
			pickTex(c.Up.Texture)
			pickTex(c.Down.Texture)
		}
	}

	blocked := make(map[uint64]struct{})
	addBlock := func(x, y int) {
		if x < 1 || x > rw-2 || y < 1 || y > rh-2 {
			return
		}
		blocked[nav.CellKey(x, y)] = struct{}{}
	}

	// 1) Цельные «кубы» стен: все четыре грани закрыты — как блок стены в Wolf3D.
	for jy := 0; jy < jh; jy++ {
		for jx := 0; jx < jw; jx++ {
			c := grid[jy][jx]
			if !allSidesSolid(c) {
				continue
			}
			ox := 1 + jx*S
			oy := 1 + jy*S
			for ly := 0; ly < S; ly++ {
				for lx := 0; lx < S; lx++ {
					addBlock(ox+lx, oy+ly)
				}
			}
		}
	}

	// 2–3) Тонкие стенки Wolf3D между соседними клетками. Для карт из простого редактора (SimpleGrid) отключаем:
	// иначе в blocked попадают лишние клетки на границе «куб стена / пол» — на миникарте не совпадает с планом.
	if !simpleGrid {
		// 2) Внутренние вертикальные границы между (jx,jy) и (jx+1,jy): одна колонка на грань.
		for jy := 0; jy < jh; jy++ {
			for jx := 0; jx < jw-1; jx++ {
				a := grid[jy][jx]
				b := grid[jy][jx+1]
				if !(sideSolid(a.Right) || sideSolid(b.Left)) {
					continue
				}
				xWall := 1 + (jx+1)*S - 1
				oy := 1 + jy*S
				for ly := 0; ly < S; ly++ {
					addBlock(xWall, oy+ly)
				}
			}
		}

		// 3) Внутренние горизонтальные границы между (jx,jy) и (jx,jy+1): одна строка.
		for jy := 0; jy < jh-1; jy++ {
			for jx := 0; jx < jw; jx++ {
				a := grid[jy][jx]
				b := grid[jy+1][jx]
				if !(sideSolid(a.Down) || sideSolid(b.Up)) {
					continue
				}
				yWall := 1 + (jy+1)*S - 1
				ox := 1 + jx*S
				for lx := 0; lx < S; lx++ {
					addBlock(ox+lx, yWall)
				}
			}
		}
	}

	// 4) Внешний периметр карты (край сетки): только если клетка не цельной стеной уже залита.
	for jy := 0; jy < jh; jy++ {
		for jx := 0; jx < jw; jx++ {
			c := grid[jy][jx]
			if allSidesSolid(c) {
				continue
			}
			ox := 1 + jx*S
			oy := 1 + jy*S
			if jx == 0 && sideSolid(c.Left) {
				for ly := 0; ly < S; ly++ {
					addBlock(ox+0, oy+ly)
				}
			}
			if jx == jw-1 && sideSolid(c.Right) {
				for ly := 0; ly < S; ly++ {
					addBlock(ox+S-1, oy+ly)
				}
			}
			if jy == 0 && sideSolid(c.Up) {
				for lx := 0; lx < S; lx++ {
					addBlock(ox+lx, oy+0)
				}
			}
			if jy == jh-1 && sideSolid(c.Down) {
				for lx := 0; lx < S; lx++ {
					addBlock(ox+lx, oy+S-1)
				}
			}
		}
	}

	// Спрайты: препятствия (бочки, колонны).
	for _, sp := range f.Sprites {
		t := strings.ToLower(strings.TrimSpace(sp.Texture))
		if t == "" {
			continue
		}
		if !strings.Contains(t, "pillar") && !strings.Contains(t, "barrel") && !strings.Contains(t, "column") {
			continue
		}
		jx := int(math.Floor(sp.X))
		jy := editorToGridY(int(math.Floor(sp.Y)), jh)
		if jx < 0 || jy < 0 || jx >= jw || jy >= jh {
			continue
		}
		ox := 1 + jx*S
		oy := 1 + jy*S
		// центр подъячейки
		addBlock(ox+S/2, oy+S/2)
	}

	ceil := strings.TrimSpace(f.Ceiling)
	fl := strings.TrimSpace(f.Floor)
	if ceil == "" {
		ceil = "FLAT5_4"
	}
	if fl == "" {
		fl = "FLOOR5_1"
	}
	if wallTex == "" {
		wallTex = "STARTAN3"
	}

	walls := make([]protocol.GridPoint, 0, len(blocked))
	for k := range blocked {
		x, y := nav.CellUnpack(k)
		walls = append(walls, protocol.GridPoint{X: x, Y: y})
	}

	spawnAt := func(jx, editorY int) protocol.GridPoint {
		gy := editorToGridY(editorY, jh)
		ox := 1 + jx*S
		oy := 1 + gy*S
		return protocol.GridPoint{X: ox + S/2, Y: oy + S/2}
	}

	spawns := []protocol.GridPoint{}
	seenSpawn := make(map[uint64]struct{})
	tryAddSpawn := func(jx, editorY int) {
		if jx < 0 || jx >= jw || editorY < 0 || editorY >= jh {
			return
		}
		c := spawnAt(jx, editorY)
		gx, gy := c.X, c.Y
		if gx < 1 || gx > rw-2 || gy < 1 || gy > rh-2 {
			return
		}
		if _, ok := blocked[nav.CellKey(gx, gy)]; ok {
			return
		}
		k := nav.CellKey(gx, gy)
		if _, dup := seenSpawn[k]; dup {
			return
		}
		seenSpawn[k] = struct{}{}
		spawns = append(spawns, c)
	}

	if len(p.SpawnCells) > 0 {
		for _, sc := range p.SpawnCells {
			tryAddSpawn(sc.X, sc.Y)
		}
	}
	if len(spawns) == 0 {
		sx, sy := p.Start.X, p.Start.Y
		if sx >= 0 && sy >= 0 && sx < jw && sy < jh {
			tryAddSpawn(sx, sy)
		}
		ex, ey := p.End.X, p.End.Y
		if ex >= 0 && ey >= 0 && ex < jw && ey < jh && (ex != p.Start.X || ey != p.Start.Y) {
			tryAddSpawn(ex, ey)
		}
	}
	if len(spawns) == 0 {
		spawns = append(spawns, protocol.GridPoint{X: rw / 2, Y: rh / 2})
	}

	title := strings.TrimSpace(f.Params.Name)
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(pathHint), filepath.Ext(pathHint))
	}
	if title == "" {
		title = "json-map"
	}

	return &Layout{
		Title:   title,
		Width:   rw,
		Height:  rh,
		WallTex: wallTex,
		Ceiling: strings.ToUpper(ceil),
		Floor:   strings.ToUpper(fl),
		Walls:   walls,
		Blocked: blocked,
		Spawns:  spawns,
	}, nil
}

func newSolidCubeCell(x, y int, tex string) Cell {
	t := strings.TrimSpace(tex)
	if t == "" {
		t = "STARTAN3"
	}
	side := Side{Colour: "WHITE", Texture: t}
	return Cell{X: x, Y: y, Left: side, Right: side, Up: side, Down: side}
}

// BuildSimpleWallFile строит JSON-карту: периметр всегда стена; wall[y][x]==true — куб стены в координатах редактора.
// spawn — опционально: true на полу задаёт точку спавна (игроки случайно между ними). Граница wall мутируется.
func BuildSimpleWallFile(jw, jh int, wall [][]bool, spawn [][]bool, title string) (*File, error) {
	if jw < 3 || jh < 3 {
		return nil, fmt.Errorf("минимальный размер карты 3×3")
	}
	if len(wall) != jh {
		return nil, fmt.Errorf("высота wall не совпадает с jh")
	}
	for y := 0; y < jh; y++ {
		if len(wall[y]) != jw {
			return nil, fmt.Errorf("строка %d: ширина wall не совпадает с jw", y)
		}
	}
	for y := 0; y < jh; y++ {
		for x := 0; x < jw; x++ {
			if x == 0 || x == jw-1 || y == 0 || y == jh-1 {
				wall[y][x] = true
			}
		}
	}
	var spawnCells []SpawnCell
	if len(spawn) > 0 {
		for y := 0; y < jh; y++ {
			if y >= len(spawn) {
				break
			}
			for x := 0; x < jw; x++ {
				if x >= len(spawn[y]) {
					break
				}
				if wall[y][x] {
					continue
				}
				if spawn[y][x] {
					spawnCells = append(spawnCells, SpawnCell{X: x, Y: y})
				}
			}
		}
	}
	sx, sy := jw/2, jh/2
	if wall[sy][sx] {
		found := false
		for y := 1; y < jh-1; y++ {
			for x := 1; x < jw-1; x++ {
				if !wall[y][x] {
					sx, sy, found = x, y, true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("нет свободной клетки для спавна")
		}
	}
	t := strings.TrimSpace(title)
	if t == "" {
		t = "custom"
	}
	f := &File{
		Params: Params{
			Name:       t,
			Width:      jw,
			Height:     jh,
			SimpleGrid: true,
		},
		Ceiling: "FLAT5_4",
		Floor:   "FLOOR5_1",
	}
	if len(spawnCells) > 0 {
		f.Params.SpawnCells = spawnCells
		f.Params.Start.X = spawnCells[0].X
		f.Params.Start.Y = spawnCells[0].Y
		f.Params.End.X = spawnCells[0].X
		f.Params.End.Y = spawnCells[0].Y
	} else {
		f.Params.Start.X, f.Params.Start.Y = sx, sy
		f.Params.End.X, f.Params.End.Y = sx, sy
	}
	for y := 0; y < jh; y++ {
		for x := 0; x < jw; x++ {
			if wall[y][x] {
				f.Walls = append(f.Walls, newSolidCubeCell(x, y, "STARTAN3"))
			}
		}
	}
	return f, nil
}
