package navmesh

import (
	"testing"

	"hack2026mart/internal/game/nav"
)

func TestBuildFromBlockedEmptyRoom(t *testing.T) {
	blocked := map[uint64]struct{}{}
	m := BuildFromBlocked(8, 8, blocked)
	if len(m.Polys) < 1 {
		t.Fatalf("expected at least one polygon, got %d", len(m.Polys))
	}
	if m.PolygonAt(2.5, 2.5) < 0 {
		t.Fatal("center should be inside mesh")
	}
	path, ok := m.FindPath(2.5, 2.5, 5.5, 5.5)
	if !ok || len(path) < 2 {
		t.Fatalf("path in open room: ok=%v path=%v", ok, path)
	}
}

func TestFindPathAroundObstacle(t *testing.T) {
	// 6x4 карта: внутренняя зона gy=1..2, gx=1..4; одна заблокированная клетка (3,2) — «щель» между верхним и нижним рядами.
	blocked := map[uint64]struct{}{
		nav.CellKey(3, 2): {},
	}
	m := BuildFromBlocked(6, 4, blocked)
	if len(m.Polys) < 2 {
		t.Fatalf("expected multiple polygons, got %d", len(m.Polys))
	}
	path, ok := m.FindPath(1.5, 1.5, 4.5, 2.5)
	if !ok {
		t.Fatal("expected path around obstacle")
	}
	if len(path) < 2 {
		t.Fatalf("path too short: %v", path)
	}
}
