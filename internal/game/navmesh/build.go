package navmesh

import (
	"hack2026mart/internal/game/nav"
)

// BuildFromBlocked строит NavMesh: жадно покрывает проходимые клетки ось-ориентированными прямоугольниками.
// width/height — размер сетки как на сервере; blocked — те же ключи, что nav.CellKey(gx,gy).
func BuildFromBlocked(width, height int, blocked map[uint64]struct{}) *Mesh {
	if width < 3 || height < 3 {
		return &Mesh{}
	}
	covered := make([][]bool, height)
	for y := range covered {
		covered[y] = make([]bool, width)
	}
	var polys []Rect
	for gy := 1; gy <= height-2; gy++ {
		for gx := 1; gx <= width-2; gx++ {
			if covered[gy][gx] {
				continue
			}
			if _, ok := blocked[nav.CellKey(gx, gy)]; ok {
				continue
			}
			w0 := 0
			for gx2 := gx; gx2 <= width-2; gx2++ {
				if _, ok := blocked[nav.CellKey(gx2, gy)]; ok {
					break
				}
				if covered[gy][gx2] {
					break
				}
				w0++
			}
			if w0 == 0 {
				continue
			}
			h0 := 0
			for gy2 := gy; gy2 <= height-2; gy2++ {
				rowOk := true
				for dx := 0; dx < w0; dx++ {
					gx2 := gx + dx
					if _, ok := blocked[nav.CellKey(gx2, gy2)]; ok {
						rowOk = false
						break
					}
					if covered[gy2][gx2] {
						rowOk = false
						break
					}
				}
				if !rowOk {
					break
				}
				h0++
			}
			for dy := 0; dy < h0; dy++ {
				for dx := 0; dx < w0; dx++ {
					covered[gy+dy][gx+dx] = true
				}
			}
			polys = append(polys, Rect{
				MinX: float64(gx),
				MinY: float64(gy),
				MaxX: float64(gx + w0),
				MaxY: float64(gy + h0),
			})
		}
	}
	computeNeighbors(polys)
	return &Mesh{Polys: polys}
}
