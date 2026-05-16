package spatial

import (
	"fmt"
	"math"
	"testing"
)

// mustUpdate is a test helper that calls Update and fatals on error.
func mustUpdate(t *testing.T, h *GridSpatialHash, id EntityID, pos EntityPosition) {
	t.Helper()
	if err := h.Update(id, pos); err != nil {
		t.Fatalf("Update(%q, %v): %v", id, pos, err)
	}
}

func TestGridHash_InsertAndQuery(t *testing.T) {
	h := NewGridSpatialHash(SpatialConfig{CellSizeM: 10})
	mustUpdate(t, h, "player-1", Pos(5, 5))
	mustUpdate(t, h, "player-2", Pos(7, 7))

	result := h.QueryRadius(Pos(5, 5), 5)
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}

	found := map[EntityID]bool{}
	for _, id := range result {
		found[id] = true
	}
	if !found["player-1"] || !found["player-2"] {
		t.Errorf("expected both player-1 and player-2, got %v", result)
	}
}

func TestGridHash_QueryRadiusExcludesFar(t *testing.T) {
	h := NewGridSpatialHash(SpatialConfig{CellSizeM: 10})
	mustUpdate(t, h, "near", Pos(5, 5))
	mustUpdate(t, h, "far", Pos(100, 100))

	result := h.QueryRadius(Pos(5, 5), 10)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(result), result)
	}
	if result[0] != "near" {
		t.Errorf("expected 'near', got %q", result[0])
	}
}

func TestGridHash_UpdatePosition(t *testing.T) {
	h := NewGridSpatialHash(SpatialConfig{CellSizeM: 10})

	mustUpdate(t, h, "p1", Pos(0, 0))
	result := h.QueryRadius(Pos(0, 0), 5)
	if len(result) != 1 {
		t.Fatalf("before move: expected 1, got %d", len(result))
	}

	mustUpdate(t, h, "p1", Pos(100, 100))

	result = h.QueryRadius(Pos(0, 0), 5)
	if len(result) != 0 {
		t.Errorf("after move, old position: expected 0, got %d", len(result))
	}

	result = h.QueryRadius(Pos(100, 100), 5)
	if len(result) != 1 {
		t.Errorf("after move, new position: expected 1, got %d", len(result))
	}
}

func TestGridHash_UpdateSameCell(t *testing.T) {
	h := NewGridSpatialHash(SpatialConfig{CellSizeM: 10})

	mustUpdate(t, h, "p1", Pos(1, 1))
	mustUpdate(t, h, "p1", Pos(2, 2)) // same cell (0,0)

	if h.Len() != 1 {
		t.Errorf("expected 1 entity after update within same cell, got %d", h.Len())
	}

	pos, ok := h.Get("p1")
	if !ok {
		t.Fatal("entity should exist")
	}
	if pos.X != 2 || pos.Z != 2 {
		t.Errorf("position = (%.1f, %.1f), want (2, 2)", pos.X, pos.Z)
	}
}

func TestGridHash_Remove(t *testing.T) {
	h := NewGridSpatialHash(SpatialConfig{CellSizeM: 10})
	mustUpdate(t, h, "p1", Pos(5, 5))

	if h.Len() != 1 {
		t.Fatalf("before remove: expected 1, got %d", h.Len())
	}

	h.Remove("p1")

	if h.Len() != 0 {
		t.Errorf("after remove: expected 0, got %d", h.Len())
	}

	result := h.QueryRadius(Pos(5, 5), 10)
	if len(result) != 0 {
		t.Errorf("query after remove: expected 0, got %d", len(result))
	}
}

func TestGridHash_RemoveNonexistent(t *testing.T) {
	h := NewGridSpatialHash(SpatialConfig{CellSizeM: 10})
	h.Remove("ghost") // must not panic
	if h.Len() != 0 {
		t.Errorf("expected 0, got %d", h.Len())
	}
}

func TestGridHash_QueryRadiusZero(t *testing.T) {
	h := NewGridSpatialHash(SpatialConfig{CellSizeM: 10})
	mustUpdate(t, h, "p1", Pos(5, 5))

	result := h.QueryRadius(Pos(5, 5), 0)
	if len(result) != 0 {
		t.Errorf("zero radius should return nothing, got %d", len(result))
	}
}

func TestGridHash_QueryRadiusExactBoundary(t *testing.T) {
	h := NewGridSpatialHash(SpatialConfig{CellSizeM: 10})

	radius := float32(10.0)
	mustUpdate(t, h, "p1", Pos(radius, 0)) // exactly at radius

	result := h.QueryRadius(Pos(0, 0), radius)
	if len(result) != 1 {
		t.Errorf("entity at exact radius boundary should be included, got %d", len(result))
	}
}

func TestGridHash_QueryRadiusJustOutsideBoundary(t *testing.T) {
	h := NewGridSpatialHash(SpatialConfig{CellSizeM: 10})

	radius := float32(10.0)
	mustUpdate(t, h, "p1", Pos(radius+0.01, 0)) // just outside

	result := h.QueryRadius(Pos(0, 0), radius)
	if len(result) != 0 {
		t.Errorf("entity just outside radius should not be included, got %d", len(result))
	}
}

func TestGridHash_CrossCellQuery(t *testing.T) {
	h := NewGridSpatialHash(SpatialConfig{CellSizeM: 10})

	mustUpdate(t, h, "west", Pos(45, 50))  // cell (4,5)
	mustUpdate(t, h, "east", Pos(55, 50))  // cell (5,5)
	mustUpdate(t, h, "north", Pos(50, 45)) // cell (5,4)
	mustUpdate(t, h, "south", Pos(50, 55)) // cell (5,5)
	mustUpdate(t, h, "far", Pos(90, 90))   // outside radius

	result := h.QueryRadius(Pos(50, 50), 8)
	if len(result) != 4 {
		t.Fatalf("expected 4 nearby entities, got %d: %v", len(result), result)
	}

	for _, id := range result {
		if id == "far" {
			t.Error("'far' should not be in radius query results")
		}
	}
}

func TestGridHash_NegativeCoordinates(t *testing.T) {
	h := NewGridSpatialHash(SpatialConfig{CellSizeM: 10})

	mustUpdate(t, h, "p1", Pos(-5, -5))
	mustUpdate(t, h, "p2", Pos(-15, -15))

	result := h.QueryRadius(Pos(-5, -5), 10)
	if len(result) != 1 {
		t.Fatalf("expected 1 result with negative coords, got %d", len(result))
	}
	if result[0] != "p1" {
		t.Errorf("expected 'p1', got %q", result[0])
	}
}

func TestGridHash_CellBoundary(t *testing.T) {
	// x=10 falls in cell 1 (Floor(10/10) = 1).
	coord := toCellCoord(Pos(10, 0), 10)
	if coord.X != 1 || coord.Z != 0 {
		t.Errorf("cell for (10,0) = (%d,%d), want (1,0)", coord.X, coord.Z)
	}

	// x=-0.01 falls in cell -1 (Floor(-0.01/10) = Floor(-0.001) = -1).
	coord = toCellCoord(Pos(-0.01, 0), 10)
	if coord.X != -1 || coord.Z != 0 {
		t.Errorf("cell for (-0.01,0) = (%d,%d), want (-1,0)", coord.X, coord.Z)
	}

	// x=0 falls in cell 0 (Floor(0/10) = 0).
	coord = toCellCoord(Pos(0, 0), 10)
	if coord.X != 0 || coord.Z != 0 {
		t.Errorf("cell for (0,0) = (%d,%d), want (0,0)", coord.X, coord.Z)
	}
}

func TestGridHash_DuplicateUpdateDeterministic(t *testing.T) {
	h := NewGridSpatialHash(SpatialConfig{CellSizeM: 10})

	for i := 0; i < 10; i++ {
		mustUpdate(t, h, "p1", Pos(5, 5))
	}

	if h.Len() != 1 {
		t.Errorf("expected 1 entity after duplicate updates, got %d", h.Len())
	}

	result := h.QueryRadius(Pos(5, 5), 5)
	if len(result) != 1 {
		t.Errorf("expected 1 result, got %d", len(result))
	}
}

func TestGridHash_Clear(t *testing.T) {
	h := NewGridSpatialHash(SpatialConfig{CellSizeM: 10})
	mustUpdate(t, h, "p1", Pos(0, 0))
	mustUpdate(t, h, "p2", Pos(10, 10))
	mustUpdate(t, h, "p3", Pos(20, 20))

	if h.Len() != 3 {
		t.Fatalf("before clear: expected 3, got %d", h.Len())
	}

	h.Clear()

	if h.Len() != 0 {
		t.Errorf("after clear: expected 0, got %d", h.Len())
	}

	result := h.QueryRadius(Pos(10, 10), 50)
	if len(result) != 0 {
		t.Errorf("query after clear: expected 0, got %d", len(result))
	}
}

func TestGridHash_Get(t *testing.T) {
	h := NewGridSpatialHash(SpatialConfig{CellSizeM: 10})
	mustUpdate(t, h, "p1", Pos(5, 7))

	pos, ok := h.Get("p1")
	if !ok {
		t.Fatal("Get should find existing entity")
	}
	if pos.X != 5 || pos.Z != 7 {
		t.Errorf("position = (%.1f, %.1f), want (5, 7)", pos.X, pos.Z)
	}
}

func TestGridHash_GetNonexistent(t *testing.T) {
	h := NewGridSpatialHash(SpatialConfig{CellSizeM: 10})

	_, ok := h.Get("ghost")
	if ok {
		t.Error("Get should return false for nonexistent entity")
	}
}

func TestGridHash_ManyEntities(t *testing.T) {
	h := NewGridSpatialHash(SpatialConfig{CellSizeM: 10})

	for i := 0; i < 200; i++ {
		x := float32(i % 100)
		z := float32(i / 100)
		mustUpdate(t, h, EntityID(fmt.Sprintf("p-%03d", i)), Pos(x, z))
	}

	if h.Len() != 200 {
		t.Errorf("expected 200 entities, got %d", h.Len())
	}

	result := h.QueryRadius(Pos(0, 0), 15)
	if len(result) >= 200 {
		t.Errorf("small radius query returned %d, expected < 200", len(result))
	}
	if len(result) == 0 {
		t.Error("small radius query should return at least 1 entity")
	}
}

func TestGridHash_RemoveCleansUpEmptyCells(t *testing.T) {
	h := NewGridSpatialHash(SpatialConfig{CellSizeM: 10})

	// Insert into a unique cell then remove — cell map entry should be deleted.
	mustUpdate(t, h, "p1", Pos(55, 55)) // cell (5,5)
	h.Remove("p1")

	// Internal check: no cells should remain.
	if len(h.cells) != 0 {
		t.Errorf("expected 0 cells after removing only entity, got %d", len(h.cells))
	}
}

func TestGridHash_CellSize(t *testing.T) {
	h := NewGridSpatialHash(SpatialConfig{CellSizeM: 5.5})
	if h.CellSize() != 5.5 {
		t.Errorf("CellSize = %.1f, want 5.5", h.CellSize())
	}
}

func TestGridHash_QueryRadiusEmptyIndex(t *testing.T) {
	h := NewGridSpatialHash(SpatialConfig{CellSizeM: 10})

	result := h.QueryRadius(Pos(0, 0), 100)
	if len(result) != 0 {
		t.Errorf("empty index query should return 0, got %d", len(result))
	}
}

func TestGridHash_DistancePrecision(t *testing.T) {
	h := NewGridSpatialHash(SpatialConfig{CellSizeM: 10})

	// Place entity at distance that would be affected by float32 precision.
	mustUpdate(t, h, "p1", Pos(3.0000001, 4.0000001))

	// Exact distance from origin: sqrt(3^2 + 4^2) = 5.
	result := h.QueryRadius(Pos(0, 0), 5.0)
	if len(result) != 1 {
		t.Errorf("expected 1 result near radius boundary, got %d", len(result))
	}

	// Just under should also work.
	result = h.QueryRadius(Pos(0, 0), 5.0001)
	if len(result) != 1 {
		t.Errorf("expected 1 result with small margin, got %d", len(result))
	}
}

func TestGridHash_MathEdgeCases(t *testing.T) {
	h := NewGridSpatialHash(SpatialConfig{CellSizeM: 10})

	// Zero position is valid.
	if err := h.Update("p1", Pos(0, 0)); err != nil {
		t.Fatalf("zero position should be valid: %v", err)
	}

	// MaxFloat32 is rejected as out of bounds.
	err := h.Update("p2", Pos(float32(math.MaxFloat32), 0))
	if err == nil {
		t.Error("MaxFloat32 position should be rejected")
	}

	// -MaxFloat32 is rejected as out of bounds.
	err = h.Update("p3", Pos(-float32(math.MaxFloat32), 0))
	if err == nil {
		t.Error("-MaxFloat32 position should be rejected")
	}
}

func TestGridHash_InvalidPositions(t *testing.T) {
	h := NewGridSpatialHash(SpatialConfig{CellSizeM: 10, MaxWorldCoord: 1000})

	tests := []struct {
		name string
		pos  EntityPosition
	}{
		{"NaN_X", Pos(float32(math.NaN()), 0)},
		{"NaN_Z", Pos(0, float32(math.NaN()))},
		{"Inf_X", Pos(float32(math.Inf(1)), 0)},
		{"NegInf_Z", Pos(0, float32(math.Inf(-1)))},
		{"oversized_X", Pos(1001, 0)},
		{"oversized_Z", Pos(0, 1001)},
		{"oversized_neg_X", Pos(-1001, 0)},
		{"oversized_neg_Z", Pos(0, -1001)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := h.Update("test", tt.pos)
			if err == nil {
				t.Error("expected error for invalid position")
			}
		})
	}
}

func TestGridHash_QueryRadiusInvalidPosition(t *testing.T) {
	h := NewGridSpatialHash(SpatialConfig{CellSizeM: 10, MaxWorldCoord: 100})

	result := h.QueryRadius(Pos(float32(math.NaN()), 0), 10)
	if result != nil {
		t.Error("NaN query position should return nil")
	}

	result = h.QueryRadius(Pos(float32(math.Inf(1)), 0), 10)
	if result != nil {
		t.Error("Inf query position should return nil")
	}

	result = h.QueryRadius(Pos(200, 0), 10)
	if result != nil {
		t.Error("out-of-bounds query position should return nil")
	}
}

func TestGridHash_MaxWorldCoordDefault(t *testing.T) {
	// MaxWorldCoord not set → uses default (100000).
	h := NewGridSpatialHash(SpatialConfig{CellSizeM: 10})
	if err := h.Update("p1", Pos(50000, 50000)); err != nil {
		t.Errorf("position within default bounds should be valid: %v", err)
	}
	if err := h.Update("p2", Pos(200000, 0)); err == nil {
		t.Error("position exceeding default bounds should be rejected")
	}
}

// Benchmarks

func BenchmarkGridHash_Update(b *testing.B) {
	h := NewGridSpatialHash(SpatialConfig{CellSizeM: 10})
	for i := 0; i < b.N; i++ {
		id := EntityID(fmt.Sprintf("p-%d", i%200))
		x := float32(i%100) + 0.5
		z := float32((i/100)%100) + 0.5
		_ = h.Update(id, Pos(x, z))
	}
}

func BenchmarkGridHash_QueryRadius(b *testing.B) {
	h := NewGridSpatialHash(SpatialConfig{CellSizeM: 10})
	for i := 0; i < 200; i++ {
		x := float32(i % 20) * 5
		z := float32(i / 20) * 5
		_ = h.Update(EntityID(fmt.Sprintf("p-%d", i)), Pos(x, z))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		x := float32(i%100) * 0.5
		_ = h.QueryRadius(Pos(x, x), 30)
	}
}

func BenchmarkGridHash_Remove(b *testing.B) {
	h := NewGridSpatialHash(SpatialConfig{CellSizeM: 10})
	for i := 0; i < 200; i++ {
		_ = h.Update(EntityID(fmt.Sprintf("p-%d", i)), Pos(float32(i), float32(i)))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.Remove(EntityID(fmt.Sprintf("p-%d", i%200)))
	}
}
