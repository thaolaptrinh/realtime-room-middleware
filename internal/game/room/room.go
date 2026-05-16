package room

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/thaonguyen/realtime-room-middleware/internal/game/delta"
	"github.com/thaonguyen/realtime-room-middleware/internal/game/interest"
	"github.com/thaonguyen/realtime-room-middleware/internal/game/player"
	"github.com/thaonguyen/realtime-room-middleware/internal/game/spatial"
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

	// currentTick is the simulation tick counter, incremented each tick loop.
	// Readable from any goroutine via atomic load; written only by runTick.
	currentTick atomic.Uint32

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

	// players maps PlayerID → PlayerState for players in this room.
	// Written only by the room loop goroutine (runTick).
	// External callers may read snapshots via GetPlayerState.
	players map[player.PlayerID]*player.PlayerState

	// spatial is the spatial hash index for proximity queries.
	// Only mutated by the room loop (inside handleCommand under sessionMu).
	spatial *spatial.GridSpatialHash

	// interestMgr computes per-client visible player sets from the spatial index.
	// Read-only after construction; safe to use from the room loop.
	interestMgr *interest.InterestManager

	// snapshotCache holds per-session ClientSnapshot state for delta computation.
	// Accessed only under sessionMu (read or write).
	snapshotCache *delta.SnapshotCache

	// deltaBuilder computes per-client PlayerDelta values.
	// Stateless; shared across all broadcast calls.
	deltaBuilder *delta.DeltaBuilder

	// dirtyPlayers tracks players whose transforms were updated since the last
	// broadcast. Accessed only under sessionMu (read or write).
	dirtyPlayers map[player.PlayerID]struct{}

	// cancel signals the tick loop to stop.
	cancel context.CancelFunc
	// done is closed by the tick loop goroutine when it exits.
	done chan struct{}
}

// newRoom constructs a Room from a RoomSpec. Call Start to activate the tick loop.
func newRoom(spec RoomSpec, logger *slog.Logger) *Room {
	defaults := DefaultRoomConfig()
	cfg := spec.Config
	if cfg.TickRateHz <= 0 {
		cfg.TickRateHz = defaults.TickRateHz
	}
	if cfg.BroadcastRateHz <= 0 {
		cfg.BroadcastRateHz = defaults.BroadcastRateHz
	}
	if cfg.CommandQueueSize <= 0 {
		cfg.CommandQueueSize = defaults.CommandQueueSize
	}
	if cfg.SpatialCellSizeM <= 0 {
		cfg.SpatialCellSizeM = defaults.SpatialCellSizeM
	}
	if cfg.InterestVisualRadiusM <= 0 {
		cfg.InterestVisualRadiusM = defaults.InterestVisualRadiusM
	}

	interestCfg := interest.DefaultInterestConfig()
	interestCfg.VisualRadiusM = cfg.InterestVisualRadiusM

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
		players:          make(map[player.PlayerID]*player.PlayerState),
		spatial:          spatial.NewGridSpatialHash(spatial.SpatialConfig{CellSizeM: cfg.SpatialCellSizeM}),
		interestMgr:      interest.NewInterestManager(interestCfg),
		snapshotCache:    delta.NewSnapshotCache(),
		deltaBuilder:     delta.NewDeltaBuilder(),
		dirtyPlayers:     make(map[player.PlayerID]struct{}),
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

// CurrentTick returns the current simulation tick counter.
// Safe to call from any goroutine.
func (r *Room) CurrentTick() uint32 {
	return r.currentTick.Load()
}

// GetPlayerState returns a snapshot of the player's current transform and version.
// Returns false if the player is not in the room.
// Safe to call from any goroutine.
func (r *Room) GetPlayerState(id player.PlayerID) (player.PlayerTransform, uint32, bool) {
	// Note: players map is only written by the room loop, but we need a lock
	// to safely read from it while the room loop may be adding/removing players.
	// For simplicity, we use the sessionMu which already covers mutations.
	r.sessionMu.RLock()
	defer r.sessionMu.RUnlock()
	p, ok := r.players[id]
	if !ok {
		return player.PlayerTransform{}, 0, false
	}
	transform, version := p.Snapshot()
	return transform, version, true
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

// NearbyPlayers returns player IDs within the given radius of the specified player,
// excluding the player themselves. Returns nil if the player is not found.
// Safe to call from any goroutine.
func (r *Room) NearbyPlayers(pid player.PlayerID, radius float32) []player.PlayerID {
	r.sessionMu.RLock()
	defer r.sessionMu.RUnlock()

	pos, ok := r.spatial.Get(spatial.EntityID(pid))
	if !ok {
		return nil
	}

	ids := r.spatial.QueryRadius(pos, radius)
	result := make([]player.PlayerID, 0, len(ids))
	for _, id := range ids {
		if id != spatial.EntityID(pid) {
			result = append(result, player.PlayerID(id))
		}
	}
	return result
}

// NearbyPlayersAt returns player IDs within the given radius of a world position.
// Safe to call from any goroutine.
func (r *Room) NearbyPlayersAt(pos player.Vector3, radius float32) []player.PlayerID {
	r.sessionMu.RLock()
	defer r.sessionMu.RUnlock()

	ids := r.spatial.QueryRadius(spatial.Pos(pos.X, pos.Z), radius)
	result := make([]player.PlayerID, 0, len(ids))
	for _, id := range ids {
		result = append(result, player.PlayerID(id))
	}
	return result
}

// SnapshotCacheLen returns the number of per-session delta snapshots currently tracked.
// Safe to call from any goroutine.
func (r *Room) SnapshotCacheLen() int {
	r.sessionMu.RLock()
	defer r.sessionMu.RUnlock()
	return r.snapshotCache.Len()
}

// DirtyPlayerCount returns the number of players whose transforms are marked dirty
// (updated since the last broadcast tick).
// Safe to call from any goroutine.
func (r *Room) DirtyPlayerCount() int {
	r.sessionMu.RLock()
	defer r.sessionMu.RUnlock()
	return len(r.dirtyPlayers)
}

// buildDeltaBatches computes a per-session DeltaBatch for all active sessions.
//
// Called by broadcast(). Must be called with sessionMu held (write lock) since it
// reads activeSessions, players, spatial, and mutates snapshotCache.
func (r *Room) buildDeltaBatches(tick uint32) map[SessionID]*delta.DeltaBatch {
	if len(r.activeSessions) == 0 {
		return nil
	}

	batches := make(map[SessionID]*delta.DeltaBatch, len(r.activeSessions))

	for sessionID, att := range r.activeSessions {
		pid := player.PlayerID(att.playerID)

		ps, ok := r.players[pid]
		if !ok {
			continue
		}

		transform, _ := ps.Snapshot()
		viewerPos := spatial.Pos(transform.Position.X, transform.Position.Z)

		interestSet := r.interestMgr.QueryVisiblePlayers(r.spatial, viewerPos, spatial.EntityID(pid))

		visiblePlayers := make([]player.PlayerID, len(interestSet.VisiblePlayers))
		for i, id := range interestSet.VisiblePlayers {
			visiblePlayers[i] = player.PlayerID(id)
		}

		snapshot := r.snapshotCache.GetOrCreate(string(sessionID))
		playerDelta := r.deltaBuilder.BuildPlayerDelta(tick, visiblePlayers, snapshot, r.players)

		batches[sessionID] = &delta.DeltaBatch{
			Tick:        tick,
			PlayerDelta: playerDelta,
		}
	}

	return batches
}
