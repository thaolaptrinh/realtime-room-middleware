package realtime

import (
	"github.com/thaonguyen/realtime-room-middleware/internal/game/bridge"
	"github.com/thaonguyen/realtime-room-middleware/internal/game/delta"
	"github.com/thaonguyen/realtime-room-middleware/internal/game/room"
)

// RoomDeltaBroadcaster adapts the bridge dispatcher to the room.Broadcaster
// interface without making room or transport packages depend on each other.
type RoomDeltaBroadcaster struct {
	dispatcher *bridge.DeltaBroadcaster
}

// NewRoomDeltaBroadcaster creates an app-layer adapter for room delta dispatch.
func NewRoomDeltaBroadcaster(dispatcher *bridge.DeltaBroadcaster) *RoomDeltaBroadcaster {
	return &RoomDeltaBroadcaster{dispatcher: dispatcher}
}

// BroadcastDelta converts typed room session IDs to bridge session IDs and
// delegates MessagePack Protocol v1 encoding/sending to the bridge package.
func (b *RoomDeltaBroadcaster) BroadcastDelta(batches map[room.SessionID]*delta.DeltaBatch) {
	if b == nil || b.dispatcher == nil || len(batches) == 0 {
		return
	}

	converted := make(map[string]*delta.DeltaBatch, len(batches))
	for sessionID, batch := range batches {
		converted[string(sessionID)] = batch
	}
	b.dispatcher.BroadcastDelta(converted)
}

var _ room.Broadcaster = (*RoomDeltaBroadcaster)(nil)
