package room

import (
	"context"
	"log/slog"
	"math"
	"time"

	"github.com/thaonguyen/realtime-room-middleware/internal/game/cluster"
	"github.com/thaonguyen/realtime-room-middleware/internal/game/player"
	"github.com/thaonguyen/realtime-room-middleware/internal/game/spatial"
)

// runTick is the room loop goroutine.
// It is the single authority for room state mutation.
// No transport goroutine may mutate room state directly.
//
// The loop exits when ctx is cancelled (via Stop or parent context cancellation).
func (r *Room) runTick(ctx context.Context) {
	defer close(r.done)

	tickInterval := time.Second / time.Duration(r.config.TickRateHz)
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	r.logger.Info("room tick loop running",
		slog.Int("tick_rate_hz", r.config.TickRateHz),
	)

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("room tick loop stopped")
			return
		case <-ticker.C:
			r.tick()
		}
	}
}

// tick executes one simulation step.
//
// Order per tick:
//  1. Expire stale object locks (before command drain so AcquireLock sees clean state).
//  2. Drain the command queue.
//  3. Recompute clusters (if triggered by interval, movement, or membership change).
//  4. Broadcast deltas at the configured broadcast rate (default 10 Hz).
func (r *Room) tick() {
	tick := r.currentTick.Add(1)
	r.releaseExpiredLocks()
	r.drainCommands(tick)

	// Cluster recompute scheduling (Phase 1 position cluster sync).
	if r.shouldRecomputeCluster(tick) {
		r.recomputeCluster(tick)
	}

	if r.isBroadcastTick(tick) {
		r.broadcast(tick)
	}
}

// shouldRecomputeCluster evaluates whether a cluster recompute is due.
// Triggers: interval cadence, movement threshold exceeded, or membership change.
// Called from the room loop; reads room-loop-only fields.
func (r *Room) shouldRecomputeCluster(tick uint32) bool {
	cfg := r.config.ClusterConfig
	if !cfg.Enabled {
		return false
	}

	// Interval trigger: recompute every ReclusterIntervalTicks.
	ticksSinceLast := tick - r.lastClusterTick
	if ticksSinceLast >= uint32(cfg.ReclusterIntervalTicks) {
		return true
	}

	// Movement trigger: if any player moved more than MovementThreshold since last recompute.
	if r.maxMovementSinceLastCluster > cfg.MovementThreshold {
		return true
	}

	// Membership change trigger: join/leave/disconnect changed player count.
	if r.clusterMembershipDirty {
		return true
	}

	return false
}

// recomputeCluster runs the K-Means cluster allocator and updates the room's cluster output.
// Called from the room loop; writes to clusterOutput and resets scheduling flags.
func (r *Room) recomputeCluster(tick uint32) {
	startTime := time.Now()

	r.sessionMu.Lock()
	defer r.sessionMu.Unlock()

	// Build ClusterInput from current player states.
	players := make([]cluster.ClusterPlayer, 0, len(r.players))
	for pid, ps := range r.players {
		transform, _ := ps.Snapshot()
		players = append(players, cluster.ClusterPlayer{
			PlayerID: pid,
			Position: spatial.Pos(transform.Position.X, transform.Position.Z),
		})
	}

	input := cluster.ClusterInput{Players: players}

	// Call the cluster allocator (stateful; retains previous output for hysteresis).
	output, err := r.clusterAlloc.Compute(input, r.config.ClusterConfig)
	if err != nil {
		r.logger.Warn("cluster recompute failed",
			slog.String("error", err.Error()),
		)
		return
	}

	// Commit the new cluster output.
	r.clusterOutput = output
	r.lastClusterTick = tick
	r.maxMovementSinceLastCluster = 0
	r.clusterMembershipDirty = false

	durationMs := float32(time.Since(startTime).Microseconds()) / 1000.0
	r.logger.Debug("cluster recompute completed",
		slog.Uint64("tick", uint64(tick)),
		slog.Int("clusters", output.K),
		slog.Int("players", len(players)),
		slog.Float64("duration_ms", float64(durationMs)),
	)
}

// releaseExpiredLocks clears any object locks whose TTL has elapsed.
// Called at the start of each tick, before command processing.
// Holds sessionMu so external readers (ObjectCount, UserLockCount) see consistent state.
func (r *Room) releaseExpiredLocks() {
	now := time.Now()
	r.sessionMu.Lock()
	released := r.lockMgr.ReleaseExpired(now)
	r.sessionMu.Unlock()
	for _, id := range released {
		r.logger.Debug("room: object lock expired", slog.String("object_id", string(id)))
	}
}

// isBroadcastTick returns true if a broadcast should occur on the given tick number.
// Broadcast fires every TickRateHz/BroadcastRateHz ticks (e.g., every 2nd tick when
// TickRateHz=20 and BroadcastRateHz=10).
func (r *Room) isBroadcastTick(tick uint32) bool {
	if r.config.BroadcastRateHz <= 0 || r.config.TickRateHz <= 0 {
		return false
	}
	if r.config.BroadcastRateHz >= r.config.TickRateHz {
		return true
	}
	ratio := uint32(r.config.TickRateHz / r.config.BroadcastRateHz)
	if ratio == 0 {
		return true
	}
	return tick%ratio == 0
}

// broadcast builds per-client delta batches and clears dirty state.
// Transport send is a future milestone — batches are computed but not yet sent.
func (r *Room) broadcast(tick uint32) {
	r.sessionMu.Lock()
	batches := r.buildDeltaBatches(tick)
	r.clearDirtyPlayers()
	r.sessionMu.Unlock()
	// batches are discarded until transport wiring is implemented.
	_ = batches
}

// clearDirtyPlayers resets the dirty player set after a broadcast.
// Must be called with sessionMu held.
func (r *Room) clearDirtyPlayers() {
	for pid := range r.dirtyPlayers {
		delete(r.dirtyPlayers, pid)
	}
}

// drainCommands processes all queued commands without blocking.
func (r *Room) drainCommands(tick uint32) {
	for {
		select {
		case cmd := <-r.commands:
			r.handleCommand(cmd, tick)
		default:
			return
		}
	}
}

// handleCommand dispatches a single RoomCommand.
// Only the room loop calls this function — it is the mutation boundary.
func (r *Room) handleCommand(cmd RoomCommand, tick uint32) {
	switch cmd.Kind {
	case CmdJoin:
		r.sessionMu.Lock()
		// Reject duplicate: same UserID cannot be in the room twice.
		if cmd.UserID != "" {
			if _, exists := r.userSessionIndex[cmd.UserID]; exists {
				r.sessionMu.Unlock()
				r.logger.Warn("room command: duplicate user join rejected",
					slog.String("session_id", string(cmd.SessionID)),
					slog.String("user_id", string(cmd.UserID)),
				)
				return
			}
		}
		r.activeSessions[cmd.SessionID] = sessionAttachment{
			playerID: cmd.PlayerID,
			userID:   cmd.UserID,
		}
		if cmd.UserID != "" {
			r.userSessionIndex[cmd.UserID] = cmd.SessionID
		}
		r.playerCount.Add(1)
		r.clusterMembershipDirty = true
		r.sessionMu.Unlock()

		// Create player state for this player.
		pid := player.PlayerID(cmd.PlayerID)
		uid := player.UserID(cmd.UserID)
		p := player.NewPlayerState(pid, uid, cmd.Timestamp)
		p.SetStatus(player.PlayerStatusActive)

		r.sessionMu.Lock()
		r.players[pid] = p
		if err := r.spatial.Update(spatial.EntityID(pid), spatial.Pos(0, 0)); err != nil {
			r.logger.Warn("room command: spatial update rejected on join",
				slog.String("player_id", string(pid)),
				slog.String("error", err.Error()),
			)
		}
		r.snapshotCache.GetOrCreate(string(cmd.SessionID))
		r.sessionMu.Unlock()

		r.logger.Debug("room command: join",
			slog.String("session_id", string(cmd.SessionID)),
			slog.String("player_id", string(cmd.PlayerID)),
			slog.String("user_id", string(cmd.UserID)),
		)

	case CmdLeave:
		r.sessionMu.Lock()
		att, ok := r.activeSessions[cmd.SessionID]
		if ok {
			delete(r.activeSessions, cmd.SessionID)
			if att.userID != "" {
				delete(r.userSessionIndex, att.userID)
			}
		}
		if ok {
			r.playerCount.Add(-1)
			r.clusterMembershipDirty = true
			if p, exists := r.players[player.PlayerID(cmd.PlayerID)]; exists {
				p.MarkStatus(player.PlayerStatusLeaving)
				delete(r.players, player.PlayerID(cmd.PlayerID))
			}
			r.spatial.Remove(spatial.EntityID(cmd.PlayerID))
			r.snapshotCache.Remove(string(cmd.SessionID))
			delete(r.dirtyPlayers, player.PlayerID(cmd.PlayerID))
		}
		r.sessionMu.Unlock()

		// Release any object locks held by this session.
		r.sessionMu.Lock()
		releasedLocks := r.lockMgr.ReleaseBySession(string(cmd.SessionID), time.Now())
		r.sessionMu.Unlock()
		for _, id := range releasedLocks {
			r.logger.Debug("room command: lock released on leave",
				slog.String("session_id", string(cmd.SessionID)),
				slog.String("object_id", string(id)),
			)
		}

		r.logger.Debug("room command: leave",
			slog.String("session_id", string(cmd.SessionID)),
			slog.String("player_id", string(cmd.PlayerID)),
		)

	case CmdDisconnect:
		r.sessionMu.Lock()
		att, ok := r.activeSessions[cmd.SessionID]
		if ok {
			delete(r.activeSessions, cmd.SessionID)
			if att.userID != "" {
				delete(r.userSessionIndex, att.userID)
			}
		}
		if ok {
			r.playerCount.Add(-1)
			r.clusterMembershipDirty = true
			if p, exists := r.players[player.PlayerID(att.playerID)]; exists {
				p.MarkStatus(player.PlayerStatusGone)
				delete(r.players, player.PlayerID(att.playerID))
			}
			r.spatial.Remove(spatial.EntityID(att.playerID))
			r.snapshotCache.Remove(string(cmd.SessionID))
			delete(r.dirtyPlayers, player.PlayerID(att.playerID))
		}
		r.sessionMu.Unlock()

		// Release any object locks held by this session.
		r.sessionMu.Lock()
		releasedLocks := r.lockMgr.ReleaseBySession(string(cmd.SessionID), time.Now())
		r.sessionMu.Unlock()
		for _, id := range releasedLocks {
			r.logger.Debug("room command: lock released on disconnect",
				slog.String("session_id", string(cmd.SessionID)),
				slog.String("object_id", string(id)),
			)
		}

		r.logger.Debug("room command: disconnect",
			slog.String("session_id", string(cmd.SessionID)),
		)

	case CmdPlayerInput:
		input, ok := cmd.Payload.(player.PlayerInput)
		if !ok {
			r.logger.Warn("room command: CmdPlayerInput payload is not PlayerInput",
				slog.String("player_id", string(cmd.PlayerID)),
			)
			return
		}
		if err := player.ValidatePlayerInput(input); err != nil {
			r.logger.Warn("room command: invalid player input",
				slog.String("player_id", string(cmd.PlayerID)),
				slog.String("error", err.Error()),
			)
			return
		}
		r.sessionMu.Lock()
		if p, exists := r.players[player.PlayerID(cmd.PlayerID)]; exists {
			// Track movement for cluster recompute trigger.
			prevTransform, _ := p.Snapshot()
			newPos := input.Transform.Position
			dx := newPos.X - prevTransform.Position.X
			dz := newPos.Z - prevTransform.Position.Z
			dist := float32(math.Sqrt(float64(dx*dx + dz*dz)))
			if dist > r.maxMovementSinceLastCluster {
				r.maxMovementSinceLastCluster = dist
			}

			p.UpdateTransform(input.Transform, tick)
			if err := r.spatial.Update(
				spatial.EntityID(cmd.PlayerID),
				spatial.Pos(input.Transform.Position.X, input.Transform.Position.Z),
			); err != nil {
				r.logger.Warn("room command: spatial update rejected on player input",
					slog.String("player_id", string(cmd.PlayerID)),
					slog.String("error", err.Error()),
				)
			}
			r.dirtyPlayers[player.PlayerID(cmd.PlayerID)] = struct{}{}
		}
		r.sessionMu.Unlock()

		r.logger.Debug("room command: player input",
			slog.String("player_id", string(cmd.PlayerID)),
			slog.Uint64("seq", uint64(input.Seq)),
		)

	case CmdUpdatePlayerTransform:
		transform, ok := cmd.Payload.(player.PlayerTransform)
		if !ok {
			r.logger.Warn("room command: CmdUpdatePlayerTransform payload is not PlayerTransform",
				slog.String("player_id", string(cmd.PlayerID)),
			)
			return
		}
		r.sessionMu.Lock()
		if p, exists := r.players[player.PlayerID(cmd.PlayerID)]; exists {
			// Track movement for cluster recompute trigger.
			prevTransform, _ := p.Snapshot()
			newPos := transform.Position
			dx := newPos.X - prevTransform.Position.X
			dz := newPos.Z - prevTransform.Position.Z
			dist := float32(math.Sqrt(float64(dx*dx + dz*dz)))
			if dist > r.maxMovementSinceLastCluster {
				r.maxMovementSinceLastCluster = dist
			}

			p.UpdateTransform(transform, tick)
			if err := r.spatial.Update(
				spatial.EntityID(cmd.PlayerID),
				spatial.Pos(transform.Position.X, transform.Position.Z),
			); err != nil {
				r.logger.Warn("room command: spatial update rejected on transform",
					slog.String("player_id", string(cmd.PlayerID)),
					slog.String("error", err.Error()),
				)
			}
			r.dirtyPlayers[player.PlayerID(cmd.PlayerID)] = struct{}{}
		}
		r.sessionMu.Unlock()

		r.logger.Debug("room command: update player transform",
			slog.String("player_id", string(cmd.PlayerID)),
		)

	case CmdObjectLockAcquire:
		payload, ok := cmd.Payload.(ObjectCommandPayload)
		if !ok {
			r.logger.Warn("room command: CmdObjectLockAcquire payload is not ObjectCommandPayload",
				slog.String("session_id", string(cmd.SessionID)),
			)
			return
		}
		r.sessionMu.Lock()
		result := r.lockMgr.AcquireLock(
			payload.ObjectID,
			string(cmd.UserID),
			string(cmd.SessionID),
			cmd.Timestamp,
		)
		r.sessionMu.Unlock()
		if result.Granted {
			r.logger.Debug("room command: object lock acquired",
				slog.String("object_id", string(payload.ObjectID)),
				slog.String("user_id", string(cmd.UserID)),
			)
		} else {
			r.logger.Debug("room command: object lock acquire rejected",
				slog.String("object_id", string(payload.ObjectID)),
				slog.String("user_id", string(cmd.UserID)),
				slog.String("reason", result.Reason),
			)
		}

	case CmdObjectLockRefresh:
		payload, ok := cmd.Payload.(ObjectCommandPayload)
		if !ok {
			r.logger.Warn("room command: CmdObjectLockRefresh payload is not ObjectCommandPayload",
				slog.String("session_id", string(cmd.SessionID)),
			)
			return
		}
		r.sessionMu.Lock()
		result := r.lockMgr.RefreshLock(
			payload.ObjectID,
			string(cmd.UserID),
			cmd.Timestamp,
		)
		r.sessionMu.Unlock()
		if result.Granted {
			r.logger.Debug("room command: object lock refreshed",
				slog.String("object_id", string(payload.ObjectID)),
				slog.String("user_id", string(cmd.UserID)),
			)
		} else {
			r.logger.Debug("room command: object lock refresh rejected",
				slog.String("object_id", string(payload.ObjectID)),
				slog.String("user_id", string(cmd.UserID)),
				slog.String("reason", result.Reason),
			)
		}

	case CmdObjectLockRelease:
		payload, ok := cmd.Payload.(ObjectCommandPayload)
		if !ok {
			r.logger.Warn("room command: CmdObjectLockRelease payload is not ObjectCommandPayload",
				slog.String("session_id", string(cmd.SessionID)),
			)
			return
		}
		r.sessionMu.Lock()
		result := r.lockMgr.ReleaseLock(
			payload.ObjectID,
			string(cmd.UserID),
			cmd.Timestamp,
		)
		r.sessionMu.Unlock()
		if result.Granted {
			r.logger.Debug("room command: object lock released",
				slog.String("object_id", string(payload.ObjectID)),
				slog.String("user_id", string(cmd.UserID)),
			)
		} else {
			r.logger.Debug("room command: object lock release rejected",
				slog.String("object_id", string(payload.ObjectID)),
				slog.String("user_id", string(cmd.UserID)),
				slog.String("reason", result.Reason),
			)
		}

	case CmdObjectUpdate:
		payload, ok := cmd.Payload.(ObjectCommandPayload)
		if !ok {
			r.logger.Warn("room command: CmdObjectUpdate payload is not ObjectCommandPayload",
				slog.String("session_id", string(cmd.SessionID)),
			)
			return
		}
		r.sessionMu.Lock()
		if payload.Transform != nil {
			if err := r.objectMgr.UpdateTransform(payload.ObjectID, *payload.Transform); err != nil {
				r.logger.Warn("room command: CmdObjectUpdate transform failed",
					slog.String("object_id", string(payload.ObjectID)),
					slog.String("error", err.Error()),
				)
			}
		}
		if payload.CustomState != nil {
			if err := r.objectMgr.UpdateCustomState(payload.ObjectID, payload.CustomState); err != nil {
				r.logger.Warn("room command: CmdObjectUpdate custom state failed",
					slog.String("object_id", string(payload.ObjectID)),
					slog.String("error", err.Error()),
				)
			}
		}
		r.sessionMu.Unlock()
		r.logger.Debug("room command: object update",
			slog.String("object_id", string(payload.ObjectID)),
		)

	default:
		r.logger.Warn("room command: unknown kind",
			slog.Int("kind", int(cmd.Kind)),
		)
	}
}
