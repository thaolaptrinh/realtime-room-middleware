package delta

import "github.com/thaonguyen/realtime-room-middleware/internal/game/player"

// ClientSnapshot tracks the last-known state that was broadcast to a single client session.
//
// VisiblePlayers maps PlayerID → last-sent version. An entry means the client
// currently holds that player in its visible set at that version.
//
// Only the room loop goroutine reads and writes ClientSnapshot.
// Not goroutine-safe by design.
type ClientSnapshot struct {
	VisiblePlayers map[player.PlayerID]uint32
}

// NewClientSnapshot creates a fresh snapshot for a newly joined session.
func NewClientSnapshot() *ClientSnapshot {
	return &ClientSnapshot{
		VisiblePlayers: make(map[player.PlayerID]uint32),
	}
}

// SnapshotCache holds per-session ClientSnapshots keyed by session ID string.
//
// Only the room loop goroutine reads and writes SnapshotCache.
// Not goroutine-safe by design.
type SnapshotCache struct {
	snapshots map[string]*ClientSnapshot
}

// NewSnapshotCache creates an empty cache.
func NewSnapshotCache() *SnapshotCache {
	return &SnapshotCache{
		snapshots: make(map[string]*ClientSnapshot),
	}
}

// GetOrCreate returns the snapshot for sessionID, creating an empty one if absent.
func (c *SnapshotCache) GetOrCreate(sessionID string) *ClientSnapshot {
	s, ok := c.snapshots[sessionID]
	if !ok {
		s = NewClientSnapshot()
		c.snapshots[sessionID] = s
	}
	return s
}

// Remove deletes the snapshot for sessionID. No-op if absent.
func (c *SnapshotCache) Remove(sessionID string) {
	delete(c.snapshots, sessionID)
}

// Len returns the number of tracked sessions.
func (c *SnapshotCache) Len() int {
	return len(c.snapshots)
}
