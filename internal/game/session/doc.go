// Package session manages the lifecycle of game-level sessions.
//
// A session represents one connected client. It wraps the underlying
// transport.RealtimeSession and tracks room attachment state.
//
// Lifecycle:
//
//	Register() → Pending
//	Attach()   → Attached
//	Detach()   → Detached
//	Close()    → Closed (and removes from all indexes)
//
// SessionManager is the global registry. It is goroutine-safe.
//
// This package does not import internal/game/room. Session-to-room association
// is coordinated by the caller via room command queues.
package session
