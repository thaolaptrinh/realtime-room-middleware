package room

import (
	"context"
	"log/slog"
	"time"
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
// Current implementation: drains and dispatches the command queue.
//
// Future milestones will extend this to:
//   - update player state
//   - update object state and release expired locks
//   - update spatial hash
//   - compute per-client interest sets
//   - allocate voice/proximity groups
//   - build and enqueue delta packets
func (r *Room) tick() {
	r.drainCommands()
}

// drainCommands processes all queued commands without blocking.
func (r *Room) drainCommands() {
	for {
		select {
		case cmd := <-r.commands:
			r.handleCommand(cmd)
		default:
			return
		}
	}
}

// handleCommand dispatches a single RoomCommand.
// Only the room loop calls this function — it is the mutation boundary.
func (r *Room) handleCommand(cmd RoomCommand) {
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
		r.sessionMu.Unlock()
		r.playerCount.Add(1)

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
		r.sessionMu.Unlock()
		if ok {
			r.playerCount.Add(-1)
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
		r.sessionMu.Unlock()
		if ok {
			r.playerCount.Add(-1)
		}

		r.logger.Debug("room command: disconnect",
			slog.String("session_id", string(cmd.SessionID)),
		)

	default:
		r.logger.Warn("room command: unknown kind",
			slog.Int("kind", int(cmd.Kind)),
		)
	}
}
