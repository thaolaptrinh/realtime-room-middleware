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
// Current implementation: increments tick counter, drains and dispatches the command queue.
//
// Future milestones will extend this to:
//   - update player state
//   - update object state and release expired locks
//   - compute per-client interest sets
//   - allocate voice/proximity groups
//   - build and enqueue delta packets
func (r *Room) tick() {
	tick := r.currentTick.Add(1)
	r.drainCommands(tick)
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
