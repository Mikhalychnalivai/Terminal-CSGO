// Package jsonmap загружает карты из JSON (формат doom wed/map*.json).
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

	"hack2026mart/internal/game/protocol"
)

// defaultScale — сколько клеток движка на одну клетку редактора (минимум 2, чтобы
// были и «пол», и «стена» на границе между соседними клетками).
const defaultScale = 2

// File — корень JSON-файла карты.
type File struct {
	Params  Params   `json:"Params"`
	Ceiling string   `json:"Ceiling"`
	Floor   string   `json:"Floor"`
	Walls   []Cell   `json:"Walls"`
	Sprites []Sprite `json:"Sprites"`
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
}

// Side — грань клетки (Colour NONE = нет стены).
type Side struct {
	Colour  string `json:"Colour"`
	Texture string `json:"Texture"`
}

// Cell — одна клетка сетки с четырьмя гранями.
type Cell struct {
	X, Y        int `json:"x"`
	Left, Right Side
	Up, Down    Side
}

// Sprite — объект (бочка, колонна и т.д.).
type Sprite struct {
	X, Y    float64 `json:"x"`
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
	Blocked map[string]struct{}
	Spawns  []protocol.GridPoint
}

func sideSolid(s Side) bool {
	c := strings.TrimSpace(strings.ToUpper(s.Colour))
	if c == "NONE" || c == "" {
		return false
	}
	// hex из map2: 0xFFFFFF и т.п.
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
	if err != nil || n < 2 {
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
	p := f.Params
	if p.Width < 1 || p.Height < 1 {
		return nil, fmt.Errorf("invalid Params.Width/Height")
	}

	jw, jh := p.Width, p.Height
	S := getScale()
	rw := jw*S + 2
	rh := jh*S + 2

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

	blocked := make(map[string]struct{})
	addBlock := func(x, y int) {
		if x < 1 || x > rw-2 || y < 1 || y > rh-2 {
			return
		}
		blocked[fmt.Sprintf("%d:%d", x, y)] = struct{}{}
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

	// 2) Внутренние вертикальные границы между (jx,jy) и (jx+1,jy): одна колонка на грань.
	for jy := 0; jy < jh; jy++ {
		for jx := 0; jx < jw-1; jx++ {
			a := grid[jy][jx]
			b := grid[jy][jx+1]
			if !(sideSolid(a.Right) || sideSolid(b.Left)) {
				continue
			}
			// Колонка между подсетками: правая грань левой клетки = (jx+1)*S в координатах комнаты без +1 offset
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
		var x, y int
		_, _ = fmt.Sscanf(k, "%d:%d", &x, &y)
		walls = append(walls, protocol.GridPoint{X: x, Y: y})
	}

	spawnAt := func(jx, editorY int) protocol.GridPoint {
		gy := editorToGridY(editorY, jh)
		ox := 1 + jx*S
		oy := 1 + gy*S
		return protocol.GridPoint{X: ox + S/2, Y: oy + S/2}
	}

	spawns := []protocol.GridPoint{}
	sx, sy := p.Start.X, p.Start.Y
	if sx >= 0 && sy >= 0 && sx < jw && sy < jh {
		spawns = append(spawns, spawnAt(sx, sy))
	}
	ex, ey := p.End.X, p.End.Y
	if ex >= 0 && ey >= 0 && ex < jw && ey < jh && (ex != sx || ey != sy) {
		spawns = append(spawns, spawnAt(ex, ey))
	}
	if len(spawns) == 0 {
		spawns = append(spawns, protocol.GridPoint{X: rw / 2, Y: rh / 2})
	}

	title := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
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
