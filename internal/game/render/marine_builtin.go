package render

// Встроенный «shooter guy»: шлем/кожа, визор, зелёная броня, чёрное ружьё, сапоги — 8 направлений (билборд).

var (
	marineFrontLines = []string{
		".......hhh.......",
		".......vvv.......",
		"......sdgggds....",
		".....skkkkkks....",
		"......dg...gd....",
		"......bb...bb....",
	}
	marineBackLines = []string{
		".......hhh.......",
		".......vvv.......",
		"......dggggd.....",
		".......gggg......",
		"......bb...bb....",
	}
	marineSideLeftLines = []string{
		"........hh.......",
		"........hv.......",
		".......sgg.......",
		".......skkk......",
		".......dgg.......",
		"........gb.......",
		".........b.......",
	}
	marineQuarter1Lines = []string{
		".......hhh.......",
		"......vvvv.......",
		".....ssdgggg.....",
		"....sskkkks......",
		".....dg...g......",
		".....bb...b......",
	}
	marineQuarter3Lines = []string{
		"........hh.......",
		".......hvv.......",
		"......dgggg......",
		".......sgk.......",
		"......bb.b.......",
	}
)

func marinePixel(ch rune) (rune, uint32) {
	switch ch {
	case '.', ' ':
		return 0, 0
	case 'h': // шлем
		return '█', RGBPacked(200, 162, 110)
	case 'v': // визор
		return '█', RGBPacked(44, 48, 58)
	case 'g': // броня
		return '█', RGBPacked(0, 156, 48)
	case 'd': // тень брони
		return '█', RGBPacked(0, 98, 34)
	case 's': // кожа рук
		return '█', RGBPacked(226, 180, 136)
	case 'k': // ружьё
		return '█', RGBPacked(20, 20, 26)
	case 'b': // сапоги
		return '█', RGBPacked(88, 90, 100)
	default:
		return 0, 0
	}
}

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
			r, c := marinePixel(ch)
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

// BuildBuiltinMarine8 — 8 кадров как у shooter (0 лицом к камере, 4 спиной, 2/6 в профиль).
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
