// Package bridge connects the room delta output to the transport layer.
//
// It converts domain delta types (internal/game/delta) to protocol wire types
// (internal/protocol), encodes them as MessagePack envelopes, and sends them
// through the shared RealtimeSession interface (internal/transport).
//
// The bridge does not import the room package. Room defines a Broadcaster
// interface that can be satisfied by the bridge's DeltaBroadcaster via a
// thin adapter at server wiring time.
//
// Boundary rules:
//   - protocol must not import game packages.
//   - transport packages must not import game packages.
//   - delta builder must not depend on transport.
//   - Bridge may import delta, player, protocol, and transport.
package bridge

import (
	"fmt"

	"github.com/thaonguyen/realtime-room-middleware/internal/game/delta"
	"github.com/thaonguyen/realtime-room-middleware/internal/game/player"
	"github.com/thaonguyen/realtime-room-middleware/internal/protocol"
	"github.com/thaonguyen/realtime-room-middleware/internal/transport"
)

// SessionLookup retrieves a RealtimeSession by session ID.
// Returns nil if the session is no longer connected.
type SessionLookup func(sessionID string) transport.RealtimeSession

// SendResult reports the outcome of sending to one session.
type SendResult struct {
	SessionID string
	Transport transport.TransportType
	Error     error
}

// PlayerStateView is a lightweight view of player state for FullSnapshot construction.
// The caller (room or wiring code) builds this from PlayerState.
type PlayerStateView struct {
	PlayerID  player.PlayerID
	Transform player.PlayerTransform
	Version   uint32
}

// DeltaBroadcaster dispatches encoded delta batches to sessions via RealtimeSession.
// Create with NewDeltaBroadcaster. The session lookup is provided at construction time
// and is called for each session in the batch map.
type DeltaBroadcaster struct {
	lookup SessionLookup
}

// NewDeltaBroadcaster creates a broadcaster that uses the given session lookup.
func NewDeltaBroadcaster(lookup SessionLookup) *DeltaBroadcaster {
	return &DeltaBroadcaster{lookup: lookup}
}

// BroadcastDelta sends delta batches to all sessions in the map.
// It is a convenience wrapper around DispatchBatches.
// Errors are collected but not returned; use DispatchBatches directly
// for per-session error reporting.
func (d *DeltaBroadcaster) BroadcastDelta(batches map[string]*delta.DeltaBatch) {
	_ = DispatchBatches(batches, d.lookup)
}

// ConvertPlayerDelta converts a domain delta.PlayerDelta to a protocol PlayerDelta wire struct.
// AnimState is set to 0 (not tracked in the domain transform yet).
func ConvertPlayerDelta(d *delta.PlayerDelta) protocol.PlayerDelta {
	if d == nil {
		return protocol.PlayerDelta{}
	}

	enters := make([]protocol.PlayerEnterDelta, len(d.Enters))
	for i, e := range d.Enters {
		enters[i] = protocol.PlayerEnterDelta{
			PlayerID: string(e.PlayerID),
			X:        e.Transform.Position.X,
			Z:        e.Transform.Position.Z,
			Yaw:      yawFromRotation(e.Transform.Rotation),
			Version:  e.Version,
		}
	}

	updates := make([]protocol.PlayerUpdateDelta, len(d.Updates))
	for i, u := range d.Updates {
		updates[i] = protocol.PlayerUpdateDelta{
			PlayerID: string(u.PlayerID),
			X:        u.Transform.Position.X,
			Z:        u.Transform.Position.Z,
			Yaw:      yawFromRotation(u.Transform.Rotation),
			Version:  u.Version,
		}
	}

	leaves := make([]protocol.PlayerLeaveDelta, len(d.Leaves))
	for i, l := range d.Leaves {
		leaves[i] = protocol.PlayerLeaveDelta{
			PlayerID: string(l.PlayerID),
		}
	}

	return protocol.PlayerDelta{
		Tick:    d.Tick,
		Enters:  enters,
		Updates: updates,
		Leaves:  leaves,
	}
}

// ConvertFullSnapshot converts player state views to a protocol FullSnapshot wire struct.
func ConvertFullSnapshot(tick uint32, players []PlayerStateView) protocol.FullSnapshot {
	snapshots := make([]protocol.PlayerSnapshot, len(players))
	for i, p := range players {
		snapshots[i] = protocol.PlayerSnapshot{
			PlayerID: string(p.PlayerID),
			X:        p.Transform.Position.X,
			Z:        p.Transform.Position.Z,
			Yaw:      yawFromRotation(p.Transform.Rotation),
			Version:  p.Version,
		}
	}
	return protocol.FullSnapshot{
		Tick:    tick,
		Players: snapshots,
	}
}

// SendPlayerDelta encodes a domain PlayerDelta and sends it through a RealtimeSession.
func SendPlayerDelta(sess transport.RealtimeSession, pd *delta.PlayerDelta) error {
	wire := ConvertPlayerDelta(pd)
	data, err := protocol.EncodeAndWrap(protocol.CurrentVersion, protocol.TypePlayerDelta, 0, pd.Tick, wire)
	if err != nil {
		return fmt.Errorf("encode player delta: %w", err)
	}
	return sess.Send(data)
}

// SendFullSnapshot encodes a FullSnapshot and sends it through a RealtimeSession.
func SendFullSnapshot(sess transport.RealtimeSession, tick uint32, players []PlayerStateView) error {
	wire := ConvertFullSnapshot(tick, players)
	data, err := protocol.EncodeAndWrap(protocol.CurrentVersion, protocol.TypeFullSnapshot, 0, tick, wire)
	if err != nil {
		return fmt.Errorf("encode full snapshot: %w", err)
	}
	return sess.Send(data)
}

// DispatchBatches sends encoded delta batches to all sessions via the lookup function.
// Returns per-session send results, including errors for missing or failed sessions.
// Empty batches are skipped. Does not mutate room state — it only reads the batch map.
func DispatchBatches(batches map[string]*delta.DeltaBatch, lookup SessionLookup) []SendResult {
	if len(batches) == 0 {
		return nil
	}

	results := make([]SendResult, 0, len(batches))
	for sessionID, batch := range batches {
		if batch == nil || batch.IsEmpty() {
			continue
		}
		if batch.PlayerDelta == nil || batch.PlayerDelta.IsEmpty() {
			continue
		}

		sess := lookup(sessionID)
		if sess == nil {
			results = append(results, SendResult{
				SessionID: sessionID,
				Error:     fmt.Errorf("session not found: %s", sessionID),
			})
			continue
		}

		err := SendPlayerDelta(sess, batch.PlayerDelta)
		results = append(results, SendResult{
			SessionID: sessionID,
			Transport: sess.Transport(),
			Error:     err,
		})
	}
	return results
}

// yawFromRotation extracts a yaw angle from the rotation quaternion's Y component.
// This is a simplified mapping for Phase 1; a proper quaternion-to-Euler conversion
// can be substituted when needed.
func yawFromRotation(q player.Quaternion) float32 {
	return q.Y
}
