package render

import (
	"math"
	"sort"

	"hack2026mart/internal/game/protocol"
)

const spriteMaxDist = 48.0

// doomSpriteRot8 — индекс кадра 0..7 (как PLAYA1..8): куда смотрит модель относительно камеры.
// phi — направление от другого игрока к нам (вектор к камере).
func doomSpriteRot8(phi, playerFacing float64) int {
	rel := phi - playerFacing
	for rel < -math.Pi {
		rel += 2 * math.Pi
	}
	for rel > math.Pi {
		rel -= 2 * math.Pi
	}
	rot := int((rel + math.Pi/8) / (math.Pi / 4))
	if rot < 0 {
		rot = 0
	}
	if rot > 7 {
		rot = 7
	}
	return rot
}

// lumaCharRamp — совпадает с wad_graphics.lumaChar (HUD-пистолет).
var lumaCharRamp = []rune{'@', '#', '8', '&', 'O', 'o', '*', '+', ':', '.', ' '}

// thinGlyphForDistance: базовый глиф уже из lumaChar (как у пистолета); дальше — сдвиг к тонкому концу рампы.
func thinGlyphForDistance(base rune, distRatio float64) rune {
	if distRatio < 0 {
		distRatio = 0
	}
	if distRatio > 1 {
		distRatio = 1
	}
	idx := -1
	for i, r := range lumaCharRamp {
		if r == base {
			idx = i
			break
		}
	}
	if idx < 0 {
		return base
	}
	push := int(distRatio * 6)
	ni := idx + push
	if ni >= len(lumaCharRamp) {
		ni = len(lumaCharRamp) - 1
	}
	return lumaCharRamp[ni]
}

func fogSpriteRGB(c uint32, distRatio float64) uint32 {
	fog := math.Min(1, math.Max(0, distRatio))
	r := int(byte(c >> 16))
	g := int(byte(c >> 8))
	b := int(byte(c))
	r = int(float64(r) * (1 - fog*0.55))
	g = int(float64(g) * (1 - fog*0.55))
	b = int(float64(b) * (1 - fog*0.55))
	if r < 0 {
		r = 0
	}
	if g < 0 {
		g = 0
	}
	if b < 0 {
		b = 0
	}
	if r > 255 {
		r = 255
	}
	if g > 255 {
		g = 255
	}
	if b > 255 {
		b = 255
	}
	return RGBPacked(byte(r), byte(g), byte(b))
}

type playerDraw struct {
	dist   float64
	p      protocol.PlayerState
	relAng float64 // угол относительно взгляда (для колонки)
}

// drawOtherPlayers рисует других игроков поверх мира (ближе стены — поверх).
func drawOtherPlayers(
	scene [][]rune,
	colors [][]uint32,
	distAtCol []float64,
	me protocol.PlayerState,
	snap *protocol.RoomSnapshot,
	selfID string,
	viewW, viewH int,
	fov float64,
	gfx *WadGraphics,
) {
	if snap == nil || len(snap.Players) < 2 {
		return
	}
	var list []playerDraw
	for _, p := range snap.Players {
		if p.ID == selfID {
			continue
		}
		dx := float64(p.X) - float64(me.X)
		dy := float64(p.Y) - float64(me.Y)
		dist := math.Hypot(dx, dy)
		if dist < 0.15 || dist > spriteMaxDist {
			continue
		}
		relAng := math.Atan2(dy, dx) - me.Angle
		for relAng < -math.Pi {
			relAng += 2 * math.Pi
		}
		for relAng > math.Pi {
			relAng -= 2 * math.Pi
		}
		if math.Abs(relAng) > fov/2+0.05 {
			continue
		}
		list = append(list, playerDraw{dist: dist, p: p, relAng: relAng})
	}
	if len(list) == 0 {
		return
	}
	sort.Slice(list, func(i, j int) bool { return list[i].dist > list[j].dist })

	for _, item := range list {
		drawOnePlayer(scene, colors, distAtCol, me, item, viewW, viewH, fov, gfx)
	}
}

func drawOnePlayer(
	scene [][]rune,
	colors [][]uint32,
	distAtCol []float64,
	me protocol.PlayerState,
	item playerDraw,
	viewW, viewH int,
	fov float64,
	gfx *WadGraphics,
) {
	p := item.p
	dist := item.dist
	relAng := item.relAng

	phi := math.Atan2(float64(me.Y)-float64(p.Y), float64(me.X)-float64(p.X))
	rot := doomSpriteRot8(phi, p.Angle)

	corrected := dist * math.Cos(relAng)
	if corrected < 0.12 {
		corrected = 0.12
	}

	spriteH := int(float64(viewH) * 0.82 / corrected)
	if spriteH < 2 {
		spriteH = 2
	}
	if spriteH > viewH-1 {
		spriteH = viewH - 1
	}

	var fr *PistolHUDFrame
	if gfx != nil && gfx.PlayerSpritesOK && rot < len(gfx.Player8) {
		f := gfx.Player8[rot]
		fr = &f
	}
	if fr == nil || len(fr.Chars) == 0 {
		drawPlayerFallback(scene, colors, distAtCol, item, viewW, viewH, fov, rot, spriteH)
		return
	}

	rows := len(fr.Chars)
	cols := len(fr.Chars[0])
	spriteW := int(float64(spriteH) * float64(cols) / float64(rows))
	if spriteW < 1 {
		spriteW = 1
	}
	if spriteW > viewW-1 {
		spriteW = viewW - 1
	}

	centerCol := int((relAng/fov + 0.5) * float64(viewW-1))
	left := centerCol - spriteW/2
	top := (viewH - spriteH) / 2

	dr := dist / spriteMaxDist

	for sx := 0; sx < spriteW; sx++ {
		sc := left + sx
		if sc < 0 || sc >= viewW {
			continue
		}
		if dist >= distAtCol[sc]-0.02 {
			continue
		}
		u := float64(sx) / float64(max(1, spriteW-1))
		ui := int(u * float64(cols-1))
		if ui < 0 {
			ui = 0
		}
		if ui >= cols {
			ui = cols - 1
		}
		for sy := 0; sy < spriteH; sy++ {
			y := top + sy
			if y < 0 || y >= viewH {
				continue
			}
			v := float64(sy) / float64(max(1, spriteH-1))
			vi := int(v * float64(rows-1))
			if vi < 0 {
				vi = 0
			}
			if vi >= rows {
				vi = rows - 1
			}
			ch := fr.Chars[vi][ui]
			if ch == 0 {
				continue
			}
			col := fr.RGB[vi][ui]
			col = fogSpriteRGB(col, dr)
			outCh := thinGlyphForDistance(ch, dr)
			scene[y][sc] = outCh
			colors[y][sc] = col
		}
	}
}

func drawPlayerFallback(
	scene [][]rune,
	colors [][]uint32,
	distAtCol []float64,
	item playerDraw,
	viewW, viewH int,
	fov float64,
	rot int,
	spriteH int,
) {
	// Стрелка направления в центре овального силуэта; глифы по глубине как у WAD-спрайта.
	arrow := []rune{'→', '↗', '↑', '↖', '←', '↙', '↓', '↘'}
	if rot < 0 {
		rot = 0
	}
	if rot > 7 {
		rot = 7
	}
	relAng := item.relAng
	dist := item.dist

	spriteW := max(5, spriteH*3/5)
	if spriteW%2 == 0 {
		spriteW++
	}
	centerCol := int((relAng/fov + 0.5) * float64(viewW-1))
	left := centerCol - spriteW/2
	top := (viewH - spriteH) / 2
	dr := dist / spriteMaxDist
	skin := fogSpriteRGB(RGBPacked(210, 165, 125), dr)
	armor := fogSpriteRGB(RGBPacked(0, 140, 55), dr)

	for sx := 0; sx < spriteW; sx++ {
		sc := left + sx
		if sc < 0 || sc >= viewW {
			continue
		}
		if dist >= distAtCol[sc]-0.02 {
			continue
		}
		nx := float64(sx)/float64(max(1, spriteW-1)) - 0.5
		for sy := 0; sy < spriteH; sy++ {
			y := top + sy
			if y < 0 || y >= viewH {
				continue
			}
			ny := float64(sy)/float64(max(1, spriteH-1)) - 0.5
			// Овал силуэта
			if nx*nx*1.15+ny*ny*0.95 > 0.24 {
				continue
			}
			var lum int
			var c uint32
			// Верх «шлем», низ «сапоги», центр ярче
			switch {
			case ny < -0.12:
				lum = 230 - int(math.Abs(nx)*100)
				c = skin
			case ny > 0.15:
				lum = 120 - int(math.Abs(nx)*60)
				c = fogSpriteRGB(RGBPacked(90, 92, 100), dr)
			default:
				lum = 180 - int(math.Abs(nx)*140) - int(math.Abs(ny)*40)
				c = armor
			}
			if lum < 35 {
				lum = 35
			}
			if lum > 255 {
				lum = 255
			}
			base := lumaChar(clamp(lum, 0, 255))
			gch := thinGlyphForDistance(base, dr)
			if sx == spriteW/2 && sy == spriteH/2 {
				gch = arrow[rot]
			}
			scene[y][sc] = gch
			colors[y][sc] = c
		}
	}
}
