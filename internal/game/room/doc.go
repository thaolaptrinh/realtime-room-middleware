// Package room implements the Room struct, room tick loop, RoomManager, and
// in-memory RoomRegistry for Phase 1 single-vps mode.
//
// # Mutation rule
//
// Only the room loop goroutine (runTick) may mutate room state.
// Transport goroutines (KCP, WebSocket) must enqueue RoomCommands via Room.Enqueue.
//
// # Room lifecycle
//
//	newRoom()   → status Created
//	Start(ctx)  → status Running  (tick goroutine launched)
//	Stop()      → status Draining → status Closed  (tick goroutine exits)
//
// # Phase 1 scope
//
// This package implements the room domain foundation for single-vps mode.
// Player sync, spatial hashing, delta broadcast, object locking, voice grouping,
// and Redis/KEDA integration are deferred to later milestones.
package room
