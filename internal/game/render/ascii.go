package render

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"hack2026mart/internal/game/protocol"
)

// wallKey — ключ клетки стены без fmt (совпадает с nav.CellKey / predKey в gateway).
func wallKey(x, y int) uint64 {
	return uint64(uint32(x))<<32 | uint64(uint32(y))
}

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
	if me.Dead {
		return DeathScreen(termW, termH, me.KilledBy)
	}
	blocked := map[uint64]struct{}{}
	for _, w := range snap.Walls {
		blocked[wallKey(w.X, w.Y)] = struct{}{}
	}
	viewW := clamp(termW, 70, 220)
	viewH := clamp(termH-4, 24, 70)
	scene := make([][]rune, viewH)
	colors := make([][]uint32, viewH)
	// Тёмный фон; стены и пол только из меша (buildMeshFromSnapshot + rasterMeshScene).
	for y := range scene {
		scene[y] = make([]rune, viewW)
		colors[y] = make([]uint32, viewW)
		for x := range scene[y] {
			scene[y][x] = ' '
			colors[y][x] = meshBackdropColor(y, viewH)
		}
	}
	// Горизонтальный FOV: было 60° (π/3), +20° → 80°.
	fov := math.Pi/3.0 + 20.0*math.Pi/180.0
	distAtCol := make([]float64, viewW)
	for i := range distAtCol {
		// Колонки без геометрии не должны давать depth=0 (иначе спрайты отсекаются).
		distAtCol[i] = 1e9
	}
	tris := buildMeshFromSnapshot(snap, blocked)
	cam := Vec3{X: me.X, Y: meshEyeHeight, Z: me.Y}
	rasterMeshScene(scene, colors, tris, cam, me.Angle, fov, viewW, viewH, distAtCol)

	drawOtherPlayers(scene, colors, distAtCol, me, snap, playerID, viewW, viewH, fov, gfx, hud.NowUnixNano)

	// Подсветка мира от чужих выстрелов отложена (см. applyOpponentEnvFlash / opponentEnvFlashStrength).

	drawWeaponHUD(scene, colors, hud, gfx)
	drawShotEffects(scene, colors, hud, gfx)
	drawDamageVignette(colors, viewW, viewH, hud)
	drawHitConfirmCrosshair(scene, colors, viewW, viewH, me.HitConfirmAgeMs)
	rightReserve := 0
	if viewW >= leaderboardRightCols+36 {
		rightReserve = leaderboardRightCols
	}
	drawKillFeed(scene, colors, snap.KillFeed, viewW, viewH, rightReserve)
	drawLeaderboardHUD(scene, colors, snap, playerID, viewW, viewH)
	hpHud, armHud := me.HP, me.Armor
	// Подстраховка: компактный state без players в JSON давал hp=0.
	if hpHud <= 0 && !me.Dead {
		hpHud = 100
	}
	if armHud < 0 {
		armHud = 0
	}
	drawPlayerStatsHUD(scene, colors, hpHud, armHud, hud.Weapon, me.PistolMag, me.RifleMag, me.RifleReserve, viewW, viewH)
	drawMoneyGainFlash(scene, colors, hud, viewW, viewH)
	drawMoneyHUD(scene, colors, me.Money, viewW, viewH)
	drawMinimap(scene, colors, blocked, snap.Width, snap.Height, me, viewW, viewH, snap.Players, playerID)
	if hud.BuyMenuOpen {
		drawBuyMenuOverlay(scene, colors, viewW, viewH)
	}
	if hud.ScoreboardOpen {
		drawScoreboardOverlay(scene, colors, snap, playerID, hud, viewW, viewH)
	}

	var b strings.Builder
	b.Grow(viewW * viewH * 24)
	b.WriteString("\x1b[H")
	b.WriteString(fmt.Sprintf("DOOM SSH ARENA FPS | map=%s room=%s\n", snap.MapTitle, snap.RoomID))
	b.WriteString("Controls: WASD, SPACE, R, 1/2, B shop, Tab score, Esc close overlays, q quit\n\n")
	writeColoredScene(&b, scene, colors)
	b.WriteString("\x1b[J")
	return b.String()
}

// asciiFogDepthRamp — как в Asciipocalypse (Rasterizer fogString): ближе к камере плотнее символ.
// Репозиторий: https://github.com/wonrzrzeczny/Asciipocalypse
var asciiFogDepthRamp = []rune("@&#8x*,:. ")

// meshBackdropColor — фон до отрисовки меша: ровная тёмная пустота (без «пола-заставки»).
func meshBackdropColor(y, viewH int) uint32 {
	if viewH < 2 {
		return RGBPacked(5, 6, 10)
	}
	t := float64(y) / float64(viewH-1)
	r := byte(5 + (1-t)*12)
	g := byte(6 + (1-t)*14)
	b := byte(10 + (1-t)*22)
	return RGBPacked(r, g, b)
}

// depthGlyphFromLumJitter — маппинг яркости на asciiFogDepthRamp; ближе = плотнее (@&#…), дальше — . и пробел.
// jitter имитирует offset[i,j] в Asciipocalypse Rasterizer (дизеринг по глубине).
func depthGlyphFromLumJitter(lum int, col, screenY int) rune {
	if lum < 0 {
		lum = 0
	}
	if lum > 255 {
		lum = 255
	}
	ramp := asciiFogDepthRamp
	n := len(ramp)
	if n < 2 {
		return '@'
	}
	j := (fractPhase(float64(col)*0.173+float64(screenY)*0.251) - 0.5) * 1.15
	inv := float64(255-lum) + j
	if inv < 0 {
		inv = 0
	}
	if inv > 255 {
		inv = 255
	}
	t := inv / 255.0 * float64(n-1)
	idx := int(t + 0.5)
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		idx = n - 1
	}
	return ramp[idx]
}

func drawWeaponHUD(scene [][]rune, colors [][]uint32, hud GunHUDState, gfx *WadGraphics) {
	w := hud.Weapon
	if w == 0 {
		w = HUDWeaponPistol
	}
	if w == HUDWeaponRifle {
		drawRifleHUD(scene, colors, hud)
		return
	}
	drawPistol(scene, colors, hud)
}

// rifleHUDAnchor — позиция ASCII-автомата на экране (совпадает с drawRifleHUD).
func rifleHUDAnchor(hud GunHUDState, screenW, screenH int) (startX, yBase, cols, rows int, ok bool) {
	frames := BuiltinRifleHUDFrames
	if len(frames) == 0 {
		return 0, 0, 0, 0, false
	}
	fi := pistolFrameFromHUD(hud, len(frames))
	if rifleReloadAnimActive(hud) {
		fi = 0
	}
	fr := frames[fi]
	rows = len(fr.Chars)
	if rows == 0 || len(fr.Chars[0]) == 0 {
		return 0, 0, 0, 0, false
	}
	cols = len(fr.Chars[0])
	center := screenW / 2
	yBase = screenH - rows - 1
	if yBase < 0 {
		yBase = 0
	}
	bobY, bobX := walkBobOffsets(hud)
	turnY, turnX := turnSwayOffsets(hud)
	var rkx, rky int
	if rifleReloadAnimActive(hud) {
		rkx, rky = RifleReloadAnchorOffset(hud.NowUnixNano - hud.ReloadStartUnixNano)
	} else {
		rkx, rky = rifleFireRecoil(fi)
	}
	yBase += bobY + turnY + rky
	yBase += 6
	startX = center + screenW/8 - cols/2 + bobX + turnX + rkx + 4
	if startX < 0 {
		startX = 0
	}
	if startX+cols > screenW {
		startX = screenW - cols
		if startX < 0 {
			startX = 0
		}
	}
	return startX, yBase, cols, rows, true
}

func drawRifleHUD(scene [][]rune, colors [][]uint32, hud GunHUDState) {
	h := len(scene)
	if h == 0 {
		return
	}
	w := len(scene[0])
	frames := BuiltinRifleHUDFrames
	if len(frames) == 0 {
		return
	}
	startX, yBase, cols, rows, ok := rifleHUDAnchor(hud, w, h)
	if !ok {
		return
	}
	fi := pistolFrameFromHUD(hud, len(frames))
	if rifleReloadAnimActive(hud) {
		fi = 0
	}
	fr := frames[fi]
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
}

// pistolHUDAnchor — позиция встроенного HUD-пистолета (кадры A–D), для гильзы; совпадает с drawPistol.
func pistolHUDAnchor(hud GunHUDState, screenW, screenH int) (startX, yBase, cols, rows int, ok bool) {
	frames := BuiltinPistolHUDFrames
	if len(frames) == 0 {
		return 0, 0, 0, 0, false
	}
	h := screenH
	fi := pistolHUDFrameFromHUD(hud, len(frames))
	if pistolReloadAnimActive(hud) {
		fi = 0
	}
	fr := frames[fi]
	rows = len(fr.Chars)
	if rows == 0 || len(fr.Chars[0]) == 0 {
		return 0, 0, 0, 0, false
	}
	cols = len(fr.Chars[0])
	// Ниже стандартной привязки к низу на 9 строк; по X — между центром и правым краем.
	const pistolRightMargin = 2
	yBase = h - rows - 1 + 9
	if yBase < 0 {
		yBase = 0
	}
	bobY, bobX := walkBobOffsets(hud)
	turnY, turnX := turnSwayOffsets(hud)
	var prx, pry int
	if pistolReloadAnimActive(hud) {
		prx, pry = PistolReloadAnchorOffset(hud.NowUnixNano - hud.ReloadStartUnixNano)
	} else {
		prx, pry = pistolFireRecoil(fi)
	}
	yBase += bobY + turnY + pry
	centerX := screenW/2 - cols/2 - 2
	rightX := screenW - cols - pistolRightMargin
	// ~11/16 смещения от правого якоря к центральному (ближе к центру, чем у правого края).
	const num, den = 11, 16
	startX = rightX + (centerX-rightX)*num/den
	startX += bobX + turnX + prx
	if startX < 0 {
		startX = 0
	}
	if startX+cols > screenW {
		startX = screenW - cols
		if startX < 0 {
			startX = 0
		}
	}
	return startX, yBase, cols, rows, true
}

func drawPistol(scene [][]rune, colors [][]uint32, hud GunHUDState) {
	h := len(scene)
	if h == 0 {
		return
	}
	w := len(scene[0])
	frames := BuiltinPistolHUDFrames
	if len(frames) == 0 {
		return
	}
	startX, yBase, cols, rows, ok := pistolHUDAnchor(hud, w, h)
	if !ok {
		return
	}
	fi := pistolHUDFrameFromHUD(hud, len(frames))
	if pistolReloadAnimActive(hud) {
		fi = 0
	}
	fr := frames[fi]
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
}

// drawShotEffects — без линии-трассы: вспышка + гильзы (автомат и пистолет).
func drawShotEffects(scene [][]rune, colors [][]uint32, hud GunHUDState, gfx *WadGraphics) {
	drawMuzzleFlashCenter(scene, colors, hud)
	drawRifleShellEject(scene, colors, hud)
	drawPistolShellEject(scene, colors, hud)
}

// drawMuzzleFlashCenter — мерцание у прицела (центр экрана), без вертикальной трассы.
func drawMuzzleFlashCenter(scene [][]rune, colors [][]uint32, hud GunHUDState) {
	if !showMuzzleFlash(hud) {
		return
	}
	h := len(scene)
	if h == 0 {
		return
	}
	w := len(scene[0])
	cx := w / 2
	const flashDrop = 10
	startY := h/2 + 2 + flashDrop
	weapon := hud.Weapon
	if weapon == 0 {
		weapon = HUDWeaponPistol
	}
	if weapon == HUDWeaponRifle {
		startY -= 12 // автомат: выше к прицелу
	} else if weapon == HUDWeaponPistol {
		startY -= 8 // пистолет: ещё выше
	}
	if startY < 0 {
		startY = 0
	}
	elapsed := hud.NowUnixNano - hud.FireStartUnixNano
	// Лёгкое мерцание яркости за ~70 ms
	pulse := int((elapsed / 11e6) % 3)
	br := 255 - pulse*12
	if br < 195 {
		br = 195
	}
	bg := br - 35
	bb := br - 175
	if bg < 120 {
		bg = 120
	}
	if bb < 40 {
		bb = 40
	}
	col := RGBPacked(byte(br), byte(bg), byte(bb))
	// Компактнее вспышка: 3×2 вместо 5×3
	for dy := 0; dy < 2; dy++ {
		y := startY + dy
		if y >= 0 && y < h {
			for dx := -1; dx <= 1; dx++ {
				x := cx + dx
				if x >= 0 && x < w {
					scene[y][x] = '*'
					colors[y][x] = col
				}
			}
		}
	}
}

// drawRifleShellEject — гильза вылетает сбоку от модели автомата вправо и вниз.
func drawRifleShellEject(scene [][]rune, colors [][]uint32, hud GunHUDState) {
	weapon := hud.Weapon
	if weapon == 0 {
		weapon = HUDWeaponPistol
	}
	if weapon != HUDWeaponRifle {
		return
	}
	if rifleReloadAnimActive(hud) {
		return
	}
	if hud.FireStartUnixNano == 0 {
		return
	}
	el := hud.NowUnixNano - hud.FireStartUnixNano
	if el < 25e6 || el > 400e6 {
		return
	}
	sw, sh := len(scene[0]), len(scene)
	sx, yBase, cols, rows, ok := rifleHUDAnchor(hud, sw, sh)
	if !ok {
		return
	}
	t := float64(el-25e6) / 1e9
	// Точка вылета на 6 символов левее правого края — вправо и по дуге вниз
	ejectX := float64(sx + cols - 1 - 6)
	ejectY := float64(yBase + rows/3)
	vx := 26.0
	g := 38.0
	shellX := int(ejectX + vx*t + 5*t*t)
	shellY := int(ejectY - 3*t*4 + g*t*t)
	if shellY < 0 || shellY+1 >= sh || shellX < 0 || shellX+1 >= sw {
		return
	}
	tum := int((el / 18e6) % 5)
	chars := []rune{':', '\'', ',', '\u00b7', '.'}
	ch0 := chars[tum]
	ch1 := chars[(tum+2)%len(chars)]
	brass := RGBPacked(210, 165, 75)
	brassDark := RGBPacked(175, 130, 55)
	// Гильза чуть крупнее: блок 2×2
	scene[shellY][shellX] = ch0
	colors[shellY][shellX] = brass
	scene[shellY][shellX+1] = ch1
	colors[shellY][shellX+1] = brassDark
	scene[shellY+1][shellX] = ch1
	colors[shellY+1][shellX] = brassDark
	scene[shellY+1][shellX+1] = ch0
	colors[shellY+1][shellX+1] = brass
}

// drawPistolShellEject — гильза с правого края пистолета (ближе к модели, чем у автомата).
func drawPistolShellEject(scene [][]rune, colors [][]uint32, hud GunHUDState) {
	weapon := hud.Weapon
	if weapon == 0 {
		weapon = HUDWeaponPistol
	}
	if weapon != HUDWeaponPistol {
		return
	}
	if pistolReloadAnimActive(hud) {
		return
	}
	if hud.FireStartUnixNano == 0 {
		return
	}
	el := hud.NowUnixNano - hud.FireStartUnixNano
	if el < 25e6 || el > 400e6 {
		return
	}
	sw, sh := len(scene[0]), len(scene)
	sx, yBase, cols, rows, ok := pistolHUDAnchor(hud, sw, sh)
	if !ok {
		return
	}
	t := float64(el-25e6) / 1e9
	// Правый край bbox: sx+cols−1; раньше вылет на −1, ещё на 6 символов ближе к модели ⇒ −7 от края.
	const pistolShellInsetFromRight = 14
	ejectX := float64(sx + cols - 1 - pistolShellInsetFromRight)
	ejectY := float64(yBase + rows/3)
	vx := 14.0
	g := 28.0
	shellX := int(ejectX + vx*t + 3*t*t)
	shellY := int(ejectY - 2*t*3 + g*t*t)
	if shellY < 0 || shellY+1 >= sh || shellX < 0 || shellX+1 >= sw {
		return
	}
	tum := int((el / 18e6) % 5)
	chars := []rune{':', '\'', ',', '\u00b7', '.'}
	ch0 := chars[tum]
	ch1 := chars[(tum+2)%len(chars)]
	brass := RGBPacked(210, 165, 75)
	brassDark := RGBPacked(175, 130, 55)
	scene[shellY][shellX] = ch0
	colors[shellY][shellX] = brass
	scene[shellY][shellX+1] = ch1
	colors[shellY][shellX+1] = brassDark
	scene[shellY+1][shellX] = ch1
	colors[shellY+1][shellX] = brassDark
	scene[shellY+1][shellX+1] = ch0
	colors[shellY+1][shellX+1] = brass
}

// DeathScreen — полноэкранная заставка в стиле SSH-меню: тёмно-красный фон, рамка, подсказки.
func DeathScreen(termW, termH int, killedBy string) string {
	killedBy = deathScreenSanitizeKiller(killedBy)
	if killedBy == "" {
		killedBy = "неизвестно"
	}
	tw := clamp(termW, 40, 200)
	th := clamp(termH, 14, 80)

	box := []string{
		"  ╔══════════════════════════════════════════════════════════╗",
		"  ║                                                          ║",
		"  ║                    В  Ы   П О Г И Б Л И                    ║",
		"  ║                                                          ║",
		"  ╚══════════════════════════════════════════════════════════╝",
	}
	killLine := "  Убийца: " + killedBy
	hintLine := "  [r] возродиться          [q] выйти из сервера"
	innerW := 0
	for _, ln := range box {
		if w := utf8.RuneCountInString(ln); w > innerW {
			innerW = w
		}
	}
	if w := utf8.RuneCountInString(killLine); w > innerW {
		innerW = w
	}
	if w := utf8.RuneCountInString(hintLine); w > innerW {
		innerW = w
	}
	boxW := innerW
	marg := (tw - boxW) / 2
	if marg < 0 {
		marg = 0
	}
	ind := strings.Repeat(" ", marg)

	contentLines := 2 + len(box) + 3 // фон×2, рамка, убийца, пустая строка, подсказка
	topM := (th - contentLines) / 2
	if topM < 0 {
		topM = 0
	}

	var b strings.Builder
	b.Grow(tw * th / 2)

	bgLine := func(depth int) {
		r := 32 + depth*4
		if r > 72 {
			r = 72
		}
		b.WriteString(fmt.Sprintf("\x1b[48;2;%d;0;0m", r))
		b.WriteString(strings.Repeat(" ", tw))
		b.WriteString("\x1b[0m\n")
	}
	b.WriteString("\x1b[2J\x1b[H")
	for i := 0; i < topM; i++ {
		bgLine(i % 3)
	}
	for i := 0; i < 2; i++ {
		bgLine(i)
	}
	for _, ln := range box {
		b.WriteString(fmt.Sprintf("\x1b[48;2;40;0;0m%s", ind))
		b.WriteString(deathScreenRGB(255, 65, 65, ln))
		pad := boxW - utf8.RuneCountInString(ln)
		if pad > 0 {
			b.WriteString(strings.Repeat(" ", pad))
		}
		b.WriteString("\x1b[0m\n")
	}
	b.WriteString(fmt.Sprintf("\x1b[48;2;40;0;0m%s", ind))
	b.WriteString(deathScreenRGB(255, 185, 185, killLine))
	padK := boxW - utf8.RuneCountInString(killLine)
	if padK > 0 {
		b.WriteString(strings.Repeat(" ", padK))
	}
	b.WriteString("\x1b[0m\n")
	b.WriteString(fmt.Sprintf("\x1b[48;2;40;0;0m%s\n", ind+strings.Repeat(" ", boxW)))
	b.WriteString(fmt.Sprintf("\x1b[48;2;40;0;0m%s", ind))
	b.WriteString(deathScreenRGB(255, 210, 140, hintLine))
	padH := boxW - utf8.RuneCountInString(hintLine)
	if padH > 0 {
		b.WriteString(strings.Repeat(" ", padH))
	}
	b.WriteString("\x1b[0m\n")
	rest := th - topM - contentLines
	if rest < 0 {
		rest = 0
	}
	for i := 0; i < rest; i++ {
		bgLine((i + topM) % 4)
	}
	b.WriteString("\x1b[0m\x1b[J")
	return b.String()
}

func deathScreenRGB(r, g, b int, s string) string {
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm%s\x1b[0m", r, g, b, s)
}

func deathScreenSanitizeKiller(s string) string {
	var out strings.Builder
	for _, r := range s {
		if r == '\x1b' {
			continue
		}
		if r < 32 && r != ' ' {
			continue
		}
		out.WriteRune(r)
		if out.Len() >= 56 {
			break
		}
	}
	return strings.TrimSpace(out.String())
}

func padRuneWidth(s string, w int) string {
	rs := []rune(s)
	if len(rs) >= w {
		return string(rs[:w])
	}
	return s + strings.Repeat(" ", w-len(rs))
}

// drawPlayerStatsHUD — слева снизу: строка выше — боезапас; снизу — HP (красный), ARM (синий).
func drawPlayerStatsHUD(scene [][]rune, colors [][]uint32, hp, armor, weapon, pistolMag, rifleMag, rifleReserve, viewW, viewH int) {
	h := len(scene)
	if h < 1 || viewW < 1 {
		return
	}
	gray := RGBPacked(180, 180, 180)
	red := RGBPacked(220, 65, 65)
	blue := RGBPacked(70, 140, 255)
	gold := RGBPacked(230, 190, 90)
	putRow := func(y, startX int, label string, valueRunes string, valueCol uint32) {
		x := startX
		put := func(r rune, c uint32) {
			if x >= 0 && x < viewW && y >= 0 && y < h {
				scene[y][x] = r
				colors[y][x] = c
			}
			x++
		}
		for _, r := range label {
			put(r, gray)
		}
		for _, r := range valueRunes {
			put(r, valueCol)
		}
	}
	if h >= 3 {
		w := weapon
		if w == 0 {
			w = HUDWeaponPistol
		}
		if w == HUDWeaponRifle {
			putRow(h-2, 1, "AMMO ", strconv.Itoa(rifleMag)+"|"+strconv.Itoa(rifleReserve), gold)
		} else {
			putRow(h-2, 1, "AMMO ", strconv.Itoa(pistolMag)+"|\u221e", gold)
		}
	}
	y := h - 1
	x := 1
	put := func(r rune, c uint32) {
		if x >= 0 && x < viewW && y >= 0 && y < h {
			scene[y][x] = r
			colors[y][x] = c
		}
		x++
	}
	for _, r := range "HP " {
		put(r, gray)
	}
	for _, r := range strconv.Itoa(hp) {
		put(r, red)
	}
	for _, r := range "  ARM " {
		put(r, gray)
	}
	for _, r := range strconv.Itoa(armor) {
		put(r, blue)
	}
}

// drawMoneyGainFlash — над строкой D: жёлтое мерцание суммы пополнения (~1 с).
func drawMoneyGainFlash(scene [][]rune, colors [][]uint32, hud GunHUDState, viewW, viewH int) {
	if hud.MoneyGainAmount <= 0 || hud.MoneyGainFlashUntilUnixNano == 0 {
		return
	}
	if hud.NowUnixNano == 0 || hud.NowUnixNano >= hud.MoneyGainFlashUntilUnixNano {
		return
	}
	h := len(scene)
	if h < 5 || viewW < 4 {
		return
	}
	y := h - 4
	if y < 0 {
		return
	}
	s := "+" + strconv.Itoa(hud.MoneyGainAmount)
	bright := RGBPacked(255, 235, 70)
	dim := RGBPacked(195, 155, 35)
	phase := (hud.NowUnixNano / 100_000_000) % 2
	col := bright
	if phase == 1 {
		col = dim
	}
	x := 1
	for _, r := range s {
		if x >= viewW {
			break
		}
		scene[y][x] = r
		colors[y][x] = col
		x++
	}
}

// drawMoneyHUD — слева над строкой боезапаса: валюта D (как в CS).
func drawMoneyHUD(scene [][]rune, colors [][]uint32, money, viewW, viewH int) {
	h := len(scene)
	if h < 4 || viewW < 6 {
		return
	}
	y := h - 3
	if y < 0 {
		return
	}
	gray := RGBPacked(160, 160, 170)
	green := RGBPacked(90, 220, 130)
	label := "D "
	val := strconv.Itoa(money)
	x := 1
	for _, r := range label {
		if x >= viewW {
			return
		}
		scene[y][x] = r
		colors[y][x] = gray
		x++
	}
	for _, r := range val {
		if x >= viewW {
			return
		}
		scene[y][x] = r
		colors[y][x] = green
		x++
	}
}

// drawBuyMenuOverlay — меню покупок по B (1 — патроны, 2 — броня).
func drawBuyMenuOverlay(scene [][]rune, colors [][]uint32, viewW, viewH int) {
	h := len(scene)
	if h < 7 || viewW < 24 {
		return
	}
	lines := []string{
		" МАГАЗИН · [B] закрыть ",
		" [1] +30 патронов в запас (автомат) — 50 D · макс. 120 ",
		" [2] Броня +100 (макс. 100) — 100 D ",
	}
	boxW := 4
	for _, s := range lines {
		if n := utf8.RuneCountInString(s); n+2 > boxW {
			boxW = n + 2
		}
	}
	if boxW > viewW-2 {
		boxW = viewW - 2
	}
	if boxW < 12 {
		return
	}
	x0 := (viewW - boxW) / 2
	if x0 < 0 {
		x0 = 0
	}
	y0 := h/2 - 2
	if y0 < 1 {
		y0 = 1
	}
	bg := RGBPacked(18, 22, 38)
	border := RGBPacked(100, 160, 240)
	text := RGBPacked(220, 230, 245)
	dim := RGBPacked(140, 180, 220)
	boxH := len(lines) + 2
	for yy := 0; yy < boxH; yy++ {
		row := y0 + yy
		if row >= h {
			break
		}
		for xx := 0; xx < boxW; xx++ {
			col := x0 + xx
			if col >= viewW {
				break
			}
			scene[row][col] = ' '
			colors[row][col] = bg
		}
	}
	putBorder := func(row, col int, r rune, c uint32) {
		if row >= 0 && row < h && col >= 0 && col < viewW {
			scene[row][col] = r
			colors[row][col] = c
		}
	}
	for x := 0; x < boxW; x++ {
		putBorder(y0, x0+x, '-', border)
		putBorder(y0+boxH-1, x0+x, '-', border)
	}
	for y := 0; y < boxH; y++ {
		putBorder(y0+y, x0, '|', border)
		putBorder(y0+y, x0+boxW-1, '|', border)
	}
	putBorder(y0, x0, '+', border)
	putBorder(y0, x0+boxW-1, '+', border)
	putBorder(y0+boxH-1, x0, '+', border)
	putBorder(y0+boxH-1, x0+boxW-1, '+', border)
	for i, line := range lines {
		rs := []rune(line)
		if len(rs) > boxW-2 {
			rs = rs[:boxW-2]
		}
		row := y0 + 1 + i
		if row >= h {
			break
		}
		tc := text
		if i > 0 {
			tc = dim
		}
		for j, r := range rs {
			col := x0 + 1 + j
			if col >= viewW {
				break
			}
			scene[row][col] = r
			colors[row][col] = tc
		}
	}
}

func scoreboardPingCell(pl protocol.PlayerState, selfID string, hud GunHUDState) string {
	if pl.ID == selfID {
		if hud.PingRTTMs > 0 {
			return strconv.Itoa(hud.PingRTTMs)
		}
		if pl.PingMs > 0 {
			return strconv.Itoa(pl.PingMs)
		}
		if hud.StateLagMs > 0 {
			return "~" + strconv.Itoa(hud.StateLagMs)
		}
		return "~0"
	}
	if pl.PingMs > 0 {
		return strconv.Itoa(pl.PingMs)
	}
	return "?"
}

// drawScoreboardOverlay — Tab: игроки, K/D, деньги, пинг (мс) у каждого; свой — по RTT до room.
func drawScoreboardOverlay(scene [][]rune, colors [][]uint32, snap *protocol.RoomSnapshot, selfID string, hud GunHUDState, viewW, viewH int) {
	h := len(scene)
	if h < 10 || viewW < 36 || snap == nil || len(snap.Players) == 0 {
		return
	}
	pls := append([]protocol.PlayerState(nil), snap.Players...)
	sort.Slice(pls, func(i, j int) bool {
		if pls[i].Kills != pls[j].Kills {
			return pls[i].Kills > pls[j].Kills
		}
		return pls[i].Name < pls[j].Name
	})
	lines := []string{
		" ИГРОКИ · [Tab] закрыть ",
		" Имя           уб смр   D  мс ",
	}
	for _, pl := range pls {
		nm := pl.Name
		if utf8.RuneCountInString(nm) > 11 {
			rs := []rune(nm)
			nm = string(rs[:11]) + "."
		}
		pingStr := scoreboardPingCell(pl, selfID, hud)
		lines = append(lines, fmt.Sprintf(" %-11s %3d %3d %4d %5s", nm, pl.Kills, pl.Deaths, pl.Money, pingStr))
	}
	boxW := 4
	for _, s := range lines {
		if n := utf8.RuneCountInString(s); n+2 > boxW {
			boxW = n + 2
		}
	}
	if boxW > viewW-2 {
		boxW = viewW - 2
	}
	// Не требовать минимум 40: при русских заголовках и компактных строках boxW бывает 35–39 — иначе оверлей не рисовался вообще.
	if boxW < 8 {
		return
	}
	x0 := (viewW - boxW) / 2
	if x0 < 0 {
		x0 = 0
	}
	y0 := 1
	if h-len(lines)-4 < 2 {
		y0 = 0
	}
	bg := RGBPacked(16, 20, 32)
	border := RGBPacked(90, 200, 140)
	text := RGBPacked(220, 235, 245)
	dim := RGBPacked(150, 175, 200)
	selfCol := RGBPacked(255, 220, 120)
	boxH := len(lines) + 2
	for yy := 0; yy < boxH; yy++ {
		row := y0 + yy
		if row >= h {
			break
		}
		for xx := 0; xx < boxW; xx++ {
			col := x0 + xx
			if col >= viewW {
				break
			}
			scene[row][col] = ' '
			colors[row][col] = bg
		}
	}
	putB := func(row, col int, r rune, c uint32) {
		if row >= 0 && row < h && col >= 0 && col < viewW {
			scene[row][col] = r
			colors[row][col] = c
		}
	}
	for x := 0; x < boxW; x++ {
		putB(y0, x0+x, '-', border)
		putB(y0+boxH-1, x0+x, '-', border)
	}
	for y := 0; y < boxH; y++ {
		putB(y0+y, x0, '|', border)
		putB(y0+y, x0+boxW-1, '|', border)
	}
	putB(y0, x0, '+', border)
	putB(y0, x0+boxW-1, '+', border)
	putB(y0+boxH-1, x0, '+', border)
	putB(y0+boxH-1, x0+boxW-1, '+', border)
	for i, line := range lines {
		rs := []rune(line)
		if len(rs) > boxW-2 {
			rs = rs[:boxW-2]
		}
		row := y0 + 1 + i
		if row >= h {
			break
		}
		tc := text
		if i == 1 {
			tc = dim
		}
		if i >= 2 {
			pi := i - 2
			if pi < len(pls) && pls[pi].ID == selfID {
				tc = selfCol
			} else {
				tc = dim
			}
		}
		for j, r := range rs {
			col := x0 + 1 + j
			if col >= viewW {
				break
			}
			scene[row][col] = r
			colors[row][col] = tc
		}
	}
}

// minimapDirRune — стрелка направления взгляда. Угол как в stepMove (cos→X, sin→Y); на миникарте Y растёт вниз,
// поэтому вызывают с -angle, чтобы ↑/↓ совпали с движением по карте (влево/вправо без инверсии).
func minimapDirRune(angle float64) rune {
	a := angle
	for a < 0 {
		a += 2 * math.Pi
	}
	for a >= 2*math.Pi {
		a -= 2 * math.Pi
	}
	ix := int((a + math.Pi/8) / (math.Pi / 4))
	if ix >= 8 {
		ix -= 8
	}
	if ix < 0 {
		ix = 0
	}
	arrows := []rune{'→', '↗', '↑', '↖', '←', '↙', '↓', '↘'}
	return arrows[ix]
}

// drawMinimap — справа снизу: рамка, стены, проходы; противники — оранжевый маркер; зелёная стрелка — вы.
func drawMinimap(scene [][]rune, colors [][]uint32, blocked map[uint64]struct{}, mapW, mapH int, me protocol.PlayerState, viewW, viewH int, players []protocol.PlayerState, selfID string) {
	if viewW < 28 || viewH < 16 || mapW < 2 || mapH < 2 {
		return
	}
	// Внутренние размеры сетки = размер редактора (jw×jh), не rw×rh (у них разное соотношение сторон при padding).
	innerMapW := mapW - 2
	innerMapH := mapH - 2
	if innerMapW < 1 || innerMapH < 1 {
		return
	}

	// По горизонтали: один символ миникарты на колонку редактора (jw), пока влезает в ~18 столбцов.
	// Раньше было innerW=18 при innerMapW=16: формулы ii↔gx и маркер игрока не совпадали (целочисленное деление).
	innerW := innerMapW
	if innerW > 18 {
		innerW = 18
	}
	innerH := (innerW * innerMapH) / innerMapW
	if innerH < 6 {
		innerH = 6
	}
	if innerH > 12 {
		innerH = 12
	}
	mw := innerW + 2
	mh := innerH + 2
	if mw+4 > viewW {
		innerW = viewW - 6
		if innerW < 10 {
			return
		}
		mw = innerW + 2
		innerH = (innerW * innerMapH) / innerMapW
		if innerH < 6 {
			innerH = 6
		}
		if innerH > 12 {
			innerH = 12
		}
		mh = innerH + 2
	}
	if topY := viewH - 2 - mh; topY < 2 {
		return
	}
	startX := viewW - mw - 2
	topY := viewH - 2 - mh
	if startX < 0 {
		return
	}

	// Клетки редактора (Wolf3D) попадают в движок как (jw+2)×(jh+2): внешнее кольцо — void, не в blocked.
	// Рисуем только внутреннюю область 1..mapW-2, чтобы миникарта совпадала с сеткой редактора, а не с padding.

	frame := RGBPacked(140, 150, 165)
	wallCol := RGBPacked(95, 98, 115)
	floorCol := RGBPacked(55, 62, 72)
	playerCol := RGBPacked(100, 255, 130)
	otherCol := RGBPacked(255, 175, 70)

	for j := 0; j < mh; j++ {
		for i := 0; i < mw; i++ {
			x := startX + i
			y := topY + j
			if x < 0 || x >= viewW || y < 0 || y >= len(scene) {
				continue
			}
			var ch rune
			var col uint32
			switch {
			case j == 0 && i == 0:
				ch = '+'
				col = frame
			case j == 0 && i == mw-1:
				ch = '+'
				col = frame
			case j == mh-1 && i == 0:
				ch = '+'
				col = frame
			case j == mh-1 && i == mw-1:
				ch = '+'
				col = frame
			case j == 0 || j == mh-1:
				ch = '-'
				col = frame
			case i == 0 || i == mw-1:
				ch = '|'
				col = frame
			default:
				ii := i - 1
				jj := j - 1
				gx := 1 + ii*innerMapW/innerW
				gy := 1 + jj*innerMapH/innerH
				if gx >= mapW-1 {
					gx = mapW - 2
				}
				if gy >= mapH-1 {
					gy = mapH - 2
				}
				if _, ok := blocked[wallKey(gx, gy)]; ok {
					// '#' — одна колонка в терминале; U+2588 FULL BLOCK часто «широкий» и ломает выравнивание HUD.
					ch = '#'
					col = wallCol
				} else {
					ch = '·'
					col = floorCol
				}
			}
			scene[y][x] = ch
			colors[y][x] = col
		}
	}

	putMinimapCell := func(worldX, worldY float64, ch rune, col uint32) {
		// Индекс клетки движка [1..mapW-2]: совпадает с сеткой редактора (jx = gx-1), без сдвига по (world-1).
		gx := int(math.Floor(worldX + 1e-6))
		gy := int(math.Floor(worldY + 1e-6))
		if gx < 1 {
			gx = 1
		}
		if gx > mapW-2 {
			gx = mapW - 2
		}
		if gy < 1 {
			gy = 1
		}
		if gy > mapH-2 {
			gy = mapH - 2
		}
		editorJx := gx - 1
		editorJy := gy - 1
		pi := 1 + editorJx*innerW/innerMapW
		pj := 1 + editorJy*innerH/innerMapH
		if pi < 1 {
			pi = 1
		}
		if pj < 1 {
			pj = 1
		}
		if pi > mw-2 {
			pi = mw - 2
		}
		if pj > mh-2 {
			pj = mh - 2
		}
		px := startX + pi
		py := topY + pj
		if px >= 0 && px < viewW && py >= 0 && py < len(scene) {
			scene[py][px] = ch
			colors[py][px] = col
		}
	}
	for _, op := range players {
		if op.ID == selfID || op.Dead {
			continue
		}
		putMinimapCell(op.X, op.Y, 'o', otherCol)
	}
	putMinimapCell(me.X, me.Y, minimapDirRune(-me.Angle), playerCol)
}

// writeUint8 пишет десятичные цифры v (0..255) в buf, возвращает число записанных байт.
func writeUint8(buf []byte, v byte) int {
	n := int(v)
	if n >= 100 {
		buf[0] = byte('0' + n/100)
		buf[1] = byte('0' + (n/10)%10)
		buf[2] = byte('0' + n%10)
		return 3
	}
	if n >= 10 {
		buf[0] = byte('0' + n/10)
		buf[1] = byte('0' + n%10)
		return 2
	}
	buf[0] = byte('0' + n)
	return 1
}

// writeAnsiColor — \x1b[38;2;r;g;bm без fmt и без лишних аллокаций.
func writeAnsiColor(buf []byte, r, g, b byte) int {
	off := 0
	copy(buf[off:], "\x1b[38;2;")
	off += 7
	off += writeUint8(buf[off:], r)
	buf[off] = ';'
	off++
	off += writeUint8(buf[off:], g)
	buf[off] = ';'
	off++
	off += writeUint8(buf[off:], b)
	buf[off] = 'm'
	off++
	return off
}

func writeColoredScene(sb *strings.Builder, scene [][]rune, colors [][]uint32) {
	var esc [24]byte
	var lr, lg, lb byte
	var has bool
	for y := 0; y < len(scene); y++ {
		for x := 0; x < len(scene[y]); x++ {
			c := colors[y][x]
			r := byte(c >> 16)
			g := byte(c >> 8)
			b := byte(c)
			if !has || r != lr || g != lg || b != lb {
				n := writeAnsiColor(esc[:], r, g, b)
				sb.Write(esc[:n])
				lr, lg, lb = r, g, b
				has = true
			}
			sb.WriteRune(scene[y][x])
		}
		sb.WriteString("\x1b[0m\x1b[K\n")
		has = false
	}
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

// opponentEnvFlashStrength — насколько «подсветить» мир от выстрелов других игроков (0..1), ~1 с после выстрела.
func opponentEnvFlashStrength(snap *protocol.RoomSnapshot, selfID string) float64 {
	var maxS float64
	for _, pl := range snap.Players {
		if pl.ID == selfID || pl.Dead {
			continue
		}
		if pl.FireAgeMs > 0 && pl.FireAgeMs <= opponentFireFlashMs {
			s := 1.0 - float64(pl.FireAgeMs)/float64(opponentFireFlashMs)
			if s > maxS {
				maxS = s
			}
		}
	}
	return maxS
}

func applyOpponentEnvFlash(colors [][]uint32, strength float64) {
	if strength < 0.02 {
		return
	}
	t := strength * 0.42
	warm := RGBPacked(255, 235, 120)
	for y := range colors {
		for x := range colors[y] {
			colors[y][x] = blendTowardYellow(colors[y][x], warm, t)
		}
	}
}

const damageVignetteDurNano = int64(350_000_000)

func drawDamageVignette(colors [][]uint32, w, h int, hud GunHUDState) {
	if hud.DamageFlashUntilUnixNano == 0 {
		return
	}
	remain := hud.DamageFlashUntilUnixNano - hud.NowUnixNano
	if remain <= 0 {
		return
	}
	strength := float64(remain) / float64(damageVignetteDurNano)
	if strength > 1 {
		strength = 1
	}
	const edgeBand = 8
	red := RGBPacked(220, 25, 35)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			ed := edgeDist(x, y, w, h)
			if ed > edgeBand {
				continue
			}
			edgeFactor := 1.0 - float64(ed)/float64(edgeBand)
			t := strength * edgeFactor * 0.62
			colors[y][x] = blendTowardRedPacked(colors[y][x], red, t)
		}
	}
}

func edgeDist(x, y, w, h int) int {
	d := x
	if w-1-x < d {
		d = w - 1 - x
	}
	if y < d {
		d = y
	}
	if h-1-y < d {
		d = h - 1 - y
	}
	return d
}

func blendTowardRedPacked(base uint32, target uint32, t float64) uint32 {
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

const hitConfirmDisplayMs = 600

// drawHitConfirmCrosshair — короткий «плюс» в центре при попадании по игроку (сервер: hit_confirm_age_ms).
func drawHitConfirmCrosshair(scene [][]rune, colors [][]uint32, viewW, viewH, hitAgeMs int) {
	if hitAgeMs <= 0 || hitAgeMs > hitConfirmDisplayMs {
		return
	}
	strength := 1.0 - float64(hitAgeMs)/float64(hitConfirmDisplayMs)
	if strength < 0.05 {
		return
	}
	cx := viewW / 2
	cy := viewH / 2
	hot := RGBPacked(255, 230, 90)
	arms := []struct {
		dx, dy int
		ch     rune
	}{
		{0, -1, '|'}, {0, 1, '|'}, {-1, 0, '-'}, {1, 0, '-'}, {0, 0, '+'},
	}
	for _, a := range arms {
		x := cx + a.dx
		y := cy + a.dy
		if x < 0 || x >= viewW || y < 0 || y >= viewH {
			continue
		}
		scene[y][x] = a.ch
		colors[y][x] = blendTowardRedPacked(colors[y][x], hot, strength*0.92)
	}
}

const killFeedMaxLines = 5

// leaderboardRightCols — ширина полосы таблицы лидеров справа (не перекрывать киллфид).
const leaderboardRightCols = 22

// drawLeaderboardHUD — справа сверху: сортировка по убийствам (больше — выше), у лидера символ короны.
func drawLeaderboardHUD(scene [][]rune, colors [][]uint32, snap *protocol.RoomSnapshot, selfID string, viewW, viewH int) {
	if snap == nil || len(snap.Players) == 0 || viewW < leaderboardRightCols+8 || viewH < 3 {
		return
	}
	pls := append([]protocol.PlayerState(nil), snap.Players...)
	sort.Slice(pls, func(i, j int) bool {
		if pls[i].Kills != pls[j].Kills {
			return pls[i].Kills > pls[j].Kills
		}
		return pls[i].Name < pls[j].Name
	})
	x0 := viewW - leaderboardRightCols
	titleCol := RGBPacked(190, 205, 235)
	gold := RGBPacked(255, 215, 75)
	dim := RGBPacked(155, 175, 200)
	selfCol := RGBPacked(255, 225, 130)
	crown := '\u2654' // ♔

	hdr := []rune("TOP")
	hx := x0 + leaderboardRightCols - len(hdr)
	if hx < x0 {
		hx = x0
	}
	for i, r := range hdr {
		col := hx + i
		if col >= viewW || col < 0 {
			continue
		}
		scene[0][col] = r
		colors[0][col] = titleCol
	}

	max := len(pls)
	if max > 8 {
		max = 8
	}
	for idx := 0; idx < max; idx++ {
		pl := pls[idx]
		y := 1 + idx
		if y >= viewH {
			break
		}
		nm := pl.Name
		rs := []rune(nm)
		if len(rs) > 9 {
			nm = string(rs[:9]) + "."
		} else {
			nm = string(rs)
		}
		prefix := ' '
		if idx == 0 {
			prefix = crown
		}
		line := fmt.Sprintf("%c%-9s %3d", prefix, nm, pl.Kills)
		lr := []rune(line)
		if len(lr) > leaderboardRightCols {
			lr = lr[:leaderboardRightCols]
		}
		for xi, r := range lr {
			col := x0 + xi
			if col >= viewW || col < 0 {
				continue
			}
			scene[y][col] = r
			c := dim
			if idx == 0 {
				c = gold
			}
			if pl.ID == selfID {
				c = selfCol
			}
			colors[y][col] = c
		}
	}
}

func drawKillFeed(scene [][]rune, colors [][]uint32, feed []protocol.KillFeedEntry, viewW, viewH, rightReserve int) {
	if rightReserve < 0 {
		rightReserve = 0
	}
	if len(feed) == 0 || viewW < 12 || viewH < 3 {
		return
	}
	maxW := viewW - rightReserve
	if maxW < 1 {
		return
	}
	start := len(feed) - killFeedMaxLines
	if start < 0 {
		start = 0
	}
	y := 0
	for i := start; i < len(feed); i++ {
		line := feed[i].Killer + " > " + feed[i].Victim
		rs := []rune(line)
		if len(rs) > maxW {
			rs = rs[len(rs)-maxW:]
		}
		x0 := viewW - rightReserve - len(rs)
		if x0 < 0 {
			x0 = 0
		}
		for j, r := range rs {
			xx := x0 + j
			if xx >= viewW || y >= viewH {
				break
			}
			scene[y][xx] = r
			colors[y][xx] = RGBPacked(255, 245, 230)
		}
		y++
	}
}
