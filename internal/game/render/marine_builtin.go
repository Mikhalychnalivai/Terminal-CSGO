package render

// Встроенный морпех: как HUD-пистолет — глиф из lumaChar(яркость), не █ на всё тело.
// Семантические теги дают разные RGB и дельты яркости (свет/тень), силуэт уже́ и читаемый.

func rgbLum(c uint32) int {
	r := int(byte(c >> 16))
	g := int(byte(c >> 8))
	b := int(byte(c))
	return (r*299 + g*587 + b*114) / 1000
}

func marineGlyphForTag(tag rune) (rune, uint32) {
	// Буквенные теги (если есть); иначе — пользовательский ASCII-арт (= * # % …).
	switch tag {
	case ' ', '\t':
		return 0, 0
	case 'H': // блик шлема
		c := RGBPacked(232, 200, 150)
		return lumaChar(clamp(rgbLum(c)+38, 0, 255)), c
	case 'h': // шлем
		c := RGBPacked(198, 158, 105)
		return lumaChar(clamp(rgbLum(c), 0, 255)), c
	case 'm': // тень шлема
		c := RGBPacked(130, 95, 62)
		return lumaChar(clamp(rgbLum(c)-28, 0, 255)), c
	case 'V': // блик визора
		c := RGBPacked(90, 98, 118)
		return lumaChar(clamp(rgbLum(c)+25, 0, 255)), c
	case 'v': // визор
		c := RGBPacked(40, 44, 54)
		return lumaChar(clamp(rgbLum(c), 0, 255)), c
	case 'S': // кожа светлее
		c := RGBPacked(238, 198, 155)
		return lumaChar(clamp(rgbLum(c)+22, 0, 255)), c
	case 's': // кожа
		c := RGBPacked(212, 168, 128)
		return lumaChar(clamp(rgbLum(c), 0, 255)), c
	case 't': // кожа тень
		c := RGBPacked(165, 118, 85)
		return lumaChar(clamp(rgbLum(c)-22, 0, 255)), c
	case 'G': // броня блик
		c := RGBPacked(60, 220, 95)
		return lumaChar(clamp(rgbLum(c)+32, 0, 255)), c
	case 'g': // броня
		c := RGBPacked(0, 150, 48)
		return lumaChar(clamp(rgbLum(c), 0, 255)), c
	case 'd': // броня тень
		c := RGBPacked(0, 72, 26)
		return lumaChar(clamp(rgbLum(c)-18, 0, 255)), c
	case 'K': // ствол / металл
		c := RGBPacked(72, 72, 78)
		return lumaChar(clamp(rgbLum(c)+18, 0, 255)), c
	case 'k': // оружие
		c := RGBPacked(22, 22, 30)
		return lumaChar(clamp(rgbLum(c), 0, 255)), c
	case 'z': // оружие тень
		c := RGBPacked(12, 12, 16)
		return lumaChar(clamp(rgbLum(c)-22, 0, 255)), c
	case 'b': // сапог
		c := RGBPacked(92, 95, 105)
		return lumaChar(clamp(rgbLum(c), 0, 255)), c
	case 'n': // сапог тень
		c := RGBPacked(58, 60, 68)
		return lumaChar(clamp(rgbLum(c)-24, 0, 255)), c
	default:
		return marineGlyphFromAscii(tag)
	}
}

// Заполняется в init() из marine_user_art.go (4 колонки ASCII).
var (
	marineFrontLines    []string
	marineBackLines     []string
	marineSideLeftLines []string
	marineQuarter1Lines []string
	marineQuarter3Lines []string
)

func parseMarineLayout(lines []string) PistolHUDFrame {
	h := len(lines)
	if h == 0 {
		return PistolHUDFrame{}
	}
	rows := make([][]rune, h)
	maxW := 0
	for i, line := range lines {
		rows[i] = []rune(line)
		if len(rows[i]) > maxW {
			maxW = len(rows[i])
		}
	}
	fr := PistolHUDFrame{
		Chars: make([][]rune, h),
		RGB:   make([][]uint32, h),
	}
	for y := 0; y < h; y++ {
		fr.Chars[y] = make([]rune, maxW)
		fr.RGB[y] = make([]uint32, maxW)
		for x := 0; x < maxW; x++ {
			var ch rune = ' '
			if x < len(rows[y]) {
				ch = rows[y][x]
			}
			r, c := marineGlyphForTag(ch)
			fr.Chars[y][x] = r
			fr.RGB[y][x] = c
		}
	}
	return fr
}

func flipMarineFrameH(fr PistolHUDFrame) PistolHUDFrame {
	rows := len(fr.Chars)
	if rows == 0 {
		return fr
	}
	cols := len(fr.Chars[0])
	out := PistolHUDFrame{
		Chars: make([][]rune, rows),
		RGB:   make([][]uint32, rows),
	}
	for y := 0; y < rows; y++ {
		out.Chars[y] = make([]rune, cols)
		out.RGB[y] = make([]uint32, cols)
		for x := 0; x < cols; x++ {
			out.Chars[y][x] = fr.Chars[y][cols-1-x]
			out.RGB[y][x] = fr.RGB[y][cols-1-x]
		}
	}
	return out
}

// BuildBuiltinMarine8 — 8 кадров как у Doom (0 лицом к камере, 4 спиной, 2/6 в профиль).
func BuildBuiltinMarine8() [8]PistolHUDFrame {
	var out [8]PistolHUDFrame
	front := parseMarineLayout(marineFrontLines)
	back := parseMarineLayout(marineBackLines)
	sideL := parseMarineLayout(marineSideLeftLines)
	q1 := parseMarineLayout(marineQuarter1Lines)
	q3 := parseMarineLayout(marineQuarter3Lines)

	out[0] = front
	out[4] = back
	out[2] = sideL
	out[6] = flipMarineFrameH(sideL)
	out[1] = q1
	out[7] = flipMarineFrameH(q1)
	out[3] = q3
	out[5] = flipMarineFrameH(q3)
	return out
}
