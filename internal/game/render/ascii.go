package render

import (
	"fmt"
	"math"
	"strings"

	"hack2026mart/internal/game/protocol"
)

func Frame(playerID string, snap *protocol.RoomSnapshot, hud GunHUDState, termW int, termH int, gfx *WadGraphics) string {
	if snap == nil || snap.Width < 4 || snap.Height < 4 {
		return "loading..."
	}
	me := protocol.PlayerState{}
	found := false
	for _, p := range snap.Players {
		if p.ID == playerID {
			me = p
			found = true
			break
		}
	}
	if !found {
		return "waiting for spawn..."
	}
	blocked := map[string]struct{}{}
	for _, w := range snap.Walls {
		blocked[pointKey(w.X, w.Y)] = struct{}{}
	}
	viewW := clamp(termW, 70, 220)
	viewH := clamp(termH-4, 24, 70)
	scene := make([][]rune, viewH)
	colors := make([][]uint32, viewH)
	half := viewH / 2
	// Фон за стенами: пустой «небосвод» (без FLAT), пол — спокойный градиент с перспективой.
	for y := range scene {
		scene[y] = make([]rune, viewW)
		colors[y] = make([]uint32, viewW)
		for x := range scene[y] {
			if y < half {
				scene[y][x] = ' '
				colors[y][x] = voidCeilingColor(y, half)
			} else {
				scene[y][x] = floorPerspectiveGlyph(y, viewH, half, x, viewW)
				colors[y][x] = floorPerspectiveColor(y, viewH, half, x, viewW)
			}
		}
	}
	fov := math.Pi / 3.0
	maxDist := 48.0
	distAtCol := make([]float64, viewW)
	rawDist := make([]float64, viewW)
	hitSides := make([]bool, viewW)
	texUs := make([]float64, viewW)
	for col := 0; col < viewW; col++ {
		rayAngle := me.Angle - fov/2 + fov*(float64(col)/float64(viewW-1))
		dist, hitSide, texU := castRay(float64(me.X)+0.5, float64(me.Y)+0.5, rayAngle, blocked, maxDist)
		corrected := dist * math.Cos(rayAngle-me.Angle)
		if corrected < 0.001 {
			corrected = 0.001
		}
		rawDist[col] = dist
		distAtCol[col] = corrected
		hitSides[col] = hitSide
		texUs[col] = texU
	}
	prevWallTop := viewH / 2
	prevWallBottom := viewH / 2
	for col := 0; col < viewW; col++ {
		dist := rawDist[col]
		hitSide := hitSides[col]
		texU := texUs[col]
		corrected := distAtCol[col]
		wallH := int(float64(viewH) * 0.9 / corrected)
		if wallH > viewH {
			wallH = viewH
		}
		top := (viewH - wallH) / 2
		bottom := top + wallH
		jumpL := 0.0
		if col > 0 {
			jumpL = math.Abs(rawDist[col] - rawDist[col-1])
		}
		jumpR := 0.0
		if col < viewW-1 {
			jumpR = math.Abs(rawDist[col] - rawDist[col+1])
		}
		edgeL := jumpL > 1.8
		edgeR := jumpR > 1.8
		edgeVert := col > 0 && (absInt(top-prevWallTop) > 2 || absInt(bottom-prevWallBottom) > 2)
		edge := edgeL || edgeR || edgeVert
		dr := dist / maxDist
		prevWallTop = top
		prevWallBottom = bottom
		for y := top; y < bottom && y < viewH; y++ {
			if y >= 0 {
				var ch rune
				var wallCol uint32
				vv := 0.5
				if bottom-top > 1 {
					vv = float64(y-top) / float64(bottom-top-1)
				}
				if gfx != nil && gfx.OK {
					uu := fractPhase(texU)
					var br int
					wallCol, br = gfx.SampleWall(uu, vv, dr, hitSide)
					ch = wallDepthGlyph(br, vv, edgeL, edgeR, edgeVert, jumpL, jumpR, hitSide)
					if edge {
						wallCol = darkenPacked(wallCol, 0.82)
					}
				} else {
					lum := wallLumFromRay(dist, maxDist, hitSide, texU)
					ch = wallDepthGlyph(lum, vv, edgeL, edgeR, edgeVert, jumpL, jumpR, hitSide)
					wallCol = wallColorRGB(dist, maxDist, hitSide, edge, texU)
				}
				scene[y][col] = ch
				colors[y][col] = wallCol
			}
		}
	}

	drawOtherPlayers(scene, colors, distAtCol, me, snap, playerID, viewW, viewH, fov, gfx)

	drawTracer(scene, colors, hud)
	drawPistol(scene, colors, hud, gfx)

	var b strings.Builder
	b.WriteString("\x1b[2J\x1b[H")
	b.WriteString(fmt.Sprintf("DOOM SSH ARENA FPS | map=%s room=%s\n", snap.MapTitle, snap.RoomID))
	b.WriteString("Controls: W/S move, A/D turn, SPACE fire, q quit\n\n")
	writeColoredScene(&b, scene, colors)
	return b.String()
}

func castRay(px, py, angle float64, blocked map[string]struct{}, maxDist float64) (float64, bool, float64) {
	step := 0.08
	prevX := int(math.Floor(px))
	for d := 0.2; d <= maxDist; d += step {
		hitX := px + math.Cos(angle)*d
		hitY := py + math.Sin(angle)*d
		x := int(math.Floor(hitX))
		y := int(math.Floor(hitY))
		if _, ok := blocked[pointKey(x, y)]; ok {
			// Вертикальная грань сетки (NS): u = позиция вдоль стены по Y.
			// Горизонтальная (EW): u = позиция по X.
			hitVertical := x != prevX
			var u float64
			if hitVertical {
				u = fractPhase(hitY)
			} else {
				u = fractPhase(hitX)
			}
			return d, hitVertical, u
		}
		prevX = x
	}
	return maxDist, false, 0
}

func pointKey(x, y int) string {
	return fmt.Sprintf("%d:%d", x, y)
}

// voidCeilingColor — «пустой» небосвод (без символов, только лёгкий градиент к горизонту).
func voidCeilingColor(y, half int) uint32 {
	if half < 1 {
		return RGBPacked(14, 14, 22)
	}
	t := float64(half-1-y) / float64(half)
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	r := byte(10 + t*28)
	g := byte(10 + t*26)
	b := byte(14 + t*34)
	return RGBPacked(r, g, b)
}

func floorPerspectiveGlyph(y, viewH, half, x, viewW int) rune {
	fh := viewH - half
	if fh < 1 {
		return ':'
	}
	t := float64(y-half) / float64(fh)
	off := float64(x)/float64(viewW) + fractPhase(float64(x+y)*0.061)
	shade := t*0.85 + 0.07*math.Sin(off*6.2831853)
	lum := int(32 + shade*198)
	return floorGlyphFromLum(lum)
}

func floorGlyphFromLum(lum int) rune {
	if lum < 0 {
		lum = 0
	}
	if lum > 255 {
		lum = 255
	}
	ramp := [...]rune{' ', ' ', ':', ':', ';', '.', ',', ',', '-', '`', '~', '~', '=', '+', '*', '#'}
	n := len(ramp)
	step := 255 / (n - 1)
	i := lum / step
	if i >= n {
		i = n - 1
	}
	return ramp[i]
}

func floorPerspectiveColor(y, viewH, half, x, viewW int) uint32 {
	fh := viewH - half
	if fh < 1 {
		return RGBPacked(92, 68, 46)
	}
	t := float64(y-half) / float64(fh)
	off := float64(x) / float64(viewW)
	r := clamp(int(62+t*98+off*22), 0, 255)
	g := clamp(int(44+t*78+off*16), 0, 255)
	b := clamp(int(32+t*52+off*10), 0, 255)
	return RGBPacked(byte(r), byte(g), byte(b))
}

func wallLumFromRay(dist, maxDist float64, vertical bool, phase float64) int {
	r := dist / maxDist
	if r < 0 {
		r = 0
	}
	if r > 1 {
		r = 1
	}
	lum := int((1.0 - r*0.88) * 255)
	if vertical {
		lum = int(float64(lum) * 0.9)
	}
	if phase != 0 {
		lum += int(math.Sin(phase*1.7) * 28)
	}
	if lum < 0 {
		lum = 0
	}
	if lum > 255 {
		lum = 255
	}
	return lum
}

// wallDepthGlyph — многоступенчатая рампа как в «консольном» Doom: глубина, контур, тень у пола.
func wallDepthGlyph(br int, vv float64, edgeL, edgeR, edgeVert bool, jumpL, jumpR float64, hitVertical bool) rune {
	bottomShade := 0.45 + 0.55 * (1.0 - vv)
	topShade := 0.70 + 0.30 * vv
	adj := float64(br) * bottomShade * topShade
	if edgeVert {
		adj *= 0.84
	}
	if edgeL {
		if jumpL > 3.2 {
			adj *= 0.44
		} else {
			adj *= 0.60
		}
	}
	if edgeR {
		if jumpR > 3.2 {
			adj *= 0.70
		} else {
			adj *= 0.80
		}
	}
	if hitVertical {
		adj *= 0.90
	}
	lum := int(adj)
	if lum < 0 {
		lum = 0
	}
	if lum > 255 {
		lum = 255
	}
	if edgeL && jumpL > 2.4 && lum < 105 {
		return '│'
	}
	if edgeR && jumpR > 2.4 && lum < 90 {
		return '│'
	}
	return depthGlyphFromLum(lum)
}

func depthGlyphFromLum(lum int) rune {
	if lum < 0 {
		lum = 0
	}
	if lum > 255 {
		lum = 255
	}
	ramp := [...]rune{
		'█', '█', '█', '▓', '▓', '▓', '▓', '▒', '▒', '▒', '░', '░', '░', '·', '·', ':', ':', ';', '.', '`', ' ',
	}
	n := len(ramp)
	step := 255 / (n - 1)
	if step < 1 {
		step = 1
	}
	i := lum / step
	if i >= n {
		i = n - 1
	}
	return ramp[i]
}

func drawPistol(scene [][]rune, colors [][]uint32, hud GunHUDState, gfx *WadGraphics) {
	h := len(scene)
	if h == 0 {
		return
	}
	w := len(scene[0])
	center := w / 2

	if gfx != nil && len(gfx.Pistol) > 0 {
		fi := pistolFrameFromHUD(hud, len(gfx.Pistol))
		fr := gfx.Pistol[fi]
		rows := len(fr.Chars)
		if rows > 0 && len(fr.Chars[0]) > 0 {
			cols := len(fr.Chars[0])
			yBase := h - rows - 1
			if yBase < 0 {
				yBase = 0
			}
			bobY, bobX := walkBobOffsets(hud)
			yBase += bobY
			// Чуть левее центра + покачивание при ходьбе.
			startX := center - cols/2 - 2 + bobX
			for sy := 0; sy < rows; sy++ {
				y := yBase + sy
				if y < 0 || y >= h {
					continue
				}
				for sx := 0; sx < cols && sx < len(fr.RGB[sy]); sx++ {
					ch := fr.Chars[sy][sx]
					if ch == 0 {
						continue
					}
					x := startX + sx
					if x < 0 || x >= w {
						continue
					}
					scene[y][x] = ch
					colors[y][x] = fr.RGB[sy][sx]
				}
			}
			return
		}
	}

	// ASCII fallback если в WAD нет спрайтов
	fi := pistolFrameFromHUD(hud, 4)
	yBase := h - 4
	sprite := []string{
		"        __/######\\__        ",
		"      _/############\\_      ",
		"        \\____##____/        ",
	}
	switch fi {
	case 1:
		yBase = h - 3
		sprite[0] = "       __/##**##\\__         "
		sprite[1] = "      _/##**####**\\_        "
	case 2:
		yBase = h - 5
		sprite[0] = "        __/######\\__        "
		sprite[1] = "      _/##########\\_        "
	case 3:
		yBase = h - 4
		sprite[0] = "        __/######\\__        "
		sprite[1] = "      _/##############\\_    "
	default: // idle
		yBase = h - 3
	}
	bobY, bobX := walkBobOffsets(hud)
	yBase += bobY
	for sy, row := range sprite {
		y := yBase + sy
		if y < 0 || y >= h {
			continue
		}
		startX := center - len(row)/2 - 2 + bobX
		for sx, ch := range row {
			x := startX + sx
			if x < 0 || x >= w {
				continue
			}
			if ch != ' ' {
				scene[y][x] = ch
				if ch == '*' {
					colors[y][x] = RGBPacked(255, 240, 120)
				} else if ch == '#' {
					colors[y][x] = RGBPacked(140, 130, 125)
				} else {
					colors[y][x] = RGBPacked(210, 210, 215)
				}
			}
		}
	}
}

func drawTracer(scene [][]rune, colors [][]uint32, hud GunHUDState) {
	if !showTracer(hud) {
		return
	}
	h := len(scene)
	if h == 0 {
		return
	}
	w := len(scene[0])
	cx := w / 2
	const tracerDrop = 10 // трасса ниже центра экрана (+2 к прежнему смещению)
	startY := h/2 + 2 + tracerDrop
	endY := h/4 + tracerDrop
	if showMuzzleFlash(hud) {
		for dy := 0; dy < 3; dy++ {
			y := startY + dy
			if y >= 0 && y < h {
				for dx := -2; dx <= 2; dx++ {
					x := cx + dx
					if x >= 0 && x < w {
						scene[y][x] = '*'
						colors[y][x] = RGBPacked(255, 220, 80)
					}
				}
			}
		}
	}
	for y := startY; y >= endY; y-- {
		if y < 0 || y >= h {
			continue
		}
		scene[y][cx] = '|'
		colors[y][cx] = RGBPacked(255, 255, 100)
		if showMuzzleFlash(hud) && cx+1 < w {
			scene[y][cx+1] = '.'
			colors[y][cx+1] = RGBPacked(255, 200, 60)
		}
		if showMuzzleFlash(hud) && cx-1 >= 0 {
			scene[y][cx-1] = '.'
			colors[y][cx-1] = RGBPacked(255, 200, 60)
		}
	}
	if endY >= 0 && endY < h {
		scene[endY][cx] = 'x'
		colors[endY][cx] = RGBPacked(255, 255, 200)
	}
}

func writeColoredScene(sb *strings.Builder, scene [][]rune, colors [][]uint32) {
	var lr, lg, lb byte
	var has bool
	for y := 0; y < len(scene); y++ {
		for x := 0; x < len(scene[y]); x++ {
			c := colors[y][x]
			r := byte(c >> 16)
			g := byte(c >> 8)
			b := byte(c)
			if !has || r != lr || g != lg || b != lb {
				sb.WriteString(fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b))
				lr, lg, lb = r, g, b
				has = true
			}
			sb.WriteRune(scene[y][x])
		}
		sb.WriteString("\x1b[0m\n")
		has = false
	}
}

func wallColorRGB(dist, maxDist float64, vertical bool, edge bool, phase float64) uint32 {
	if edge {
		// Контур — тёмный, не белый (как тень на углу в Doom).
		return RGBPacked(58, 48, 44)
	}
	ratio := dist / maxDist
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	// Тёмная ржаво-коричневая палитра (без WAD), уход в чёрное к дальнему плану.
	rr := int(112 - ratio*78)
	gg := int(78 - ratio*58)
	bb := int(58 - ratio*42)
	if vertical {
		rr = int(float64(rr) * 0.88)
		gg = int(float64(gg) * 0.88)
		bb = int(float64(bb) * 0.88)
	}
	if phase != 0 && int(math.Abs(math.Sin(phase*1.5))*10)%2 == 1 {
		if rr < 120 {
			rr += 5
		}
		if gg < 100 {
			gg += 4
		}
	}
	rr = clamp(rr, 18, 120)
	gg = clamp(gg, 14, 95)
	bb = clamp(bb, 12, 78)
	return RGBPacked(byte(rr), byte(gg), byte(bb))
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func fractPhase(x float64) float64 {
	x = math.Mod(x, 1)
	if x < 0 {
		x += 1
	}
	return x
}

func darkenPacked(col uint32, k float64) uint32 {
	if k < 0 {
		k = 0
	}
	if k > 1 {
		k = 1
	}
	r := float64(byte(col >> 16))
	g := float64(byte(col >> 8))
	b := float64(byte(col))
	return RGBPacked(byte(r*k), byte(g*k), byte(b*k))
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
