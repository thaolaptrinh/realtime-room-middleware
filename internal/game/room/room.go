package room

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/thaonguyen/realtime-room-middleware/internal/game/cluster"
	"github.com/thaonguyen/realtime-room-middleware/internal/game/delta"
	"github.com/thaonguyen/realtime-room-middleware/internal/game/interest"
	"github.com/thaonguyen/realtime-room-middleware/internal/game/object"
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

	// objectMgr manages room object state (create, get, list, update, remove).
	// Accessed only from the room loop goroutine.
	objectMgr *object.ObjectManager

	// lockMgr enforces server-authoritative lease-based object locking.
	// Accessed only from the room loop goroutine.
	lockMgr *object.LockManager

	// --- Cluster state ----------------------------------------------------------

	// clusterAlloc is the position cluster allocator. Called only from the room loop.
	clusterAlloc cluster.ClusterAllocator

	// clusterOutput is the latest cluster assignment result. Protected by sessionMu
	// for external reads (GetClusterOutput, PlayerCluster, VisiblePlayersFor).
	// Written by the room loop under sessionMu.Lock().
	clusterOutput cluster.ClusterOutput

	// The following cluster scheduling fields are room-loop-only.
	// No mutex is needed; they are read and written exclusively by runTick.

	// lastClusterTick is the tick at which the last cluster recompute occurred.
	lastClusterTick uint32

	// maxMovementSinceLastCluster tracks the maximum single-player movement (meters)
	// since the last recompute. Triggers an early recompute when it exceeds
	// ClusterConfig.MovementThreshold.
	maxMovementSinceLastCluster float32

	// clusterMembershipDirty is set true on any join, leave, or disconnect that
	// changes the actual player count. Triggers an early recompute.
	clusterMembershipDirty bool

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

	lease := cfg.ObjectLockLease
	if lease.TTL <= 0 {
		lease = object.DefaultLockLease()
	}
	objMgr := object.NewObjectManager()

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
		objectMgr:        objMgr,
		lockMgr:          object.NewLockManager(objMgr, lease),
		clusterAlloc:     cluster.NewKMeansClusterAllocator(),
		clusterOutput:    cluster.ClusterOutput{Assignments: make(map[player.PlayerID]cluster.ClusterID), Clusters: make(map[cluster.ClusterID][]player.PlayerID), Centroids: make(map[cluster.ClusterID]spatial.EntityPosition)},
		done:             make(chan struct{}),
	}
}

// InstanceID returns the physical room instance identifier.
func (r *Room) InstanceID() RoomInstanceID { return r.instanceID }

// LogicalRoomID returns the product-facing room identifier.
func (r *Room) LogicalRoomID() LogicalRoomID { return r.logicalRoomID }

// ClusterConfig returns the cluster configuration for this room.
func (r *Room) ClusterConfig() cluster.ClusterConfig { return r.config.ClusterConfig }

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

// CreateObject registers a room object for server-side initialization.
// This is for setup by room management code, not for client-driven object creation.
// Safe to call from any goroutine while the room is running.
func (r *Room) CreateObject(id object.ObjectID, kind object.ObjectKind, transform object.ObjectTransform) error {
	r.sessionMu.Lock()
	defer r.sessionMu.Unlock()
	_, err := r.objectMgr.Create(id, kind, transform)
	return err
}

// ObjectCount returns the total number of tracked room objects (active and inactive).
// Safe to call from any goroutine; reads under sessionMu.
func (r *Room) ObjectCount() int {
	r.sessionMu.RLock()
	defer r.sessionMu.RUnlock()
	return r.objectMgr.Count()
}

// UserLockCount returns the number of active object locks held by the given user.
// Safe to call from any goroutine; reads under sessionMu.
func (r *Room) UserLockCount(userID string) int {
	r.sessionMu.RLock()
	defer r.sessionMu.RUnlock()
	return r.lockMgr.UserLockCount(userID)
}

// buildDeltaBatches computes a per-session DeltaBatch for all active sessions.
//
// Called by broadcast(). Must be called with sessionMu held (write lock) since it
// reads activeSessions, players, spatial, clusterOutput, and mutates snapshotCache.
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

		var visiblePlayers []player.PlayerID

		if r.config.ClusterConfig.Enabled {
			// Cluster-based interest: use cluster membership when enabled.
			viewerCluster, ok := r.clusterOutput.Assignments[pid]
			if !ok {
				// Player not in cluster output (e.g., empty room after join).
				visiblePlayers = []player.PlayerID{}
			} else {
				members := r.clusterOutput.Clusters[viewerCluster]
				// Exclude self.
				visiblePlayers = make([]player.PlayerID, 0, len(members))
				for _, memberID := range members {
					if memberID != pid {
						visiblePlayers = append(visiblePlayers, memberID)
					}
				}
			}
		} else {
			// Fallback: radius-based interest.
			transform, _ := ps.Snapshot()
			viewerPos := spatial.Pos(transform.Position.X, transform.Position.Z)
			interestSet := r.interestMgr.QueryVisiblePlayers(r.spatial, viewerPos, spatial.EntityID(pid))
			visiblePlayers = make([]player.PlayerID, len(interestSet.VisiblePlayers))
			for i, id := range interestSet.VisiblePlayers {
				visiblePlayers[i] = player.PlayerID(id)
			}
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

// --- Cluster query helpers ----------------------------------------------------

// GetClusterOutput returns a copy of the current cluster output.
// Safe to call from any goroutine; returns a consistent snapshot.
func (r *Room) GetClusterOutput() cluster.ClusterOutput {
	r.sessionMu.RLock()
	defer r.sessionMu.RUnlock()

	// Return a shallow copy of the output; the internal maps are not copied further
	// because they are only ever reassigned, not mutated in place, by the room loop.
	return r.clusterOutput
}

// PlayerCluster returns the cluster ID assigned to the given player.
// Returns false if the player is not found in the current cluster output.
// Safe to call from any goroutine.
func (r *Room) PlayerCluster(pid player.PlayerID) (cluster.ClusterID, bool) {
	r.sessionMu.RLock()
	defer r.sessionMu.RUnlock()
	cid, ok := r.clusterOutput.Assignments[pid]
	return cid, ok
}

// VisiblePlayersFor returns the visible players for the given player using
// cluster-based interest when enabled, or falls back to radius-based interest
// when disabled. Excludes the player themselves.
// Safe to call from any goroutine.
func (r *Room) VisiblePlayersFor(pid player.PlayerID) []player.PlayerID {
	r.sessionMu.RLock()
	defer r.sessionMu.RUnlock()

	if r.config.ClusterConfig.Enabled {
		viewerCluster, ok := r.clusterOutput.Assignments[pid]
		if !ok {
			return []player.PlayerID{}
		}
		members := r.clusterOutput.Clusters[viewerCluster]
		result := make([]player.PlayerID, 0, len(members))
		for _, memberID := range members {
			if memberID != pid {
				result = append(result, memberID)
			}
		}
		return result
	}

	// Fallback to radius-based interest.
	ps, ok := r.players[pid]
	if !ok {
		return []player.PlayerID{}
	}
	transform, _ := ps.Snapshot()
	viewerPos := spatial.Pos(transform.Position.X, transform.Position.Z)
	interestSet := r.interestMgr.QueryVisiblePlayers(r.spatial, viewerPos, spatial.EntityID(pid))
	result := make([]player.PlayerID, len(interestSet.VisiblePlayers))
	for i, id := range interestSet.VisiblePlayers {
		result[i] = player.PlayerID(id)
	}
	return result
}
