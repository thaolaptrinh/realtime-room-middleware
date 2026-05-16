// Package delta provides the per-client delta broadcast types and builder for
// player state updates.
//
// The DeltaBuilder computes what each client needs to receive in a broadcast
// tick by comparing the current room interest set against the client's
// ClientSnapshot (last-sent state). It emits Enter, Update, and Leave entries
// without caring which transport (KCP or WebSocket) the target session uses.
//
// Only the room loop goroutine may call DeltaBuilder.BuildPlayerDelta and
// mutate SnapshotCache entries. External callers may read snapshot metadata
// via SnapshotCache.Len under the room's sessionMu.
package delta
