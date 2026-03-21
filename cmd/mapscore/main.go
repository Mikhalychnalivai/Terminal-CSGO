package main

import (
	"flag"
	"fmt"
	"math"
	"sort"
	"strings"

	"hack2026mart/internal/game/wad"
)

func main() {
	wadPath := flag.String("wad", "shooter wed/SHOOTER.WAD", "Path to SHOOTER.WAD")
	mapList := flag.String("maps", "E1M1,E1M2,E1M3,E1M4,E1M5,E1M6,E1M7,E1M8,E1M9", "Comma-separated map lump names")
	flag.Parse()

	candidates := splitComma(*mapList)
	type scoreRow struct {
		MapName string
		Score   float64
		Spawns  [][2]int
	}

	rows := make([]scoreRow, 0, len(candidates))
	for _, m := range candidates {
		md, err := wad.LoadMap(*wadPath, strings.TrimSpace(m))
		if err != nil {
			fmt.Printf("%s: load error: %v\n", m, err)
			continue
		}

		spawns := projectedSpawns(md, 80, 28)
		if len(spawns) == 0 {
			fmt.Printf("%s: no player spawns (THINGS types 1..4)\n", m)
			continue
		}

		score := symmetryScore(spawns, 80, 28)
		rows = append(rows, scoreRow{MapName: md.MapName, Score: score, Spawns: spawns})
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].Score < rows[j].Score })
	if len(rows) == 0 {
		fmt.Println("No candidate maps produced scores.")
		return
	}

	fmt.Println("Best candidates (lower = more symmetric spawns):")
	for i := 0; i < len(rows) && i < 5; i++ {
		r := rows[i]
		fmt.Printf("- %s score=%.2f spawns=%s\n", r.MapName, r.Score, formatSpawns(r.Spawns))
	}

	fmt.Printf("\nPick: %s\n", rows[0].MapName)
}

func splitComma(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func formatSpawns(spawns [][2]int) string {
	sb := strings.Builder{}
	sb.WriteString("[")
	for i, p := range spawns {
		if i > 0 {
			sb.WriteString(" ")
		}
		sb.WriteString(fmt.Sprintf("(%d,%d)", p[0], p[1]))
	}
	sb.WriteString("]")
	return sb.String()
}

func projectedSpawns(md *wad.MapData, w, h int) [][2]int {
	minX, minY, maxX, maxY := bounds(md.Vertices)

	spawns := make([][2]int, 0, 4)
	for _, t := range md.Things {
		if t.Type < 1 || t.Type > 4 {
			continue
		}
		x := project(int(t.X), minX, maxX, 1, w-2)
		y := project(int(t.Y), minY, maxY, 1, h-2)
		spawns = append(spawns, [2]int{x, y})
	}
	return spawns
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
		minX = int(math.Min(float64(minX), float64(x)))
		maxX = int(math.Max(float64(maxX), float64(x)))
		minY = int(math.Min(float64(minY), float64(y)))
		maxY = int(math.Max(float64(maxY), float64(y)))
	}
	return minX, minY, maxX, maxY
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

func symmetryScore(spawns [][2]int, w, h int) float64 {
	xSum := (1 + (w - 2))
	ySum := (1 + (h - 2))
	mirrors := make([][2]int, 0, len(spawns))
	for _, s := range spawns {
		mirrors = append(mirrors, [2]int{xSum - s[0], ySum - s[1]})
	}

	var sum float64
	for i := range spawns {
		mx, my := mirrors[i][0], mirrors[i][1]
		best := math.MaxFloat64
		for j := range spawns {
			dx := float64(spawns[j][0] - mx)
			dy := float64(spawns[j][1] - my)
			d2 := dx*dx + dy*dy
			if d2 < best {
				best = d2
			}
		}
		sum += best
	}
	return sum / float64(len(spawns))
}
