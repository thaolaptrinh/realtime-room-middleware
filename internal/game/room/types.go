package room

import "time"

// LogicalRoomID is the product-facing room identifier (e.g., "expo-room-a").
// Multiple physical instances may share one logical ID.
type LogicalRoomID string

// RoomInstanceID is the physical runtime instance identifier (e.g., "expo-room-a-0001").
// Each running room has a unique instance ID.
type RoomInstanceID string

// PlayerID identifies a player within a room.
type PlayerID string

// SessionID identifies a transport session (KCP or WebSocket).
type SessionID string

// UserID is the externally authenticated user identity carried in join commands.
// Matches session.UserID and player.UserID by value; typed separately to avoid
// cross-package imports.
type UserID string

// sessionAttachment is the room-internal record stored per attached session.
// Only the room loop reads and writes this type.
type sessionAttachment struct {
	playerID PlayerID
	userID   UserID
}

// RoomStatus represents the current lifecycle state of a room instance.
type RoomStatus int

const (
	RoomStatusCreated  RoomStatus = iota // Initial state after newRoom().
	RoomStatusRunning                    // Tick loop is active.
	RoomStatusDraining                   // Shutting down; no new joins accepted.
	RoomStatusClosed                     // Tick loop has exited.
)

func (s RoomStatus) String() string {
	switch s {
	case RoomStatusCreated:
		return "created"
	case RoomStatusRunning:
		return "running"
	case RoomStatusDraining:
		return "draining"
	case RoomStatusClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// RoomConfig holds per-room configuration.
type RoomConfig struct {
	MaxPlayers            int     // Maximum concurrent players allowed.
	TickRateHz            int     // Room simulation frequency (default 20).
	BroadcastRateHz       int     // Delta broadcast frequency (default 10).
	CommandQueueSize      int     // Buffered command channel depth (default 256).
	SpatialCellSizeM      float32 // Spatial hash cell size in meters (default 10).
	InterestVisualRadiusM float32 // Visual interest radius in meters (default 30).
}

// DefaultRoomConfig returns a RoomConfig with production-default values.
func DefaultRoomConfig() RoomConfig {
	return RoomConfig{
		MaxPlayers:            200,
		TickRateHz:            20,
		BroadcastRateHz:       10,
		CommandQueueSize:      256,
		SpatialCellSizeM:      10.0,
		InterestVisualRadiusM: 30.0,
	}
}

// RoomCommandKind identifies the type of command sent to the room loop.
type RoomCommandKind uint8

const (
	CmdJoin                  RoomCommandKind = iota + 1 // Player session joining the room.
	CmdLeave                                            // Player session leaving gracefully.
	CmdDisconnect                                       // Transport session disconnected unexpectedly.
	CmdPlayerInput                                      // Player movement/transform input from client.
	CmdUpdatePlayerTransform                            // Internal: update player transform (validated).
	// Future: CmdObjectCommand, CmdObjectLock, etc.
)

// RoomCommand is an envelope for commands enqueued by transport goroutines.
// The room loop is the sole consumer; it is the only code permitted to act on
// these commands and mutate room state.
type RoomCommand struct {
	Kind      RoomCommandKind
	SessionID SessionID
	PlayerID  PlayerID
	// UserID is the authenticated user identity. Set on CmdJoin for duplicate detection.
	UserID UserID
	// Payload holds kind-specific data; typed per command kind in future milestones.
	Payload   any
	Timestamp time.Time
}

// RoomSpec is the input used to create a new room instance.
type RoomSpec struct {
	LogicalRoomID LogicalRoomID
	InstanceID    RoomInstanceID
	Config        RoomConfig
}

// RoomInstance is the registry-level record of a room.
// It tracks identity and status metadata only — live room state lives in Room.
type RoomInstance struct {
	InstanceID    RoomInstanceID
	LogicalRoomID LogicalRoomID
	Status        RoomStatus
	CreatedAt     time.Time
	ClosedAt      *time.Time // nil until the room is closed.
}
