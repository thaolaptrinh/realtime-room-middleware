package interest

import (
	"testing"

	"github.com/thaonguyen/realtime-room-middleware/internal/game/spatial"
)

func TestQueryVisiblePlayers(t *testing.T) {
	idx := spatial.NewGridSpatialHash(spatial.SpatialConfig{CellSizeM: 10})
	_ = idx.Update("viewer", spatial.Pos(0, 0))
	_ = idx.Update("nearby", spatial.Pos(5, 5))
	_ = idx.Update("far", spatial.Pos(100, 100))

	mgr := NewInterestManager(InterestConfig{VisualRadiusM: 30})
	set := mgr.QueryVisiblePlayers(idx, spatial.Pos(0, 0), "viewer")

	if len(set.VisiblePlayers) != 1 {
		t.Fatalf("expected 1 visible player, got %d", len(set.VisiblePlayers))
	}
	if set.VisiblePlayers[0] != "nearby" {
		t.Errorf("expected 'nearby', got %q", set.VisiblePlayers[0])
	}
}

func TestQueryVisiblePlayers_ExcludesSelf(t *testing.T) {
	idx := spatial.NewGridSpatialHash(spatial.SpatialConfig{CellSizeM: 10})
	_ = idx.Update("viewer", spatial.Pos(0, 0))

	mgr := NewInterestManager(InterestConfig{VisualRadiusM: 30})
	set := mgr.QueryVisiblePlayers(idx, spatial.Pos(0, 0), "viewer")

	if len(set.VisiblePlayers) != 0 {
		t.Errorf("viewer should not see self, got %d players", len(set.VisiblePlayers))
	}
}

func TestQueryVisiblePlayers_NoOneNearby(t *testing.T) {
	idx := spatial.NewGridSpatialHash(spatial.SpatialConfig{CellSizeM: 10})
	_ = idx.Update("viewer", spatial.Pos(0, 0))
	_ = idx.Update("far", spatial.Pos(100, 100))

	mgr := NewInterestManager(InterestConfig{VisualRadiusM: 30})
	set := mgr.QueryVisiblePlayers(idx, spatial.Pos(0, 0), "viewer")

	if len(set.VisiblePlayers) != 0 {
		t.Errorf("expected 0 visible players, got %d", len(set.VisiblePlayers))
	}
}

func TestQueryVisiblePlayers_MultipleNearby(t *testing.T) {
	idx := spatial.NewGridSpatialHash(spatial.SpatialConfig{CellSizeM: 10})
	_ = idx.Update("viewer", spatial.Pos(50, 50))
	_ = idx.Update("p1", spatial.Pos(45, 50))
	_ = idx.Update("p2", spatial.Pos(55, 50))
	_ = idx.Update("p3", spatial.Pos(50, 55))
	_ = idx.Update("far", spatial.Pos(0, 0))

	mgr := NewInterestManager(InterestConfig{VisualRadiusM: 10})
	set := mgr.QueryVisiblePlayers(idx, spatial.Pos(50, 50), "viewer")

	if len(set.VisiblePlayers) != 3 {
		t.Fatalf("expected 3 visible players, got %d: %v", len(set.VisiblePlayers), set.VisiblePlayers)
	}

	found := map[spatial.EntityID]bool{}
	for _, id := range set.VisiblePlayers {
		found[id] = true
	}
	if !found["p1"] || !found["p2"] || !found["p3"] {
		t.Errorf("expected p1, p2, p3; got %v", set.VisiblePlayers)
	}
	if found["far"] {
		t.Error("'far' should not be visible")
	}
}

func TestDefaultInterestConfig(t *testing.T) {
	cfg := DefaultInterestConfig()
	if cfg.VisualRadiusM != 30 {
		t.Errorf("VisualRadiusM = %.1f, want 30", cfg.VisualRadiusM)
	}
	if cfg.ObjectRadiusM != 30 {
		t.Errorf("ObjectRadiusM = %.1f, want 30", cfg.ObjectRadiusM)
	}
	if cfg.VoiceRadiusM != 30 {
		t.Errorf("VoiceRadiusM = %.1f, want 30", cfg.VoiceRadiusM)
	}
}

func TestQueryVisiblePlayers_EmptyIndex(t *testing.T) {
	idx := spatial.NewGridSpatialHash(spatial.SpatialConfig{CellSizeM: 10})

	mgr := NewInterestManager(InterestConfig{VisualRadiusM: 30})
	set := mgr.QueryVisiblePlayers(idx, spatial.Pos(0, 0), "viewer")

	if len(set.VisiblePlayers) != 0 {
		t.Errorf("empty index should return 0, got %d", len(set.VisiblePlayers))
	}
}
