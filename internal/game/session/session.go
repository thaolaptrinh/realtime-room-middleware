package session

import (
	"time"

	"github.com/thaonguyen/realtime-room-middleware/internal/transport"
)

// SessionID is the unique identifier of a transport session.
type SessionID string

// UserID is the externally authenticated user identity (from JoinRoom message).
type UserID string

// SessionState is the lifecycle state of a game-level session.
type SessionState int

const (
	// SessionStatePending — connected but not yet attached to a room.
	SessionStatePending SessionState = iota
	// SessionStateAttached — attached to a room instance.
	SessionStateAttached
	// SessionStateDetached — removed from a room; may reconnect.
	SessionStateDetached
	// SessionStateClosed — transport connection closed; session is gone.
	SessionStateClosed
)

func (s SessionState) String() string {
	switch s {
	case SessionStatePending:
		return "pending"
	case SessionStateAttached:
		return "attached"
	case SessionStateDetached:
		return "detached"
	case SessionStateClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// Session is the game-level record for a single connected client.
// It wraps the underlying transport.RealtimeSession and tracks room attachment.
//
// Session is not goroutine-safe on its own. All mutations go through
// SessionManager, which holds the appropriate lock.
type Session struct {
	id             SessionID
	userID         UserID
	roomInstanceID string // empty until Attach is called
	transportType  transport.TransportType
	conn           transport.RealtimeSession
	state          SessionState
	createdAt      time.Time
}

// ID returns the session identifier (matches the transport session ID).
func (s *Session) ID() SessionID { return s.id }

// UserID returns the authenticated user identity. Empty until Attach is called.
func (s *Session) UserID() UserID { return s.userID }

// RoomInstanceID returns the physical room instance this session is attached to.
// Empty when state is Pending, Detached, or Closed.
func (s *Session) RoomInstanceID() string { return s.roomInstanceID }

// Transport returns which realtime transport protocol this session uses.
// For observability only — must not gate game logic.
func (s *Session) Transport() transport.TransportType { return s.transportType }

// State returns the current session lifecycle state.
func (s *Session) State() SessionState { return s.state }

// CreatedAt returns the time the session was registered.
func (s *Session) CreatedAt() time.Time { return s.createdAt }

// Send queues a MessagePack packet for delivery to the client.
// Delegates to the underlying transport session.
func (s *Session) Send(packet []byte) error {
	return s.conn.Send(packet)
}

// Close terminates the underlying transport connection. Idempotent.
func (s *Session) Close() error {
	return s.conn.Close()
}
