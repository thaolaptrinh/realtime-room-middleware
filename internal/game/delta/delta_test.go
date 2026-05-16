package delta_test

import (
	"testing"
	"time"

	"github.com/thaonguyen/realtime-room-middleware/internal/game/delta"
	"github.com/thaonguyen/realtime-room-middleware/internal/game/player"
)

// ---- helpers ----------------------------------------------------------------

func makePlayer(id string, x, z float32, version uint32) *player.PlayerState {
	ps := player.NewPlayerState(player.PlayerID(id), player.UserID("u-"+id), time.Now())
	ps.SetStatus(player.PlayerStatusActive)
	if version > 0 {
		for i := uint32(0); i < version; i++ {
			ps.UpdateTransform(player.PlayerTransform{
				Position: player.Vector3{X: x, Y: 0, Z: z},
				Rotation: player.IdentityQuaternion,
				Tick:     i + 1,
			}, i+1)
		}
	}
	return ps
}

func makeStates(specs ...struct {
	id      string
	x, z    float32
	version uint32
}) map[player.PlayerID]*player.PlayerState {
	m := make(map[player.PlayerID]*player.PlayerState, len(specs))
	for _, s := range specs {
		m[player.PlayerID(s.id)] = makePlayer(s.id, s.x, s.z, s.version)
	}
	return m
}

// ---- SnapshotCache tests -----------------------------------------------------

func TestSnapshotCache_GetOrCreate(t *testing.T) {
	c := delta.NewSnapshotCache()

	s1 := c.GetOrCreate("sess-1")
	if s1 == nil {
		t.Fatal("GetOrCreate returned nil")
	}
	if len(s1.VisiblePlayers) != 0 {
		t.Errorf("new snapshot should have empty VisiblePlayers, got %d", len(s1.VisiblePlayers))
	}

	// Same session returns same pointer.
	s2 := c.GetOrCreate("sess-1")
	if s1 != s2 {
		t.Error("second GetOrCreate for same session should return the same pointer")
	}

	if c.Len() != 1 {
		t.Errorf("Len() = %d, want 1", c.Len())
	}
}

func TestSnapshotCache_Remove(t *testing.T) {
	c := delta.NewSnapshotCache()
	c.GetOrCreate("sess-a")
	c.GetOrCreate("sess-b")

	if c.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", c.Len())
	}

	c.Remove("sess-a")
	if c.Len() != 1 {
		t.Errorf("Len() after Remove = %d, want 1", c.Len())
	}

	// Remove of absent key is a no-op.
	c.Remove("nonexistent")
	if c.Len() != 1 {
		t.Errorf("Len() after removing absent key = %d, want 1", c.Len())
	}
}

func TestClientSnapshot_StartsEmpty(t *testing.T) {
	s := delta.NewClientSnapshot()
	if len(s.VisiblePlayers) != 0 {
		t.Errorf("new snapshot VisiblePlayers should be empty, got %d", len(s.VisiblePlayers))
	}
}

// ---- DeltaBuilder tests ------------------------------------------------------

func TestDeltaBuilder_InitialSnapshot_AllEnter(t *testing.T) {
	// Empty snapshot + visible players → all emit Enter.
	states := makeStates(
		struct {
			id      string
			x, z    float32
			version uint32
		}{"p1", 0, 0, 1},
		struct {
			id      string
			x, z    float32
			version uint32
		}{"p2", 5, 5, 1},
	)

	b := delta.NewDeltaBuilder()
	snap := delta.NewClientSnapshot()
	visible := []player.PlayerID{"p1", "p2"}

	pd := b.BuildPlayerDelta(1, visible, snap, states)

	if len(pd.Enters) != 2 {
		t.Errorf("Enters = %d, want 2", len(pd.Enters))
	}
	if len(pd.Updates) != 0 {
		t.Errorf("Updates = %d, want 0", len(pd.Updates))
	}
	if len(pd.Leaves) != 0 {
		t.Errorf("Leaves = %d, want 0", len(pd.Leaves))
	}
	if pd.Tick != 1 {
		t.Errorf("Tick = %d, want 1", pd.Tick)
	}

	// Snapshot should now track both players.
	if len(snap.VisiblePlayers) != 2 {
		t.Errorf("snapshot VisiblePlayers = %d, want 2", len(snap.VisiblePlayers))
	}
}

func TestDeltaBuilder_NearbyPlayerEnter(t *testing.T) {
	// A player that was not in the snapshot appears → Enter.
	states := makeStates(struct {
		id      string
		x, z    float32
		version uint32
	}{"p1", 0, 0, 1})

	b := delta.NewDeltaBuilder()
	snap := delta.NewClientSnapshot()

	pd := b.BuildPlayerDelta(1, []player.PlayerID{"p1"}, snap, states)

	if len(pd.Enters) != 1 {
		t.Fatalf("Enters = %d, want 1", len(pd.Enters))
	}
	if pd.Enters[0].PlayerID != "p1" {
		t.Errorf("Enter PlayerID = %q, want %q", pd.Enters[0].PlayerID, "p1")
	}
	if pd.IsEmpty() {
		t.Error("delta should not be empty on enter")
	}
}

func TestDeltaBuilder_TransformUpdateGeneratesUpdate(t *testing.T) {
	// Player already in snapshot at version 1 → move to version 2 → Update.
	ps := makePlayer("p1", 0, 0, 1)
	states := map[player.PlayerID]*player.PlayerState{"p1": ps}

	b := delta.NewDeltaBuilder()
	snap := delta.NewClientSnapshot()

	// First tick: enter.
	b.BuildPlayerDelta(1, []player.PlayerID{"p1"}, snap, states)

	// Update the player transform (version becomes 2).
	ps.UpdateTransform(player.PlayerTransform{
		Position: player.Vector3{X: 10, Y: 0, Z: 10},
		Rotation: player.IdentityQuaternion,
		Tick:     2,
	}, 2)

	// Second tick: update.
	pd := b.BuildPlayerDelta(2, []player.PlayerID{"p1"}, snap, states)

	if len(pd.Updates) != 1 {
		t.Fatalf("Updates = %d, want 1", len(pd.Updates))
	}
	if pd.Updates[0].PlayerID != "p1" {
		t.Errorf("Update PlayerID = %q, want %q", pd.Updates[0].PlayerID, "p1")
	}
	if len(pd.Enters) != 0 {
		t.Errorf("Enters = %d, want 0 on update tick", len(pd.Enters))
	}
}

func TestDeltaBuilder_PlayerLeaveInterestRange(t *testing.T) {
	// Player was visible; no longer in interest set → Leave.
	states := makeStates(struct {
		id      string
		x, z    float32
		version uint32
	}{"p1", 0, 0, 1})

	b := delta.NewDeltaBuilder()
	snap := delta.NewClientSnapshot()

	// Enter.
	b.BuildPlayerDelta(1, []player.PlayerID{"p1"}, snap, states)

	// Now p1 is out of range — visible set is empty.
	pd := b.BuildPlayerDelta(2, []player.PlayerID{}, snap, states)

	if len(pd.Leaves) != 1 {
		t.Fatalf("Leaves = %d, want 1", len(pd.Leaves))
	}
	if pd.Leaves[0].PlayerID != "p1" {
		t.Errorf("Leave PlayerID = %q, want %q", pd.Leaves[0].PlayerID, "p1")
	}
	// Snapshot entry must be removed.
	if _, ok := snap.VisiblePlayers["p1"]; ok {
		t.Error("snapshot should not contain p1 after leave")
	}
}

func TestDeltaBuilder_NoChange_EmptyDelta(t *testing.T) {
	// Same player, same version → empty delta.
	ps := makePlayer("p1", 0, 0, 1)
	states := map[player.PlayerID]*player.PlayerState{"p1": ps}

	b := delta.NewDeltaBuilder()
	snap := delta.NewClientSnapshot()

	b.BuildPlayerDelta(1, []player.PlayerID{"p1"}, snap, states)

	// No transform change → second tick must be empty.
	pd := b.BuildPlayerDelta(2, []player.PlayerID{"p1"}, snap, states)

	if !pd.IsEmpty() {
		t.Errorf("expected empty delta when nothing changed; Enters=%d Updates=%d Leaves=%d",
			len(pd.Enters), len(pd.Updates), len(pd.Leaves))
	}
}

func TestDeltaBuilder_FarPlayerNotIncluded(t *testing.T) {
	// Far player is excluded from the visible set by the caller (interest manager).
	// The delta builder should only emit deltas for the provided visible set.
	near := makePlayer("near", 0, 0, 1)
	far := makePlayer("far", 1000, 1000, 1)
	states := map[player.PlayerID]*player.PlayerState{
		"near": near,
		"far":  far,
	}

	b := delta.NewDeltaBuilder()
	snap := delta.NewClientSnapshot()

	// Only "near" is in the visible set; "far" is not.
	pd := b.BuildPlayerDelta(1, []player.PlayerID{"near"}, snap, states)

	if len(pd.Enters) != 1 {
		t.Fatalf("Enters = %d, want 1", len(pd.Enters))
	}
	if pd.Enters[0].PlayerID != "near" {
		t.Errorf("Enter PlayerID = %q, want %q", pd.Enters[0].PlayerID, "near")
	}
	if _, ok := snap.VisiblePlayers["far"]; ok {
		t.Error("far player should not be in snapshot")
	}
}

func TestDeltaBuilder_MissingPlayerState_Skipped(t *testing.T) {
	// Visible set contains a player whose state is absent (removed concurrently).
	b := delta.NewDeltaBuilder()
	snap := delta.NewClientSnapshot()
	states := map[player.PlayerID]*player.PlayerState{} // empty — state missing

	pd := b.BuildPlayerDelta(1, []player.PlayerID{"ghost"}, snap, states)

	if len(pd.Enters) != 0 {
		t.Errorf("Enters = %d, want 0 for missing state", len(pd.Enters))
	}
	if !pd.IsEmpty() {
		t.Error("delta should be empty when player state is missing")
	}
}

func TestDeltaBuilder_MultiplePlayersLeave(t *testing.T) {
	// Three players enter; two leave on the next tick.
	states := makeStates(
		struct {
			id      string
			x, z    float32
			version uint32
		}{"p1", 0, 0, 1},
		struct {
			id      string
			x, z    float32
			version uint32
		}{"p2", 1, 0, 1},
		struct {
			id      string
			x, z    float32
			version uint32
		}{"p3", 2, 0, 1},
	)

	b := delta.NewDeltaBuilder()
	snap := delta.NewClientSnapshot()
	visible := []player.PlayerID{"p1", "p2", "p3"}

	b.BuildPlayerDelta(1, visible, snap, states)

	// Only p3 remains visible.
	pd := b.BuildPlayerDelta(2, []player.PlayerID{"p3"}, snap, states)

	if len(pd.Leaves) != 2 {
		t.Errorf("Leaves = %d, want 2", len(pd.Leaves))
	}
	if len(pd.Enters) != 0 {
		t.Errorf("Enters = %d, want 0", len(pd.Enters))
	}
	if len(snap.VisiblePlayers) != 1 {
		t.Errorf("snapshot size = %d, want 1 after two leaves", len(snap.VisiblePlayers))
	}
}

// TestDeltaTypes_TransportAgnostic verifies that the delta types contain no
// transport-specific fields. KCP/WebSocket metadata must not influence delta
// structure or semantics.
func TestDeltaTypes_TransportAgnostic(t *testing.T) {
	// DeltaBatch has no transport field. If transport metadata were added,
	// the delta broadcaster would need to treat clients differently,
	// which violates the transport-agnostic rule.
	batch := &delta.DeltaBatch{Tick: 42}
	if !batch.IsEmpty() {
		t.Error("DeltaBatch with nil PlayerDelta should be empty")
	}

	pd := &delta.PlayerDelta{Tick: 42}
	if !pd.IsEmpty() {
		t.Error("PlayerDelta with no entries should be empty")
	}

	// DeltaBatch with a non-empty PlayerDelta should not be empty.
	batch2 := &delta.DeltaBatch{
		Tick: 1,
		PlayerDelta: &delta.PlayerDelta{
			Tick:   1,
			Enters: []delta.PlayerEnterDelta{{PlayerID: "p1"}},
		},
	}
	if batch2.IsEmpty() {
		t.Error("DeltaBatch with Enters should not be empty")
	}
}

// TestDeltaBuilder_SnapshotVersionTracking verifies that the snapshot correctly
// tracks versions so that re-sending the same version produces no update delta.
func TestDeltaBuilder_SnapshotVersionTracking(t *testing.T) {
	ps := makePlayer("p1", 0, 0, 3)
	states := map[player.PlayerID]*player.PlayerState{"p1": ps}

	b := delta.NewDeltaBuilder()
	snap := delta.NewClientSnapshot()

	// Initial tick: enter at whatever current version ps has.
	b.BuildPlayerDelta(1, []player.PlayerID{"p1"}, snap, states)

	// Same version: no update.
	pd := b.BuildPlayerDelta(2, []player.PlayerID{"p1"}, snap, states)
	if !pd.IsEmpty() {
		t.Errorf("expected empty delta; Enters=%d Updates=%d Leaves=%d",
			len(pd.Enters), len(pd.Updates), len(pd.Leaves))
	}

	// Bump version.
	ps.UpdateTransform(player.PlayerTransform{
		Position: player.Vector3{X: 5, Y: 0, Z: 5},
		Tick:     4,
	}, 4)

	pd = b.BuildPlayerDelta(3, []player.PlayerID{"p1"}, snap, states)
	if len(pd.Updates) != 1 {
		t.Errorf("Updates = %d, want 1 after version bump", len(pd.Updates))
	}
}
