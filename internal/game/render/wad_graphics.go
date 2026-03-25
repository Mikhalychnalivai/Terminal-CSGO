package render

import (
	"math"
	"strings"

	"hack2026mart/internal/game/wad"
)

// RGBPacked packs Doom PLAYPAL RGB into a uint32 for truecolor ANSI (38;2;r;g;b).
func RGBPacked(r, g, b byte) uint32 {
	return uint32(r)<<16 | uint32(g)<<8 | uint32(b)
}

// PistolHUDFrame is a downscaled pistol sprite for the terminal HUD (0 rune = transparent).
type PistolHUDFrame struct {
	Chars [][]rune
	RGB   [][]uint32
}

// WadGraphics holds decoded Doom WAD graphics for terminal sampling.
type WadGraphics struct {
	Key string

	Pal [256][3]byte

	Wall    [][]byte
	WallW   int
	WallH   int
	// WallVert — второй патч для граней, где луч пересекает вертикальную сетку (как другая
	// сторона/вариант текстуры в стиле Doom: разные освещённые полосы на NS vs EW).
	WallVert  [][]byte
	WallVertW int
	WallVertH int

	Ceiling [][]byte
	Floor   [][]byte

	// Player8 — 8 направлений встроенного морпеха (стиль Doom: шлем, визор, броня, ружьё).
	Player8         [8]PistolHUDFrame
	PlayerSpritesOK bool

	OK bool
}

// LoadWadGraphics loads PLAYPAL + wall patch + ceiling/floor flats from path.
func LoadWadGraphics(path, wallName, ceilName, floorName string) *WadGraphics {
	g := &WadGraphics{
		Key: path + "|" + wallName + "|" + ceilName + "|" + floorName,
	}
	// Морпех в мире — всегда встроенный (не зависит от WAD).
	g.Player8 = BuildBuiltinMarine8()
	g.PlayerSpritesOK = true

	a, err := wad.OpenArchive(path)
	if err != nil {
		return g
	}
	palData := a.LumpData("PLAYPAL")
	if len(palData) < 768 {
		return g
	}
	pal, err := wad.LoadPlayPal(palData[:768])
	if err != nil {
		return g
	}
	g.Pal = pal

	tryPatch := func(name string) bool {
		name = strings.TrimSpace(name)
		if name == "" || name == "-" {
			return false
		}
		data := a.LumpData(strings.ToUpper(name))
		if len(data) == 0 {
			data = a.LumpData(name)
		}
		if len(data) == 0 {
			return false
		}
		pix, ww, wh, err := wad.DecodePatch(data)
		if err != nil || ww < 2 || wh < 2 {
			return false
		}
		g.Wall = pix
		g.WallW, g.WallH = ww, wh
		return true
	}

	if !tryPatch(wallName) {
		// Fallbacks common in shareware / E1M1
		for _, fb := range []string{"STARTAN3", "STARG3", "STONE2", "BIGDOOR2"} {
			if tryPatch(fb) {
				break
			}
		}
	}

	// Второй патч для «вертикальных» попаданий (NS-грань): типичные пары из DOOM.WAD.
	tryVert := func(names ...string) {
		for _, name := range names {
			name = strings.TrimSpace(name)
			if name == "" || name == "-" {
				continue
			}
			data := a.LumpData(strings.ToUpper(name))
			if len(data) == 0 {
				data = a.LumpData(name)
			}
			if len(data) == 0 {
				continue
			}
			pix, ww, wh, err := wad.DecodePatch(data)
			if err != nil || ww < 2 || wh < 2 {
				continue
			}
			g.WallVert = pix
			g.WallVertW, g.WallVertH = ww, wh
			return
		}
	}
	// Подбор по имени основной текстуры (как в оригинальных картах E1).
	switch strings.ToUpper(strings.TrimSpace(wallName)) {
	case "STARTAN3", "STARG3", "STARGR1":
		tryVert("STARG3", "STARTAN3", "STONE2")
	case "STONE2", "STONE3":
		tryVert("STONE3", "STONE2", "BIGDOOR2")
	case "BIGDOOR2", "BIGDOOR1":
		tryVert("BIGDOOR1", "BIGDOOR2", "STONE2")
	default:
		tryVert("STARG3", "STONE2", "STARTAN3")
	}
	if len(g.WallVert) == 0 {
		g.WallVert, g.WallVertW, g.WallVertH = g.Wall, g.WallW, g.WallH
	}

	tryFlat := func(name string) [][]byte {
		name = strings.TrimSpace(name)
		if name == "" || name == "-" {
			return nil
		}
		data := a.LumpData(strings.ToUpper(name))
		if len(data) < 4096 {
			data = a.LumpData(name)
		}
		if len(data) < 4096 {
			return nil
		}
		pix, err := wad.DecodeFlat(data[:4096])
		if err != nil {
			return nil
		}
		return pix
	}

	g.Ceiling = tryFlat(ceilName)
	if g.Ceiling == nil {
		g.Ceiling = tryFlat("FLAT5_4")
	}
	g.Floor = tryFlat(floorName)
	if g.Floor == nil {
		g.Floor = tryFlat("FLOOR5_1")
	}

	g.OK = g.Wall != nil && g.WallH > 0 && g.WallW > 0
	if g.WallVert == nil || g.WallVertW < 2 {
		g.WallVert, g.WallVertW, g.WallVertH = g.Wall, g.WallW, g.WallH
	}
	return g
}

// SampleWall возвращает цвет пикселя из патча WAD и яркость (0–255) для выбора глифа в ascii
// (рампа глубины — depthGlyphFromLumJitter в mesh_raster / ascii).
// verticalHit = true — вертикальная грань сетки (NS); иначе WallVert/освещение как в DOOM.WAD.
func (g *WadGraphics) SampleWall(u, v float64, distRatio float64, verticalHit bool) (uint32, int) {
	if !g.OK {
		return RGBPacked(88, 76, 70), 92
	}
	pix := g.Wall
	ww, wh := g.WallW, g.WallH
	if verticalHit && g.WallVert != nil && len(g.WallVert) > 0 && g.WallVertW >= 2 {
		pix = g.WallVert
		ww, wh = g.WallVertW, g.WallVertH
	}
	u = fract(u)
	v = fract(v)
	ix := int(u * float64(ww-1))
	iy := int(v * float64(wh-1))
	if ix < 0 {
		ix = 0
	}
	if iy < 0 {
		iy = 0
	}
	if ix >= ww {
		ix = ww - 1
	}
	if iy >= wh {
		iy = wh - 1
	}
	idx := pix[iy][ix]
	if idx == 0 {
		// Тёмная «пустота» патча — как тень между кирпичами в Doom.
		return RGBPacked(38, 34, 36), 28
	}
	r, gg, b := int(g.Pal[idx][0]), int(g.Pal[idx][1]), int(g.Pal[idx][2])
	fog := math.Min(1, math.Max(0, distRatio))
	// Сильнее увод вдаль (типичный туман E1).
	r = int(float64(r) * (1 - fog*0.62))
	gg = int(float64(gg) * (1 - fog*0.62))
	b = int(float64(b) * (1 - fog*0.62))
	if verticalHit {
		r = int(float64(r) * 0.82)
		gg = int(float64(gg) * 0.82)
		b = int(float64(b) * 0.82)
	} else {
		r = int(float64(r) * 0.90)
		gg = int(float64(gg) * 0.90)
		b = int(float64(b) * 0.90)
	}
	r, gg, b = doomWallGradeRGB(r, gg, b)
	r = clampByte(r)
	gg = clampByte(gg)
	b = clampByte(b)
	br := wad.Brightness(g.Pal, idx)
	br = int(float64(br) * (1 - fog*0.48))
	br = int(float64(br) * 0.88)
	if br < 0 {
		br = 0
	}
	if br > 255 {
		br = 255
	}
	return RGBPacked(byte(r), byte(gg), byte(b)), br
}

// doomWallGradeRGB — тёмная приглушённая палитра в духе Doom (умбра, без «мыльных» ярких).
func doomWallGradeRGB(r, g, b int) (int, int, int) {
	fr, fg, fb := float64(r), float64(g), float64(b)
	// Общее затемнение
	fr *= 0.66
	fg *= 0.60
	fb *= 0.54
	// Десатурация к люме (меньше «кислоты»)
	l := 0.299*fr + 0.587*fg + 0.114*fb
	mix := 0.38
	fr = fr*(1-mix) + l*mix
	fg = fg*(1-mix) + l*mix
	fb = fb*(1-mix) + l*mix
	// Лёгкий сдвиг в тёплый коричневато-ржавый (стены E1)
	fr *= 1.04
	fg *= 0.98
	fb *= 0.88
	return clampByte(int(fr)), clampByte(int(fg)), clampByte(int(fb))
}

// SampleFlat samples a 64x64 flat at u,v in [0,1) — FLAT из DOOM.WAD (64×64 индексов в PLAYPAL).
func (g *WadGraphics) SampleFlat(pix [][]byte, u, v float64) (rune, uint32) {
	if len(pix) != 64 {
		return '.', RGBPacked(0, 120, 180)
	}
	u = fract(u)
	v = fract(v)
	ix := int(u * 63)
	iy := int(v * 63)
	idx := pix[iy][ix]
	r, gg, b := g.Pal[idx][0], g.Pal[idx][1], g.Pal[idx][2]
	br := wad.Brightness(g.Pal, idx)
	// Та же блочная рампа, что у стен — визуально ближе к «кирпичной» графике Doom.
	return blockLumaChar(br), RGBPacked(r, gg, b)
}

func fract(x float64) float64 {
	x = math.Mod(x, 1)
	if x < 0 {
		x += 1
	}
	return x
}

func clampByte(v int) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

// lumaChar — классическая ASCII-рампа для фона (потолок/пол) и спрайтов морпеха.
func lumaChar(l int) rune {
	if l < 0 {
		l = 0
	}
	if l > 255 {
		l = 255
	}
	chars := []rune{'@', '#', '8', '&', 'O', 'o', '*', '+', ':', '.', ' '}
	step := 255 / (len(chars) - 1)
	i := l / step
	if i >= len(chars) {
		i = len(chars) - 1
	}
	return chars[i]
}

// blockLumaChar — только для вертикальных стен (█▓▒░…), без «@».
func blockLumaChar(l int) rune {
	if l < 0 {
		l = 0
	}
	if l > 255 {
		l = 255
	}
	chars := []rune{'█', '█', '▓', '▓', '▒', '▒', '░', '░', '·', ':', ' '}
	step := 255 / (len(chars) - 1)
	i := l / step
	if i >= len(chars) {
		i = len(chars) - 1
	}
	return chars[i]
}
