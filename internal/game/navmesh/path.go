package navmesh

import (
	"math"
)

// FindPath — A* по графу полигонов; возвращает полилинию от старта до цели (через центроиды промежуточных полигонов).
// Если старт или финиш вне mesh — (nil, false).
func (m *Mesh) FindPath(sx, sy, ex, ey float64) ([]Vec2, bool) {
	if m == nil || len(m.Polys) == 0 {
		return nil, false
	}
	si := m.PolygonAt(sx, sy)
	ei := m.PolygonAt(ex, ey)
	if si < 0 || ei < 0 {
		return nil, false
	}
	if si == ei {
		return []Vec2{{sx, sy}, {ex, ey}}, true
	}
	indices, ok := m.aStar(si, ei)
	if !ok {
		return nil, false
	}
	// indices: start -> ... -> goal (полигоны по порядку)
	way := make([]Vec2, 0, len(indices)+2)
	way = append(way, Vec2{sx, sy})
	for i := 1; i < len(indices)-1; i++ {
		way = append(way, m.Polys[indices[i]].Centroid())
	}
	way = append(way, Vec2{ex, ey})
	return way, true
}

func (m *Mesh) aStar(start, goal int) ([]int, bool) {
	n := len(m.Polys)
	goalC := m.Polys[goal].Centroid()

	h := func(i int) float64 {
		return dist2(m.Polys[i].Centroid(), goalC)
	}

	open := map[int]bool{start: true}
	gScore := map[int]float64{start: 0}
	fScore := map[int]float64{start: h(start)}
	parent := map[int]int{}

	for len(open) > 0 {
		var cur int
		var bestF float64
		first := true
		for k := range open {
			fv, ok := fScore[k]
			if !ok {
				continue
			}
			if first || fv < bestF {
				bestF = fv
				cur = k
				first = false
			}
		}
		delete(open, cur)
		if cur == goal {
			// восстановить путь полигонов
			var path []int
			for p := goal; ; {
				path = append(path, p)
				pp, ok := parent[p]
				if !ok {
					break
				}
				p = pp
			}
			// path: goal ... start
			for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
				path[i], path[j] = path[j], path[i]
			}
			return path, true
		}
		cc := m.Polys[cur].Centroid()
		for _, nb := range m.Polys[cur].Neighbors {
			if nb < 0 || nb >= n {
				continue
			}
			nc := m.Polys[nb].Centroid()
			step := dist2(cc, nc)
			if math.IsNaN(step) || math.IsInf(step, 0) {
				step = 0.01
			}
			tentative := gScore[cur] + step
			oldG, ok := gScore[nb]
			if !ok || tentative < oldG {
				parent[nb] = cur
				gScore[nb] = tentative
				fScore[nb] = tentative + h(nb)
				open[nb] = true
			}
		}
	}
	return nil, false
}
