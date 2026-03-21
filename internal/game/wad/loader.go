package wad

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
)

type Vertex struct {
	X int16
	Y int16
}

type Thing struct {
	X    int16
	Y    int16
	Type int16
}

type LineDef struct {
	StartVertex uint16
	EndVertex   uint16
}

type MapData struct {
	MapName  string
	Vertices []Vertex
	Things   []Thing
	LineDefs []LineDef
	// WallTexture is a patch name from first valid SIDEDEF middle (Doom 8-char name).
	WallTexture string
	CeilingFlat string
	FloorFlat   string
}

type lump struct {
	Pos  int32
	Size int32
	Name string
}

func LoadMap(path string, mapName string) (*MapData, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read wad: %w", err)
	}
	if len(b) < 12 {
		return nil, fmt.Errorf("wad too short")
	}

	numLumps := int(binary.LittleEndian.Uint32(b[4:8]))
	dirOffset := int(binary.LittleEndian.Uint32(b[8:12]))
	if dirOffset < 0 || dirOffset+numLumps*16 > len(b) {
		return nil, fmt.Errorf("invalid wad directory")
	}

	lumps := make([]lump, 0, numLumps)
	for i := 0; i < numLumps; i++ {
		base := dirOffset + i*16
		pos := int32(binary.LittleEndian.Uint32(b[base : base+4]))
		size := int32(binary.LittleEndian.Uint32(b[base+4 : base+8]))
		rawName := b[base+8 : base+16]
		name := strings.TrimRight(string(rawName), "\x00 ")
		lumps = append(lumps, lump{Pos: pos, Size: size, Name: name})
	}

	mapIndex := -1
	for i, l := range lumps {
		if l.Name == mapName {
			mapIndex = i
			break
		}
	}
	if mapIndex < 0 {
		return nil, fmt.Errorf("map %s not found in wad", mapName)
	}

	var thingsData []byte
	var vertexData []byte
	var lineData []byte
	var sidedefData []byte
	var sectorData []byte
	for i := mapIndex + 1; i < len(lumps) && i < mapIndex+12; i++ {
		switch lumps[i].Name {
		case "THINGS":
			thingsData, err = readLump(b, lumps[i])
			if err != nil {
				return nil, err
			}
		case "VERTEXES":
			vertexData, err = readLump(b, lumps[i])
			if err != nil {
				return nil, err
			}
		case "LINEDEFS":
			lineData, err = readLump(b, lumps[i])
			if err != nil {
				return nil, err
			}
		case "SIDEDEFS":
			sidedefData, err = readLump(b, lumps[i])
			if err != nil {
				return nil, err
			}
		case "SECTORS":
			sectorData, err = readLump(b, lumps[i])
			if err != nil {
				return nil, err
			}
		}
	}

	if len(vertexData) == 0 {
		return nil, fmt.Errorf("VERTEXES not found for map %s", mapName)
	}

	vertices := parseVertices(vertexData)
	things := parseThings(thingsData)
	lines := parseLineDefs(lineData)
	wallTex, ceilFlat, floorFlat := extractTextures(sidedefData, sectorData)

	return &MapData{
		MapName:     mapName,
		Vertices:    vertices,
		Things:      things,
		LineDefs:    lines,
		WallTexture: wallTex,
		CeilingFlat: ceilFlat,
		FloorFlat:   floorFlat,
	}, nil
}

func extractTextures(sidedefData, sectorData []byte) (wall, ceil, floor string) {
	// First valid middle texture from sidedefs.
	for i := 0; i+30 <= len(sidedefData); i += 30 {
		mid := strings.TrimRight(string(sidedefData[i+20:i+28]), "\x00 ")
		if mid != "" && mid != "-" {
			wall = mid
			break
		}
	}
	if len(sectorData) >= 26 {
		floor = strings.TrimRight(string(sectorData[4:12]), "\x00 ")
		ceil = strings.TrimRight(string(sectorData[12:20]), "\x00 ")
	}
	return
}

func readLump(wad []byte, l lump) ([]byte, error) {
	start := int(l.Pos)
	end := start + int(l.Size)
	if start < 0 || end > len(wad) || end < start {
		return nil, fmt.Errorf("invalid lump bounds for %s", l.Name)
	}
	return wad[start:end], nil
}

func parseVertices(data []byte) []Vertex {
	out := make([]Vertex, 0, len(data)/4)
	r := bytes.NewReader(data)
	for r.Len() >= 4 {
		var x int16
		var y int16
		_ = binary.Read(r, binary.LittleEndian, &x)
		_ = binary.Read(r, binary.LittleEndian, &y)
		out = append(out, Vertex{X: x, Y: y})
	}
	return out
}

func parseThings(data []byte) []Thing {
	out := make([]Thing, 0, len(data)/10)
	r := bytes.NewReader(data)
	for r.Len() >= 10 {
		var x int16
		var y int16
		var angle int16
		var t int16
		var flags int16
		_ = binary.Read(r, binary.LittleEndian, &x)
		_ = binary.Read(r, binary.LittleEndian, &y)
		_ = binary.Read(r, binary.LittleEndian, &angle)
		_ = binary.Read(r, binary.LittleEndian, &t)
		_ = binary.Read(r, binary.LittleEndian, &flags)
		out = append(out, Thing{X: x, Y: y, Type: t})
	}
	return out
}

func parseLineDefs(data []byte) []LineDef {
	out := make([]LineDef, 0, len(data)/14)
	r := bytes.NewReader(data)
	for r.Len() >= 14 {
		var start uint16
		var end uint16
		var flags uint16
		var lineType uint16
		var tag uint16
		var rightSidedef uint16
		var leftSidedef uint16
		_ = binary.Read(r, binary.LittleEndian, &start)
		_ = binary.Read(r, binary.LittleEndian, &end)
		_ = binary.Read(r, binary.LittleEndian, &flags)
		_ = binary.Read(r, binary.LittleEndian, &lineType)
		_ = binary.Read(r, binary.LittleEndian, &tag)
		_ = binary.Read(r, binary.LittleEndian, &rightSidedef)
		_ = binary.Read(r, binary.LittleEndian, &leftSidedef)
		out = append(out, LineDef{
			StartVertex: start,
			EndVertex:   end,
		})
	}
	return out
}
