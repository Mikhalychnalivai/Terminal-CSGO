// Package navmesh — классический 2D NavMesh: полигоны проходимой зоны, граф соседей, A*.
// Строится из сетки стен (как карта Doom/SSH ARENA): сливаем проходимые клетки в осевые прямоугольники.
package navmesh

import "math"

// Vec2 — точка в координатах мира (как X/Y игрока на сервере).
type Vec2 struct {
	X, Y float64
}

// Rect — ось-ориентированный выпуклый полигон [MinX,MaxX)×[MinY,MaxY) в координатах клеток.
type Rect struct {
	MinX, MinY float64
	MaxX, MaxY float64
	Neighbors []int
}

// Mesh — набор полигонов и рёбер между соседними (общая сторона с ненулевым перекрытием).
type Mesh struct {
	Polys []Rect
}

// Centroid центра прямоугольника.
func (r Rect) Centroid() Vec2 {
	return Vec2{
		X: (r.MinX + r.MaxX) * 0.5,
		Y: (r.MinY + r.MaxY) * 0.5,
	}
}

// Contains — строго внутри [Min,Max) (граница относится к соседнему полигону с той же координатой Min).
func (r Rect) Contains(x, y float64) bool {
	return x >= r.MinX && x < r.MaxX && y >= r.MinY && y < r.MaxY
}

// PolygonAt возвращает индекс полигона, содержащего точку, или -1.
func (m *Mesh) PolygonAt(x, y float64) int {
	for i := range m.Polys {
		if m.Polys[i].Contains(x, y) {
			return i
		}
	}
	return -1
}

func dist2(a, b Vec2) float64 {
	dx := a.X - b.X
	dy := a.Y - b.Y
	return math.Hypot(dx, dy)
}

func intervalOverlap(a0, a1, b0, b1 float64) float64 {
	t := math.Min(a1, b1) - math.Max(a0, b0)
	if t <= 0 {
		return 0
	}
	return t
}

// shareEdge — общая сторона с ненулевой длиной (не только угол).
func shareEdge(a, b Rect) bool {
	// Вертикальная грань: MaxX одного = MinX другого
	if a.MaxX == b.MinX {
		if intervalOverlap(a.MinY, a.MaxY, b.MinY, b.MaxY) > 1e-6 {
			return true
		}
	}
	if b.MaxX == a.MinX {
		if intervalOverlap(a.MinY, a.MaxY, b.MinY, b.MaxY) > 1e-6 {
			return true
		}
	}
	// Горизонтальная грань
	if a.MaxY == b.MinY {
		if intervalOverlap(a.MinX, a.MaxX, b.MinX, b.MaxX) > 1e-6 {
			return true
		}
	}
	if b.MaxY == a.MinY {
		if intervalOverlap(a.MinX, a.MaxX, b.MinX, b.MaxX) > 1e-6 {
			return true
		}
	}
	return false
}

func computeNeighbors(polys []Rect) {
	n := len(polys)
	for i := 0; i < n; i++ {
		polys[i].Neighbors = polys[i].Neighbors[:0]
	}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if shareEdge(polys[i], polys[j]) {
				polys[i].Neighbors = append(polys[i].Neighbors, j)
				polys[j].Neighbors = append(polys[j].Neighbors, i)
			}
		}
	}
}
