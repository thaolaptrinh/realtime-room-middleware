package delta

import "github.com/thaonguyen/realtime-room-middleware/internal/game/player"

// DeltaType classifies a single player visibility change in a broadcast tick.
type DeltaType uint8

const (
	DeltaTypeEnter  DeltaType = 1 // player newly entered the viewer's interest range
	DeltaTypeUpdate DeltaType = 2 // player still visible but transform version changed
	DeltaTypeLeave  DeltaType = 3 // player left the viewer's interest range
)

// PlayerEnterDelta is sent when a player enters the viewer's interest range for the first time.
type PlayerEnterDelta struct {
	PlayerID  player.PlayerID
	Transform player.PlayerTransform
	Version   uint32
}

// PlayerUpdateDelta is sent when a visible player's transform has changed since the last broadcast.
type PlayerUpdateDelta struct {
	PlayerID  player.PlayerID
	Transform player.PlayerTransform
	Version   uint32
}

// PlayerLeaveDelta is sent when a player leaves the viewer's interest range.
type PlayerLeaveDelta struct {
	PlayerID player.PlayerID
}

// PlayerDelta aggregates all player visibility changes for a single viewer in one broadcast tick.
type PlayerDelta struct {
	Tick    uint32
	Enters  []PlayerEnterDelta
	Updates []PlayerUpdateDelta
	Leaves  []PlayerLeaveDelta
}

// IsEmpty returns true if the delta contains no changes.
func (d *PlayerDelta) IsEmpty() bool {
	return len(d.Enters) == 0 && len(d.Updates) == 0 && len(d.Leaves) == 0
}

// DeltaBatch is the complete set of delta changes for one client in a single broadcast tick.
// Future milestones will add ObjectDelta and VoiceGroupDelta fields.
type DeltaBatch struct {
	Tick        uint32
	PlayerDelta *PlayerDelta
}

// IsEmpty returns true if the batch contains no changes across all delta types.
func (b *DeltaBatch) IsEmpty() bool {
	return b.PlayerDelta == nil || b.PlayerDelta.IsEmpty()
}
