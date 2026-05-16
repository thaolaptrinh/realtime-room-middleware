package room

import (
	"context"
	"fmt"
)

// Start launches the tick loop goroutine.
// The room must be in RoomStatusCreated state.
// Returns an error if called on a room that is already running or closed.
func (r *Room) Start(ctx context.Context) error {
	r.statusMu.Lock()
	defer r.statusMu.Unlock()

	if r.status != RoomStatusCreated {
		return fmt.Errorf("room %q cannot start from status %s", r.instanceID, r.status)
	}

	tickCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	r.status = RoomStatusRunning

	go r.runTick(tickCtx)

	r.logger.Info("room started")
	return nil
}

// Stop initiates a graceful shutdown of the room tick loop.
//
// Transitions the room to Draining, cancels the tick context, waits for the
// tick goroutine to exit, then sets status to Closed.
//
// If the room was never started (Created state), Stop transitions it directly
// to Closed without waiting. If already Closed, Stop is a no-op.
func (r *Room) Stop() {
	r.statusMu.Lock()

	switch r.status {
	case RoomStatusClosed:
		r.statusMu.Unlock()
		return
	case RoomStatusCreated:
		// Never started; close directly without waiting on a goroutine.
		r.status = RoomStatusClosed
		r.statusMu.Unlock()
		r.logger.Info("room closed (never started)")
		return
	}

	r.status = RoomStatusDraining
	cancel := r.cancel
	r.statusMu.Unlock()

	if cancel != nil {
		cancel()
	}
	<-r.done

	r.statusMu.Lock()
	r.status = RoomStatusClosed
	r.statusMu.Unlock()

	r.logger.Info("room stopped")
}
