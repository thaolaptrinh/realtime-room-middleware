package player

import (
	"sync"
	"time"
)

// PlayerID is the server-assigned in-room player identifier.
type PlayerID string

// UserID is the externally authenticated user identity.
// Matches session.UserID by value; typed separately to keep the packages independent.
type UserID string

// PlayerStatus is the lifecycle state of a player within a room.
type PlayerStatus int

const (
	// PlayerStatusJoining — CmdJoin received, not yet confirmed.
	PlayerStatusJoining PlayerStatus = iota
	// PlayerStatusActive — join confirmed; player is in the room.
	PlayerStatusActive
	// PlayerStatusLeaving — CmdLeave received; pending cleanup.
	PlayerStatusLeaving
	// PlayerStatusGone — removed from room state.
	PlayerStatusGone
)

func (s PlayerStatus) String() string {
	switch s {
	case PlayerStatusJoining:
		return "joining"
	case PlayerStatusActive:
		return "active"
	case PlayerStatusLeaving:
		return "leaving"
	case PlayerStatusGone:
		return "gone"
	default:
		return "unknown"
	}
}

// PlayerState is the authoritative game runtime record for a player in a room.
//
// Only the room loop may mutate the transform fields (Position, Rotation).
// Other goroutines may read the snapshot via Snapshot() to obtain a copy.
type PlayerState struct {
	ID        PlayerID
	UserID    UserID
	Status    PlayerStatus
	JoinedAt  time.Time
	Transform PlayerTransform // current authoritative transform
	Version   uint32          // increments on each transform update
	LastSeen  time.Time       // last update (movement or keep-alive)

	mu sync.RWMutex // protects Transform, Version, LastSeen
}

// NewPlayerState creates a new PlayerState in Joining status.
// The transform is initialized to origin with identity rotation.
func NewPlayerState(id PlayerID, userID UserID, now time.Time) *PlayerState {
	return &PlayerState{
		ID:       id,
		UserID:   userID,
		Status:   PlayerStatusJoining,
		JoinedAt: now,
		Transform: PlayerTransform{
			Position: Vector3{},
			Rotation: IdentityQuaternion,
			Tick:     0,
		},
		Version:  0,
		LastSeen: now,
	}
}

// Snapshot returns a copy of the player's transform and version.
// Safe to call from any goroutine; returns a consistent snapshot.
func (p *PlayerState) Snapshot() (PlayerTransform, uint32) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.Transform, p.Version
}

// UpdateTransform atomically updates the player's transform and version.
// Must only be called by the room loop.
func (p *PlayerState) UpdateTransform(transform PlayerTransform, tick uint32) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Transform = transform
	p.Transform.Tick = tick
	p.Version++
	p.LastSeen = time.Now()
}

// MarkStatus changes the player's lifecycle status.
// Must only be called by the room loop.
func (p *PlayerState) MarkStatus(status PlayerStatus) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Status = status
}

// SetStatus sets the player's lifecycle status directly (bypasses MarkStatus).
// Must only be called by the room loop.
func (p *PlayerState) SetStatus(status PlayerStatus) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Status = status
}

// GetStatus returns the current status.
// Safe to call from any goroutine.
func (p *PlayerState) GetStatus() PlayerStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.Status
}
