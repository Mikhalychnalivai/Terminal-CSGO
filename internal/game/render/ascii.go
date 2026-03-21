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
	for y := range scene {
		scene[y] = make([]rune, viewW)
		colors[y] = make([]uint32, viewW)
		for x := range scene[y] {
			if gfx != nil && gfx.Ceiling != nil && y < half {
				u := float64(x) / float64(viewW)
				v := float64(y) / float64(half)
				scene[y][x], colors[y][x] = gfx.SampleFlat(gfx.Ceiling, u, v)
			} else if gfx != nil && gfx.Floor != nil && y >= half {
				u := float64(x) / float64(viewW)
				v := float64(y-half) / float64(viewH-half)
				scene[y][x], colors[y][x] = gfx.SampleFlat(gfx.Floor, u, v)
			} else if y < half {
				scene[y][x] = ceilingShade(y, viewH)
				colors[y][x] = ceilingColorRGB(y, viewH)
			} else {
				scene[y][x] = floorShade(y, viewH)
				colors[y][x] = floorColorRGB(y, viewH)
			}
		}
	}
	fov := math.Pi / 3.0
	maxDist := 48.0
	distAtCol := make([]float64, viewW)
	prevDist := maxDist
	prevWallTop := viewH / 2
	prevWallBottom := viewH / 2
	for col := 0; col < viewW; col++ {
		rayAngle := me.Angle - fov/2 + fov*(float64(col)/float64(viewW-1))
		dist, hitSide, texPhase := castRay(float64(me.X)+0.5, float64(me.Y)+0.5, rayAngle, blocked, maxDist)
		corrected := dist * math.Cos(rayAngle-me.Angle)
		if corrected < 0.001 {
			corrected = 0.001
		}
		distAtCol[col] = corrected
		wallH := int(float64(viewH) * 0.9 / corrected)
		if wallH > viewH {
			wallH = viewH
		}
		top := (viewH - wallH) / 2
		bottom := top + wallH
		edge := math.Abs(prevDist-dist) > 1.8 || absInt(top-prevWallTop) > 2 || absInt(bottom-prevWallBottom) > 2
		dr := dist / maxDist
		prevDist = dist
		prevWallTop = top
		prevWallBottom = bottom
		for y := top; y < bottom && y < viewH; y++ {
			if y >= 0 {
				var ch rune
				var wallCol uint32
				if gfx != nil && gfx.OK {
					vv := 0.5
					if bottom-top > 1 {
						vv = float64(y-top) / float64(bottom-top-1)
					}
					uu := fractPhase(texPhase*0.11 + float64(col)*0.01)
					ch, wallCol = gfx.SampleWall(uu, vv, dr, hitSide)
					if edge {
						ch = '|'
						wallCol = RGBPacked(240, 240, 240)
					}
				} else {
					ch = shade(dist, maxDist, hitSide, texPhase)
					wallCol = wallColorRGB(dist, maxDist, hitSide, edge, texPhase)
					if edge {
						ch = '|'
					}
					ch = applyVerticalShading(ch, y, top, bottom)
				}
				scene[y][col] = ch
				colors[y][col] = wallCol
			}
		}
		if top > 0 && top < viewH {
			scene[top][col] = '_'
			colors[top][col] = RGBPacked(120, 200, 220)
		}
		if bottom-1 >= 0 && bottom-1 < viewH {
			scene[bottom-1][col] = '-'
			colors[bottom-1][col] = RGBPacked(120, 200, 220)
		}
	}

	drawOtherPlayers(scene, colors, distAtCol, me, snap, playerID, viewW, viewH, fov, gfx)

	drawTracer(scene, colors, hud)
	drawPistol(scene, colors, hud, gfx)

	var b strings.Builder
	b.WriteString("\x1b[2J\x1b[H")
	b.WriteString(fmt.Sprintf("SHOOTER SSH ARENA FPS | map=%s room=%s\n", snap.MapTitle, snap.RoomID))
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
			hitVertical := x != prevX
			phase := hitY
			if !hitVertical {
				phase = hitX
			}
			return d, hitVertical, phase
		}
		prevX = x
	}
	return maxDist, false, 0
}

func shade(dist, maxDist float64, vertical bool, phase float64) rune {
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
	return blockLumaChar(lum)
}

func pointKey(x, y int) string {
	return fmt.Sprintf("%d:%d", x, y)
}

func ceilingShade(y, h int) rune {
	r := float64(y) / float64(h/2)
	if r < 0.20 {
		return ' '
	}
	if r < 0.35 {
		return '`'
	}
	if r < 0.50 {
		return '.'
	}
	if r < 0.75 {
		return ':'
	}
	return ';'
}

func floorShade(y, h int) rune {
	r := float64(y-h/2) / float64(h/2)
	if r < 0.15 {
		return '.'
	}
	if r < 0.30 {
		return ','
	}
	if r < 0.45 {
		return '-'
	}
	if r < 0.65 {
		return '='
	}
	return '~'
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
	default:
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
	const tracerDrop = 10
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
		return RGBPacked(245, 245, 245)
	}
	ratio := dist / maxDist
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}

	rr := byte(235 - ratio*190)
	gg := byte(215 - ratio*175)
	bb := byte(195 - ratio*155)
	if vertical {
		rr = byte(float64(rr) * 0.92)
		gg = byte(float64(gg) * 0.92)
		bb = byte(float64(bb) * 0.92)
	}
	if phase != 0 && int(math.Abs(math.Sin(phase*1.5))*10)%2 == 1 {
		if rr < 250 {
			rr += 8
		}
		if gg < 250 {
			gg += 5
		}
	}
	return RGBPacked(rr, gg, bb)
}

func ceilingColorRGB(y, h int) uint32 {
	r := float64(y) / float64(h/2)
	switch {
	case r < 0.25:
		return RGBPacked(55, 55, 95)
	case r < 0.50:
		return RGBPacked(65, 65, 105)
	case r < 0.75:
		return RGBPacked(75, 75, 115)
	default:
		return RGBPacked(85, 85, 125)
	}
}

func floorColorRGB(y, h int) uint32 {
	r := float64(y-h/2) / float64(h/2)
	switch {
	case r < 0.20:
		return RGBPacked(75, 55, 35)
	case r < 0.40:
		return RGBPacked(95, 70, 45)
	case r < 0.65:
		return RGBPacked(115, 85, 55)
	default:
		return RGBPacked(135, 100, 65)
	}
}

func applyVerticalShading(ch rune, y, top, bottom int) rune {
	if bottom <= top {
		return ch
	}
	r := float64(y-top) / float64(bottom-top)
	if ch == '|' || ch == '_' || ch == '-' {
		return ch
	}
	if r < 0.15 {
		return blockLumaChar(35)
	}
	if r > 0.85 && ch != ' ' {
		return blockLumaChar(210)
	}
	return ch
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

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
