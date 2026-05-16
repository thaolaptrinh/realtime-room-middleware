package delta

import (
	"github.com/thaonguyen/realtime-room-middleware/internal/game/object"
	"github.com/thaonguyen/realtime-room-middleware/internal/game/player"
)

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
type DeltaBatch struct {
	Tick        uint32
	PlayerDelta *PlayerDelta
	ObjectDelta *ObjectDelta // nil until object transport wiring (Milestone 4).
}

// IsEmpty returns true if the batch contains no changes across all delta types.
func (b *DeltaBatch) IsEmpty() bool {
	playerEmpty := b.PlayerDelta == nil || b.PlayerDelta.IsEmpty()
	objectEmpty := b.ObjectDelta == nil || b.ObjectDelta.IsEmpty()
	return playerEmpty && objectEmpty
}

// --- Object delta placeholder types (Milestone 4) ----------------------------

// ObjectEnterDelta is sent when an object enters a viewer's interest range for the first time.
type ObjectEnterDelta struct {
	ObjectID  object.ObjectID
	Kind      object.ObjectKind
	Transform object.ObjectTransform
	Lock      object.LockState
	Version   uint32
}

// ObjectUpdateDelta is sent when a visible object's state has changed since the last broadcast.
// Nil pointer fields indicate no change for that sub-state.
type ObjectUpdateDelta struct {
	ObjectID  object.ObjectID
	Transform *object.ObjectTransform // nil if transform unchanged
	Lock      *object.LockState       // nil if lock state unchanged
	Version   uint32
}

// ObjectLeaveDelta is sent when an object leaves a viewer's interest range.
type ObjectLeaveDelta struct {
	ObjectID object.ObjectID
}

// ObjectLockDelta is sent immediately when a lock is granted, released, or expires.
// It may be sent outside the normal broadcast cadence for low-latency lock feedback.
type ObjectLockDelta struct {
	ObjectID object.ObjectID
	Lock     object.LockState
	Version  uint32
}

// ObjectDelta aggregates all object visibility and lock changes for one viewer in a broadcast tick.
type ObjectDelta struct {
	Tick    uint32
	Enters  []ObjectEnterDelta
	Updates []ObjectUpdateDelta
	Leaves  []ObjectLeaveDelta
	Locks   []ObjectLockDelta // Lock state changes (grant, release, expire).
}

// IsEmpty returns true if the delta contains no changes.
func (d *ObjectDelta) IsEmpty() bool {
	return len(d.Enters) == 0 && len(d.Updates) == 0 && len(d.Leaves) == 0 && len(d.Locks) == 0
}
