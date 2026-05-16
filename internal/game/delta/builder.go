package delta

import "github.com/thaonguyen/realtime-room-middleware/internal/game/player"

// DeltaBuilder computes per-client PlayerDelta values from current room state.
//
// It is stateless. The caller owns the ClientSnapshot and passes it in.
// BuildPlayerDelta updates the snapshot in place to reflect what was sent.
//
// Must only be called from the room loop goroutine.
type DeltaBuilder struct{}

// NewDeltaBuilder creates a DeltaBuilder.
func NewDeltaBuilder() *DeltaBuilder {
	return &DeltaBuilder{}
}

// BuildPlayerDelta computes the delta for one viewer for a single broadcast tick.
//
// visiblePlayers is the current interest set for the viewer (from spatial/interest query),
// excluding the viewer themselves.
// snapshot is the viewer's last-sent state; it is updated in place.
// playerStates is the current authoritative player map (read via Snapshot()).
//
// Returns a PlayerDelta with zero-length (not nil) slices when there are no changes.
// The snapshot is updated before returning so it reflects the state that was sent.
func (b *DeltaBuilder) BuildPlayerDelta(
	tick uint32,
	visiblePlayers []player.PlayerID,
	snapshot *ClientSnapshot,
	playerStates map[player.PlayerID]*player.PlayerState,
) *PlayerDelta {
	pd := &PlayerDelta{Tick: tick}

	// Build a fast lookup set for currently visible players.
	nowVisible := make(map[player.PlayerID]struct{}, len(visiblePlayers))
	for _, pid := range visiblePlayers {
		nowVisible[pid] = struct{}{}
	}

	// Enters and updates: check each currently visible player against the snapshot.
	for _, pid := range visiblePlayers {
		ps, ok := playerStates[pid]
		if !ok {
			// Player state unavailable (removed concurrently before snapshot). Skip.
			continue
		}
		transform, version := ps.Snapshot()

		lastVersion, wasSeen := snapshot.VisiblePlayers[pid]
		switch {
		case !wasSeen:
			pd.Enters = append(pd.Enters, PlayerEnterDelta{
				PlayerID:  pid,
				Transform: transform,
				Version:   version,
			})
			snapshot.VisiblePlayers[pid] = version

		case version != lastVersion:
			pd.Updates = append(pd.Updates, PlayerUpdateDelta{
				PlayerID:  pid,
				Transform: transform,
				Version:   version,
			})
			snapshot.VisiblePlayers[pid] = version

		// version == lastVersion: no change; skip.
		}
	}

	// Leaves: find snapshot entries that are no longer in the visible set.
	for pid := range snapshot.VisiblePlayers {
		if _, ok := nowVisible[pid]; !ok {
			pd.Leaves = append(pd.Leaves, PlayerLeaveDelta{PlayerID: pid})
		}
	}
	for _, lv := range pd.Leaves {
		delete(snapshot.VisiblePlayers, lv.PlayerID)
	}

	return pd
}
