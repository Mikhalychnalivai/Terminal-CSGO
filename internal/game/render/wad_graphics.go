package render

import (
	"fmt"
	"math"
	"strings"

	"hack2026mart/internal/game/wad"
)

func RGBPacked(r, g, b byte) uint32 {
	return uint32(r)<<16 | uint32(g)<<8 | uint32(b)
}

type PistolHUDFrame struct {
	Chars [][]rune
	RGB   [][]uint32
}

type WadGraphics struct {
	Key string

	Pal [256][3]byte

	Wall    [][]byte
	WallW   int
	WallH   int
	Ceiling [][]byte
	Floor   [][]byte

	Pistol []PistolHUDFrame

	// Player8 — 8 направлений встроенного морпеха (стиль shooter: шлем, визор, броня, ружьё).
	Player8         [8]PistolHUDFrame
	PlayerSpritesOK bool

	OK bool
}

func LoadWadGraphics(path, wallName, ceilName, floorName string) *WadGraphics {
	g := &WadGraphics{
		Key: path + "|" + wallName + "|" + ceilName + "|" + floorName,
	}
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
		for _, fb := range []string{"STARTAN3", "STARG3", "STONE2", "BIGDOOR2"} {
			if tryPatch(fb) {
				break
			}
		}
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

	for _, name := range []string{"PISGA0", "PISGB0", "PISGC0", "PISGD0"} {
		data := a.LumpData(strings.ToUpper(name))
		if len(data) == 0 {
			data = a.LumpData(name)
		}
		if len(data) == 0 {
			continue
		}
		if fr, err := patchToHUD(data, g.Pal, 56, 13); err == nil && fr != nil {
			g.Pistol = append(g.Pistol, *fr)
		}
	}

	g.OK = g.Wall != nil && g.WallH > 0 && g.WallW > 0
	return g
}

func patchToHUD(data []byte, pal [256][3]byte, maxW, maxH int) (*PistolHUDFrame, error) {
	pix, w, h, err := wad.DecodePatch(data)
	if err != nil || w < 2 || h < 2 {
		return nil, fmt.Errorf("patch decode")
	}
	minX, minY := w, h
	maxX, maxY := -1, -1
	for yy := 0; yy < h; yy++ {
		for xx := 0; xx < w; xx++ {
			if pix[yy][xx] == 0 {
				continue
			}
			if xx < minX {
				minX = xx
			}
			if xx > maxX {
				maxX = xx
			}
			if yy < minY {
				minY = yy
			}
			if yy > maxY {
				maxY = yy
			}
		}
	}
	if maxX < minX || maxY < minY {
		return nil, fmt.Errorf("empty pistol patch")
	}
	bw := maxX - minX + 1
	bh := maxY - minY + 1
	scaleW := float64(maxW) / float64(bw)
	scaleH := float64(maxH) / float64(bh)
	scale := scaleW
	if scaleH < scale {
		scale = scaleH
	}
	outW := int(float64(bw) * scale)
	outH := int(float64(bh) * scale)
	if outW < 1 {
		outW = 1
	}
	if outH < 1 {
		outH = 1
	}
	if outW > maxW {
		outW = maxW
	}
	if outH > maxH {
		outH = maxH
	}
	fr := &PistolHUDFrame{
		Chars: make([][]rune, outH),
		RGB:   make([][]uint32, outH),
	}
	for oy := 0; oy < outH; oy++ {
		fr.Chars[oy] = make([]rune, outW)
		fr.RGB[oy] = make([]uint32, outW)
		for ox := 0; ox < outW; ox++ {
			sx := minX + int((float64(ox)+0.5)/float64(outW)*float64(bw))
			sy := minY + int((float64(oy)+0.5)/float64(outH)*float64(bh))
			if sx > maxX {
				sx = maxX
			}
			if sy > maxY {
				sy = maxY
			}
			idx := pix[sy][sx]
			if idx == 0 {
				fr.Chars[oy][ox] = 0
				continue
			}
			pr, pg, pb := pal[idx][0], pal[idx][1], pal[idx][2]
			br := wad.Brightness(pal, idx)
			fr.Chars[oy][ox] = lumaChar(br)
			fr.RGB[oy][ox] = RGBPacked(pr, pg, pb)
		}
	}
	return fr, nil
}

func (g *WadGraphics) SampleWall(u, v float64, distRatio float64, vertical bool) (rune, uint32) {
	if !g.OK {
		return ' ', RGBPacked(200, 200, 200)
	}
	u = fract(u)
	v = fract(v)
	ix := int(u * float64(g.WallW-1))
	iy := int(v * float64(g.WallH-1))
	if ix < 0 {
		ix = 0
	}
	if iy < 0 {
		iy = 0
	}
	if ix >= g.WallW {
		ix = g.WallW - 1
	}
	if iy >= g.WallH {
		iy = g.WallH - 1
	}
	idx := g.Wall[iy][ix]
	if idx == 0 {
		return '░', RGBPacked(55, 55, 60)
	}
	r, gg, b := int(g.Pal[idx][0]), int(g.Pal[idx][1]), int(g.Pal[idx][2])
	fog := math.Min(1, math.Max(0, distRatio))
	r = int(float64(r) * (1 - fog*0.55))
	gg = int(float64(gg) * (1 - fog*0.55))
	b = int(float64(b) * (1 - fog*0.55))
	if vertical {
		r = int(float64(r) * 0.9)
		gg = int(float64(gg) * 0.9)
		b = int(float64(b) * 0.9)
	}
	r = clampByte(r)
	gg = clampByte(gg)
	b = clampByte(b)
	br := wad.Brightness(g.Pal, idx)
	br = int(float64(br) * (1 - fog*0.4))
	ch := blockLumaChar(br)
	return ch, RGBPacked(byte(r), byte(gg), byte(b))
}

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
	return lumaChar(br), RGBPacked(r, gg, b)
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
