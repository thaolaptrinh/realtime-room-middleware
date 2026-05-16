package player

import "time"

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

// PlayerState is the game runtime record for a player in a room.
//
// Phase 1: identity and join metadata only.
// Position, rotation, animation state, dirty mask, and version are deferred
// to Milestone 3 (Player Sync).
type PlayerState struct {
	ID       PlayerID
	UserID   UserID
	Status   PlayerStatus
	JoinedAt time.Time
	// Future fields (Milestone 3):
	// Position  Vec2
	// Rotation  float32
	// AnimState uint16
	// Version   uint32
	// Dirty     DirtyMask
}
