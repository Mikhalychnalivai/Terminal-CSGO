package wad

import "fmt"

// LoadPlayPal returns palette 0 from PLAYPAL lump (256 RGB triples).
func LoadPlayPal(playpal []byte) ([256][3]byte, error) {
	var pal [256][3]byte
	if len(playpal) < 256*3 {
		return pal, fmt.Errorf("PLAYPAL too short")
	}
	for i := 0; i < 256; i++ {
		off := i * 3
		pal[i][0] = playpal[off]
		pal[i][1] = playpal[off+1]
		pal[i][2] = playpal[off+2]
	}
	return pal, nil
}

// Brightness returns 0..255 luminance for palette index.
func Brightness(pal [256][3]byte, idx byte) int {
	i := int(idx)
	r := int(pal[i][0])
	g := int(pal[i][1])
	b := int(pal[i][2])
	return (r*299 + g*587 + b*114) / 1000
}
