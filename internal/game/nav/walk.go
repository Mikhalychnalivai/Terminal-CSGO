// Package nav — проходимость и перемещение по карте (не рендер).
// Карта по-прежнему задаётся сеткой стен (как в Doom), но игрок моделируется кругом
// со скольжением вдоль стен — ближе к «не клеточному» FPS, чем проверка одной клетки floor(x,y).
// Это не Unity NavMesh (полигоны + A*), а непрерывная коллизия по тем же данным.
package nav

import (
	"math"
)

// PlayerRadius — радиус игрока в координатах клетки (~1 единица = ширина клетки). Должен быть < 0.5, чтобы пройти коридор в 1 клетку.
const PlayerRadius = 0.26

// CellKey упаковывает координаты клетки в uint64 без аллокаций (совпадает с render.wallKey / predKey).
func CellKey(gx, gy int) uint64 {
	return uint64(uint32(gx))<<32 | uint64(uint32(gy))
}

// CellUnpack — обратно к (gx, gy) для обхода map[uint64]struct{}.
func CellUnpack(k uint64) (gx, gy int) {
	return int(int32(k >> 32)), int(int32(k & 0xffffffff))
}

func clampF(x, a, b float64) float64 {
	if x < a {
		return a
	}
	if x > b {
		return b
	}
	return x
}

// circleIntersectsBlockedCell — пересечение круга с квадратом клетки [gx,gx+1)×[gy,gy+1).
func circleIntersectsBlockedCell(cx, cy, r float64, gx, gy int) bool {
	minx, miny := float64(gx), float64(gy)
	maxx, maxy := float64(gx+1), float64(gy+1)
	qx := clampF(cx, minx, maxx)
	qy := clampF(cy, miny, maxy)
	dx := cx - qx
	dy := cy - qy
	return dx*dx+dy*dy < r*r
}

// CircleWalkable — центр (cx,cy) допустим: круг не заходит в стены и не выходит за внутреннюю область карты [1,w-1) по обеим осям (как граница проходимой зоны).
func CircleWalkable(blocked map[uint64]struct{}, w, h int, cx, cy float64) bool {
	r := PlayerRadius
	// Жёсткая рамка: «пол» между линиями x=1 и x=w-1 (границы клеток 1..w-2).
	if cx-r < 1.0 || cy-r < 1.0 || cx+r > float64(w-1) || cy+r > float64(h-1) {
		return false
	}
	minGX := int(math.Floor(cx - r))
	maxGX := int(math.Floor(cx + r))
	minGY := int(math.Floor(cy - r))
	maxGY := int(math.Floor(cy + r))
	for gx := minGX; gx <= maxGX; gx++ {
		for gy := minGY; gy <= maxGY; gy++ {
			if _, ok := blocked[CellKey(gx, gy)]; !ok {
				continue
			}
			if circleIntersectsBlockedCell(cx, cy, r, gx, gy) {
				return false
			}
		}
	}
	return true
}

// TryMoveSlide — сначала диагональ, затем скольжение по осям (классический slide для FPS).
func TryMoveSlide(blocked map[uint64]struct{}, w, h int, ox, oy, nx, ny float64) (fx, fy float64, ok bool) {
	if CircleWalkable(blocked, w, h, nx, ny) {
		return nx, ny, true
	}
	if CircleWalkable(blocked, w, h, nx, oy) {
		return nx, oy, true
	}
	if CircleWalkable(blocked, w, h, ox, ny) {
		return ox, ny, true
	}
	return ox, oy, false
}
