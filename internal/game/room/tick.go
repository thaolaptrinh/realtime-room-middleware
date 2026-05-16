package room

import (
	"context"
	"log/slog"
	"time"

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
// Each tick: drains the command queue, then broadcasts deltas at the configured
// broadcast rate (default 10 Hz). Future milestones will add object lock expiry,
// voice group allocation, and packet encoding/sending.
func (r *Room) tick() {
	tick := r.currentTick.Add(1)
	r.drainCommands(tick)
	if r.isBroadcastTick(tick) {
		r.broadcast(tick)
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
			if p, exists := r.players[player.PlayerID(cmd.PlayerID)]; exists {
				p.MarkStatus(player.PlayerStatusLeaving)
				delete(r.players, player.PlayerID(cmd.PlayerID))
			}
			r.spatial.Remove(spatial.EntityID(cmd.PlayerID))
			r.snapshotCache.Remove(string(cmd.SessionID))
			delete(r.dirtyPlayers, player.PlayerID(cmd.PlayerID))
		}
		r.sessionMu.Unlock()

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
			if p, exists := r.players[player.PlayerID(att.playerID)]; exists {
				p.MarkStatus(player.PlayerStatusGone)
				delete(r.players, player.PlayerID(att.playerID))
			}
			r.spatial.Remove(spatial.EntityID(att.playerID))
			r.snapshotCache.Remove(string(cmd.SessionID))
			delete(r.dirtyPlayers, player.PlayerID(att.playerID))
		}
		r.sessionMu.Unlock()

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

	default:
		r.logger.Warn("room command: unknown kind",
			slog.Int("kind", int(cmd.Kind)),
		)
	}
}
