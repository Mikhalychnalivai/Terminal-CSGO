package render

import (
	"math"
	"sort"

	"hack2026mart/internal/game/protocol"
)

const spriteMaxDist = 48.0

// spriteScreenScale — во сколько экрана по высоте масштабировать модель при corrected≈1 (было 0.82 — мелко).
const spriteScreenScale = 1.22

// spriteMinHeightFrac — минимальная высота спрайта как доля viewport (дальние цели остаются читаемыми).
const spriteMinHeightFrac = 0.26

// deadCorpseSquashDiv — труп: высота в N раз меньше «живой» (приплюснут вниз).
const deadCorpseSquashDiv = 3

// spriteExtraHeightLines — дополнительная высота модели + сдвиг вниз (как просили).
const spriteExtraHeightLines = 4

const spriteFootShiftDown = 2

// playerSpriteAnimOffsets — покачивание спрайта: при ходьбе двухчастотный шаг (вертикаль + лёгкий качок вбок).
func playerSpriteAnimOffsets(p protocol.PlayerState, nowUnixNano int64) (dy, dx int) {
	if p.Dead {
		return 0, 0
	}
	t := float64(nowUnixNano) / 1e9
	if p.Moving {
		ph := float64(p.WalkPhase%8) * (2 * math.Pi / 8)
		// Основной шаг по синусу фазы; вторая гармоника даёт «перекат» как при перестановке ног.
		step := math.Sin(ph)
		roll := math.Sin(ph * 2.0)
		dy = int(math.Round(step*4.8 + roll*1.1))
		dx = int(math.Round(roll*2.6 + math.Cos(ph)*0.9))
		return dy, dx
	}
	// idle: чуть заметнее «дыхание»
	dy = int(math.Round(math.Sin(t*1.7) * 1.45))
	dx = int(math.Round(math.Sin(t*2.3) * 0.7))
	return dy, dx
}

// spriteDepthShade — псевдо-3D: бока цилиндра темнее, ноги ниже по яркости, лёгкий перелив по углу на экране.
func spriteDepthShade(col uint32, u, v float64, relAng float64) uint32 {
	if u < 0 {
		u = 0
	}
	if u > 1 {
		u = 1
	}
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	nx := u*2 - 1
	edge := 1.0 - math.Abs(nx)*0.48
	vert := 0.68 + 0.32*(1.0-v)
	light := 1.0 + 0.07*math.Cos(relAng*1.15)
	k := edge * vert * light
	if k < 0.38 {
		k = 0.38
	}
	if k > 1.0 {
		k = 1.0
	}
	return darkenPacked(col, k)
}

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

// lumaCharRamp — совпадает с wad_graphics.lumaChar (морпех / дистанция).
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
	nowUnixNano int64,
) {
	if snap == nil || len(snap.Players) < 2 {
		return
	}
	var list []playerDraw
	for _, p := range snap.Players {
		if p.ID == selfID {
			continue
		}
		dx := p.X - me.X
		dy := p.Y - me.Y
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
		drawOnePlayer(scene, colors, distAtCol, me, item, viewW, viewH, fov, gfx, nowUnixNano)
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
	nowUnixNano int64,
) {
	p := item.p
	dist := item.dist
	relAng := item.relAng

	phi := math.Atan2(me.Y-p.Y, me.X-p.X)
	rot := doomSpriteRot8(phi, p.Angle)

	corrected := dist * math.Cos(relAng)
	if corrected < 0.12 {
		corrected = 0.12
	}

	spriteH := int(float64(viewH) * spriteScreenScale / corrected)
	minH := int(float64(viewH) * spriteMinHeightFrac)
	if minH < 6 {
		minH = 6
	}
	if spriteH < minH {
		spriteH = minH
	}
	if spriteH < 2 {
		spriteH = 2
	}
	if spriteH > viewH-1 {
		spriteH = viewH - 1
	}
	spriteH += spriteExtraHeightLines
	if spriteH > viewH-1 {
		spriteH = viewH - 1
	}
	fullH := spriteH
	drawH := fullH
	if p.Dead {
		drawH = max(3, fullH/deadCorpseSquashDiv)
	}

	var fr *PistolHUDFrame
	if gfx != nil && gfx.PlayerSpritesOK && rot < len(gfx.Player8) {
		f := gfx.Player8[rot]
		fr = &f
	}
	if fr == nil || len(fr.Chars) == 0 {
		drawPlayerFallback(scene, colors, distAtCol, item, viewW, viewH, fov, rot, drawH, nowUnixNano)
		return
	}

	rows := len(fr.Chars)
	cols := len(fr.Chars[0])
	spriteW := int(float64(drawH) * float64(cols) / float64(rows))
	if spriteW < 1 {
		spriteW = 1
	}
	if p.Dead {
		spriteW = max(spriteW, min(viewW-1, drawH*5/2))
	}
	if spriteW > viewW-1 {
		spriteW = viewW - 1
	}

	bobY, bobX := playerSpriteAnimOffsets(p, nowUnixNano)
	centerCol := int((relAng/fov + 0.5) * float64(viewW-1))
	left := centerCol - spriteW/2 + bobX
	top := (viewH-drawH)/2 + bobY
	if !p.Dead {
		top += spriteFootShiftDown
	}

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
		for sy := 0; sy < drawH; sy++ {
			y := top + sy
			if y < 0 || y >= viewH {
				continue
			}
			v := float64(sy) / float64(max(1, drawH-1))
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
			ut := float64(ui) / float64(max(1, cols-1))
			vt := float64(vi) / float64(max(1, rows-1))
			col := fr.RGB[vi][ui]
			col = fogSpriteRGB(col, dr)
			col = spriteDepthShade(col, ut, vt, relAng)
			outCh := thinGlyphForDistance(ch, dr)
			scene[y][sc] = outCh
			colors[y][sc] = col
		}
	}
	if !p.Dead {
		drawOpponentMuzzleOverlay(scene, colors, distAtCol, left, top, spriteW, drawH, viewW, viewH, dist, p.FireAgeMs)
	} else {
		drawDeathBloodOverlay(scene, colors, distAtCol, left, top, spriteW, drawH, viewW, viewH, dist, nowUnixNano)
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
	nowUnixNano int64,
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
	p := item.p

	bobY, bobX := playerSpriteAnimOffsets(p, nowUnixNano)
	spriteW := max(7, spriteH*13/20)
	if p.Dead {
		spriteW = max(spriteW, min(viewW-1, spriteH*5/2))
	}
	if spriteW%2 == 0 {
		spriteW++
	}
	centerCol := int((relAng/fov + 0.5) * float64(viewW-1))
	left := centerCol - spriteW/2 + bobX
	top := (viewH-spriteH)/2 + bobY
	if !p.Dead {
		top += spriteFootShiftDown
	}
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
			u := float64(sx)/float64(max(1, spriteW-1)) + 1e-6
			v := float64(sy)/float64(max(1, spriteH-1))
			c = spriteDepthShade(c, u, v, relAng)
			scene[y][sc] = gch
			colors[y][sc] = c
		}
	}
	if !p.Dead {
		drawOpponentMuzzleOverlay(scene, colors, distAtCol, left, top, spriteW, spriteH, viewW, viewH, dist, p.FireAgeMs)
	} else {
		drawDeathBloodOverlay(scene, colors, distAtCol, left, top, spriteW, spriteH, viewW, viewH, dist, nowUnixNano)
	}
}

const opponentFireFlashMs = 1000

// blendTowardYellow смешивает base с target (0..1).
func blendTowardYellow(base uint32, target uint32, t float64) uint32 {
	if t <= 0 {
		return base
	}
	if t >= 1 {
		return target
	}
	br := float64(byte(base >> 16))
	bg := float64(byte(base >> 8))
	bb := float64(byte(base))
	tr := float64(byte(target >> 16))
	tg := float64(byte(target >> 8))
	tb := float64(byte(target))
	return RGBPacked(
		byte(br*(1-t)+tr*t),
		byte(bg*(1-t)+tg*t),
		byte(bb*(1-t)+tb*t),
	)
}

// drawOpponentMuzzleOverlay — жёлтые «искры» в центре спрайта при недавнем выстреле (FireAgeMs с сервера).
func drawOpponentMuzzleOverlay(scene [][]rune, colors [][]uint32, distAtCol []float64, left, top, spriteW, spriteH, viewW, viewH int, dist float64, fireAgeMs int) {
	if fireAgeMs <= 0 || fireAgeMs > opponentFireFlashMs {
		return
	}
	strength := 1.0 - float64(fireAgeMs)/float64(opponentFireFlashMs)
	cx := left + spriteW/2
	cy := top + spriteH/2
	burst := []rune{'*', '@', '+', '·'}
	yellowHot := RGBPacked(255, 235, 75)
	for dy := -2; dy <= 2; dy++ {
		for dx := -2; dx <= 2; dx++ {
			if dx*dx+dy*dy > 6 {
				continue
			}
			sc := cx + dx
			ry := cy + dy
			if sc < 0 || sc >= viewW || ry < 0 || ry >= viewH {
				continue
			}
			if dist >= distAtCol[sc]-0.02 {
				continue
			}
			ch := burst[(dx*7+dy*3+dx*dy+12)&3]
			edge := 1.0 - float64(dx*dx+dy*dy)/8.0
			if edge < 0 {
				edge = 0
			}
			t := strength * (0.5 + 0.5*edge)
			scene[ry][sc] = ch
			colors[ry][sc] = blendTowardYellow(colors[ry][sc], yellowHot, t*0.88)
		}
	}
}

// drawDeathBloodOverlay — лужа у ног и струи вниз (без «фонтана» вокруг).
func drawDeathBloodOverlay(
	scene [][]rune,
	colors [][]uint32,
	distAtCol []float64,
	left, top, spriteW, spriteH, viewW, viewH int,
	dist float64,
	nowUnixNano int64,
) {
	if spriteW < 1 || spriteH < 1 {
		return
	}
	dr := dist / spriteMaxDist
	bloodCh := []rune{':', ';', '~', ',', '`', '.', '·', '\''}
	footY := top + spriteH - 1
	t := float64(nowUnixNano) / 1e9

	// Лужа под приплюснутым телом
	for px := left; px < left+spriteW; px++ {
		if px < 0 || px >= viewW {
			continue
		}
		py := footY + 1
		if py >= viewH {
			continue
		}
		if dist >= distAtCol[px]-0.02 {
			continue
		}
		h := uint32(px*92837111 + footY*17)
		if h%2 != 0 {
			continue
		}
		ch := bloodCh[h%uint32(len(bloodCh))]
		col := fogSpriteRGB(RGBPacked(byte(130+int(h%70)), byte(6+int(h%10)), byte(10+int(h%16))), dr)
		scene[py][px] = ch
		colors[py][px] = col
	}

	nStreams := 6 + spriteW/6
	if nStreams < 4 {
		nStreams = 4
	}
	span := max(1, nStreams-1)
	for s := 0; s < nStreams; s++ {
		seed := int64(s*7919 + footY*31 + left)
		startX := left + (s*spriteW)/span
		startX = clamp(startX, left, left+spriteW-1)
		maxDrop := 10 + int(seed%11)
		for step := 1; step < maxDrop; step++ {
			px := startX + int(math.Sin(float64(step)*0.42+t*2.1+float64(s)*0.7)*2.2)
			py := footY + step
			if px < 0 || px >= viewW || py < 0 || py >= viewH {
				continue
			}
			if dist >= distAtCol[px]-0.02 {
				continue
			}
			if (seed+int64(step*13))%4 == 0 {
				continue
			}
			h := uint32(seed + int64(step*17) + int64(px*47))
			ch := bloodCh[h%uint32(len(bloodCh))]
			dark := 1.0 - float64(step)/float64(maxDrop+6)
			if dark < 0.22 {
				dark = 0.22
			}
			r := int(float64(110+int(h%85)) * dark)
			g := int(float64(4+int(h%9)) * dark)
			b := int(float64(6+int(h%12)) * dark)
			col := fogSpriteRGB(RGBPacked(byte(clamp(r, 0, 255)), byte(clamp(g, 0, 255)), byte(clamp(b, 0, 255))), dr)
			scene[py][px] = ch
			colors[py][px] = col
		}
	}
}
