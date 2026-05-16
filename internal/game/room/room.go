package room

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
)

// Room is the runtime instance of a room.
//
// It holds the inbound command queue, owns the tick loop goroutine, and
// exposes lifecycle control (Start, Stop).
//
// Only the room loop (runTick goroutine) may mutate room state.
// Transport goroutines must call Enqueue to submit commands.
type Room struct {
	instanceID    RoomInstanceID
	logicalRoomID LogicalRoomID
	config        RoomConfig
	logger        *slog.Logger

	// commands is the inbound queue from transport goroutines.
	// Only the tick loop reads from it.
	commands chan RoomCommand

	statusMu sync.RWMutex
	status   RoomStatus

	// playerCount is updated by the room loop on join/leave/disconnect commands.
	// Modified only inside runTick; readable from any goroutine via atomic load.
	playerCount atomic.Int32

	// sessionMu protects activeSessions and userSessionIndex.
	// The room loop holds the write lock when mutating (inside handleCommand).
	// External callers hold the read lock via HasSession, HasUser, ActiveSessions.
	sessionMu sync.RWMutex

	// activeSessions maps SessionID → attachment info.
	// Written only by the room loop goroutine (runTick).
	activeSessions map[SessionID]sessionAttachment

	// userSessionIndex maps UserID → SessionID for duplicate-join detection.
	// Written only by the room loop goroutine (runTick).
	userSessionIndex map[UserID]SessionID

	// cancel signals the tick loop to stop.
	cancel context.CancelFunc
	// done is closed by the tick loop goroutine when it exits.
	done chan struct{}
}

// newRoom constructs a Room from a RoomSpec. Call Start to activate the tick loop.
func newRoom(spec RoomSpec, logger *slog.Logger) *Room {
	cfg := spec.Config
	if cfg.TickRateHz <= 0 {
		cfg.TickRateHz = DefaultRoomConfig().TickRateHz
	}
	if cfg.CommandQueueSize <= 0 {
		cfg.CommandQueueSize = DefaultRoomConfig().CommandQueueSize
	}

	return &Room{
		instanceID:    spec.InstanceID,
		logicalRoomID: spec.LogicalRoomID,
		config:        cfg,
		logger: logger.With(
			slog.String("room_instance_id", string(spec.InstanceID)),
			slog.String("logical_room_id", string(spec.LogicalRoomID)),
		),
		commands:         make(chan RoomCommand, cfg.CommandQueueSize),
		status:           RoomStatusCreated,
		activeSessions:   make(map[SessionID]sessionAttachment),
		userSessionIndex: make(map[UserID]SessionID),
		done:             make(chan struct{}),
	}
}

// InstanceID returns the physical room instance identifier.
func (r *Room) InstanceID() RoomInstanceID { return r.instanceID }

// LogicalRoomID returns the product-facing room identifier.
func (r *Room) LogicalRoomID() LogicalRoomID { return r.logicalRoomID }

// Status returns the current lifecycle status.
func (r *Room) Status() RoomStatus {
	r.statusMu.RLock()
	defer r.statusMu.RUnlock()
	return r.status
}

// PlayerCount returns the number of tracked players.
// This is a placeholder counter updated by join/leave/disconnect commands.
func (r *Room) PlayerCount() int {
	return int(r.playerCount.Load())
}

// HasSession reports whether the given session is currently attached to this room.
// Safe to call from any goroutine.
func (r *Room) HasSession(id SessionID) bool {
	r.sessionMu.RLock()
	defer r.sessionMu.RUnlock()
	_, ok := r.activeSessions[id]
	return ok
}

// HasUser reports whether a player with the given user ID is currently in this room.
// Safe to call from any goroutine.
func (r *Room) HasUser(id UserID) bool {
	r.sessionMu.RLock()
	defer r.sessionMu.RUnlock()
	_, ok := r.userSessionIndex[id]
	return ok
}

// ActiveSessions returns a snapshot of the currently attached session IDs.
// Safe to call from any goroutine. The returned slice is a copy.
func (r *Room) ActiveSessions() []SessionID {
	r.sessionMu.RLock()
	defer r.sessionMu.RUnlock()
	ids := make([]SessionID, 0, len(r.activeSessions))
	for id := range r.activeSessions {
		ids = append(ids, id)
	}
	return ids
}

// Enqueue submits a command to the room loop queue.
// Returns an error if the room is not running or if the command queue is full.
// Safe to call from any goroutine (transport adapters, session manager, etc.).
func (r *Room) Enqueue(cmd RoomCommand) error {
	r.statusMu.RLock()
	status := r.status
	r.statusMu.RUnlock()

	if status != RoomStatusRunning {
		return fmt.Errorf("room %q is not running (status: %s)", r.instanceID, status)
	}

	select {
	case r.commands <- cmd:
		return nil
	default:
		return fmt.Errorf("room %q command queue full", r.instanceID)
	}
}
