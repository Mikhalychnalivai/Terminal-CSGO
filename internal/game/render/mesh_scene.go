package render

import (
	"hack2026mart/internal/game/nav"
	"hack2026mart/internal/game/protocol"
)

const meshWallHeight = 1.0

// MeshTri — треугольник уровня для софт-растеризации (Asciipocalypse-стиль).
type MeshTri struct {
	V0, V1, V2 Vec3
	Kind       meshTriKind
	// WallAxis: 0 — пол/потолок; 1 — стена в плоскости X (дольше по Z); 2 — стена в плоскости Z (дольше по X).
	WallAxis uint8
	// FloorCellX/Z — для Kind==meshFloor: клетка карты (1..w-2, 1..h-2), сущность «проходимая плитка».
	// Для стен/потолка: -1.
	FloorCellX int16
	FloorCellZ int16
}

type meshTriKind int

const (
	meshWall meshTriKind = iota
	meshFloor
	meshCeiling
)

func cellBlocked(blocked map[uint64]struct{}, gx, gy int) bool {
	_, ok := blocked[nav.CellKey(gx, gy)]
	return ok
}

// walkable: проходимая клетка внутри границ карты (как на сервере).
func cellWalkable(blocked map[uint64]struct{}, w, h, gx, gy int) bool {
	if gx < 1 || gx > w-2 || gy < 1 || gy > h-2 {
		return false
	}
	return !cellBlocked(blocked, gx, gy)
}

// buildMeshFromSnapshot — стены как вертикальные грани между blocked и walkable/void + пол/потолок.
func buildMeshFromSnapshot(snap *protocol.RoomSnapshot, blocked map[uint64]struct{}) []MeshTri {
	w, h := snap.Width, snap.Height
	if w < 4 || h < 4 {
		return nil
	}
	var tris []MeshTri

	// Потолок: один квад на внутреннюю область.
	x0, z0 := float64(1), float64(1)
	x1, z1 := float64(w-2)+1, float64(h-2)+1
	yc := meshWallHeight
	tris = append(tris,
		MeshTri{V0: Vec3{x0, yc, z0}, V1: Vec3{x1, yc, z0}, V2: Vec3{x1, yc, z1}, Kind: meshCeiling, FloorCellX: -1, FloorCellZ: -1},
		MeshTri{V0: Vec3{x0, yc, z0}, V1: Vec3{x1, yc, z1}, V2: Vec3{x0, yc, z1}, Kind: meshCeiling, FloorCellX: -1, FloorCellZ: -1},
	)
	// Пол — только проходимые клетки: каждая клетка = сущность «плитка» (два треугольника), не сплошной задник.
	for gx := 1; gx <= w-2; gx++ {
		for gz := 1; gz <= h-2; gz++ {
			if !cellWalkable(blocked, w, h, gx, gz) {
				continue
			}
			addFloorTileQuad(&tris, gx, gz)
		}
	}

	// Внешний периметр комнаты: стены на границе проходимой зоны (между void и внутренностью).
	for gz := 1; gz <= h-2; gz++ {
		// Запад x=1: слева void (x<1), справа возможный проход
		if cellWalkable(blocked, w, h, 1, gz) {
			addWallQuadX(&tris, 1, gz, +1) // нормаль в сторону -X (внутрь комнаты с +X)
		}
		// Восток x=w-1 граница последней колонки клеток w-2
		if cellWalkable(blocked, w, h, w-2, gz) {
			addWallQuadX(&tris, float64(w-1), gz, -1)
		}
	}
	for gx := 1; gx <= w-2; gx++ {
		if cellWalkable(blocked, w, h, gx, 1) {
			addWallQuadZ(&tris, gx, 1, +1)
		}
		if cellWalkable(blocked, w, h, gx, h-2) {
			addWallQuadZ(&tris, gx, float64(h-1), -1)
		}
	}

	// Внутренние стены: грань между blocked и walkable.
	for gx := 1; gx <= w-2; gx++ {
		for gz := 1; gz <= h-2; gz++ {
			if !cellBlocked(blocked, gx, gz) {
				continue
			}
			// +X
			if cellWalkable(blocked, w, h, gx+1, gz) {
				addWallQuadX(&tris, float64(gx+1), gz, -1)
			}
			// -X
			if cellWalkable(blocked, w, h, gx-1, gz) {
				addWallQuadX(&tris, float64(gx), gz, +1)
			}
			// +Z
			if cellWalkable(blocked, w, h, gx, gz+1) {
				addWallQuadZ(&tris, gx, float64(gz+1), -1)
			}
			// -Z
			if cellWalkable(blocked, w, h, gx, gz-1) {
				addWallQuadZ(&tris, gx, float64(gz), +1)
			}
		}
	}
	return tris
}

// addFloorTileQuad — одна проходимая клетка пола y=0, квад [gx,gx+1]×[gz,gz+1] в XZ.
func addFloorTileQuad(tris *[]MeshTri, gx, gz int) {
	const yf = 0.0
	x0, z0 := float64(gx), float64(gz)
	x1, z1 := float64(gx+1), float64(gz+1)
	cx, cz := int16(gx), int16(gz)
	*tris = append(*tris,
		MeshTri{V0: Vec3{x0, yf, z0}, V1: Vec3{x1, yf, z1}, V2: Vec3{x1, yf, z0}, Kind: meshFloor, FloorCellX: cx, FloorCellZ: cz},
		MeshTri{V0: Vec3{x0, yf, z0}, V1: Vec3{x0, yf, z1}, V2: Vec3{x1, yf, z1}, Kind: meshFloor, FloorCellX: cx, FloorCellZ: cz},
	)
}

// addWallQuadX: вертикальная стена в плоскости x = xf, полоса z ∈ [gz, gz+1]. dir — знак нормали к проходу.
func addWallQuadX(tris *[]MeshTri, xf float64, gz int, dir float64) {
	y0, y1 := 0.0, meshWallHeight
	z0, z1 := float64(gz), float64(gz+1)
	var a0, a1, a2, a3 Vec3
	if dir > 0 {
		// нормаль +X: треугольники видны с +X
		a0 = Vec3{xf, y0, z0}
		a1 = Vec3{xf, y1, z0}
		a2 = Vec3{xf, y1, z1}
		a3 = Vec3{xf, y0, z1}
	} else {
		a0 = Vec3{xf, y0, z0}
		a1 = Vec3{xf, y0, z1}
		a2 = Vec3{xf, y1, z1}
		a3 = Vec3{xf, y1, z0}
	}
	*tris = append(*tris,
		MeshTri{V0: a0, V1: a1, V2: a2, Kind: meshWall, WallAxis: 1, FloorCellX: -1, FloorCellZ: -1},
		MeshTri{V0: a0, V1: a2, V2: a3, Kind: meshWall, WallAxis: 1, FloorCellX: -1, FloorCellZ: -1},
	)
}

func addWallQuadZ(tris *[]MeshTri, gx int, zf float64, dir float64) {
	y0, y1 := 0.0, meshWallHeight
	x0, x1 := float64(gx), float64(gx+1)
	var a0, a1, a2, a3 Vec3
	if dir > 0 {
		// нормаль +Z
		a0 = Vec3{x0, y0, zf}
		a1 = Vec3{x1, y0, zf}
		a2 = Vec3{x1, y1, zf}
		a3 = Vec3{x0, y1, zf}
	} else {
		a0 = Vec3{x0, y0, zf}
		a1 = Vec3{x0, y1, zf}
		a2 = Vec3{x1, y1, zf}
		a3 = Vec3{x1, y0, zf}
	}
	*tris = append(*tris,
		MeshTri{V0: a0, V1: a1, V2: a2, Kind: meshWall, WallAxis: 2, FloorCellX: -1, FloorCellZ: -1},
		MeshTri{V0: a0, V1: a2, V2: a3, Kind: meshWall, WallAxis: 2, FloorCellX: -1, FloorCellZ: -1},
	)
}
