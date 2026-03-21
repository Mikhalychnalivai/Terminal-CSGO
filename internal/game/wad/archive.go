package wad

import (
	"encoding/binary"
	"fmt"
	"os"
	"strings"
)

// Archive is a loaded IWAD/PWAD with directory access.
type Archive struct {
	raw   []byte
	lumps []lumpInfo
}

type lumpInfo struct {
	Pos  int32
	Size int32
	Name string
}

// OpenArchive reads a WAD file and builds lump directory.
func OpenArchive(path string) (*Archive, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(b) < 12 {
		return nil, fmt.Errorf("wad too short")
	}
	numLumps := int(binary.LittleEndian.Uint32(b[4:8]))
	dirOffset := int(binary.LittleEndian.Uint32(b[8:12]))
	if dirOffset < 0 || dirOffset+numLumps*16 > len(b) {
		return nil, fmt.Errorf("invalid wad directory")
	}
	lumps := make([]lumpInfo, 0, numLumps)
	for i := 0; i < numLumps; i++ {
		base := dirOffset + i*16
		pos := int32(binary.LittleEndian.Uint32(b[base : base+4]))
		size := int32(binary.LittleEndian.Uint32(b[base+4 : base+8]))
		rawName := b[base+8 : base+16]
		name := strings.TrimRight(string(rawName), "\x00 ")
		lumps = append(lumps, lumpInfo{Pos: pos, Size: size, Name: name})
	}
	return &Archive{raw: b, lumps: lumps}, nil
}

// LumpData returns raw bytes for a named lump, or nil if not found.
func (a *Archive) LumpData(name string) []byte {
	name = strings.ToUpper(strings.TrimSpace(name))
	for _, l := range a.lumps {
		if strings.EqualFold(l.Name, name) {
			start := int(l.Pos)
			end := start + int(l.Size)
			if start < 0 || end > len(a.raw) || end < start {
				return nil
			}
			out := make([]byte, end-start)
			copy(out, a.raw[start:end])
			return out
		}
	}
	return nil
}
