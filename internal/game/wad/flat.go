package wad

import "fmt"

// DecodeFlat turns a 4096-byte FLAT lump into 64x64 palette indices.
func DecodeFlat(data []byte) ([][]byte, error) {
	if len(data) < 4096 {
		return nil, fmt.Errorf("flat too short")
	}
	pix := make([][]byte, 64)
	for y := 0; y < 64; y++ {
		pix[y] = make([]byte, 64)
		copy(pix[y], data[y*64:y*64+64])
	}
	return pix, nil
}
