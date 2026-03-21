package wad

import (
	"encoding/binary"
	"fmt"
)

// DecodePatch decodes a Doom patch lump into palette-index pixels [y][x]. 0 = transparent.
func DecodePatch(data []byte) ([][]byte, int, int, error) {
	if len(data) < 8 {
		return nil, 0, 0, fmt.Errorf("patch too small")
	}
	width := int(binary.LittleEndian.Uint16(data[0:2]))
	height := int(binary.LittleEndian.Uint16(data[2:4]))
	if width <= 0 || height <= 0 || width > 4096 || height > 4096 {
		return nil, 0, 0, fmt.Errorf("bad patch dimensions")
	}
	if len(data) < 8+width*4 {
		return nil, 0, 0, fmt.Errorf("patch header incomplete")
	}
	pix := make([][]byte, height)
	for y := range pix {
		pix[y] = make([]byte, width)
	}
	for col := 0; col < width; col++ {
		off := int(binary.LittleEndian.Uint32(data[8+col*4 : 12+col*4]))
		if off < 0 || off >= len(data) {
			continue
		}
		p := off
		for {
			if p >= len(data) {
				break
			}
			row := int(data[p])
			p++
			if row == 255 {
				break
			}
			if p >= len(data) {
				break
			}
			length := int(data[p])
			p++
			if p >= len(data) {
				break
			}
			p++ // unused
			for i := 0; i < length; i++ {
				if p >= len(data) {
					break
				}
				yy := row + i
				if yy >= 0 && yy < height && col < width {
					pix[yy][col] = data[p]
				}
				p++
			}
			if p < len(data) {
				p++ // unused after column
			}
		}
	}
	return pix, width, height, nil
}
