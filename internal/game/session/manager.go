package session

import (
	"fmt"
	"sync"
	"time"

	"github.com/thaonguyen/realtime-room-middleware/internal/transport"
)

// SessionManager is the global registry of active game sessions.
// It is goroutine-safe. Each operation acquires the appropriate lock.
//
// SessionManager does NOT own room state. Room attach/detach is coordinated
// by the caller (typically a packet handler goroutine) that also enqueues
// commands into the room loop.
type SessionManager struct {
	mu     sync.RWMutex
	byID   map[SessionID]*Session
	byUser map[UserID]*Session // one active session per user
}

// NewSessionManager returns a ready-to-use SessionManager.
func NewSessionManager() *SessionManager {
	return &SessionManager{
		byID:   make(map[SessionID]*Session),
		byUser: make(map[UserID]*Session),
	}
}

// Register creates and stores a new session in the Pending state.
// Returns an error if a session with the same ID already exists.
//
// The session starts with no user identity; call Attach to assign one.
func (m *SessionManager) Register(
	id SessionID,
	t transport.TransportType,
	conn transport.RealtimeSession,
) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.byID[id]; exists {
		return nil, fmt.Errorf("session %q already registered", id)
	}

	sess := &Session{
		id:            id,
		transportType: t,
		conn:          conn,
		state:         SessionStatePending,
		createdAt:     time.Now(),
	}

	m.byID[id] = sess
	return sess, nil
}

// Get returns the session with the given ID, or (nil, false) if not found.
func (m *SessionManager) Get(id SessionID) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.byID[id]
	return s, ok
}

// GetByUser returns the session for the given user ID, or (nil, false) if not found.
func (m *SessionManager) GetByUser(userID UserID) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.byUser[userID]
	return s, ok
}

// Attach assigns a user identity and room instance to a Pending session,
// transitioning it to the Attached state.
//
// Returns an error if:
//   - the session is not found,
//   - the session is not in Pending state, or
//   - another active session already owns the given userID.
func (m *SessionManager) Attach(id SessionID, userID UserID, roomInstanceID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	sess, ok := m.byID[id]
	if !ok {
		return fmt.Errorf("session %q not found", id)
	}
	if sess.state != SessionStatePending {
		return fmt.Errorf("session %q cannot attach: current state is %s", id, sess.state)
	}
	if userID != "" {
		if existing, exists := m.byUser[userID]; exists && existing.id != id {
			return fmt.Errorf("user %q already has an active session (%s)", userID, existing.id)
		}
	}

	sess.userID = userID
	sess.roomInstanceID = roomInstanceID
	sess.state = SessionStateAttached
	if userID != "" {
		m.byUser[userID] = sess
	}
	return nil
}

// Detach removes a session from its room, transitioning it to Detached state.
// Idempotent if already Detached or Closed.
func (m *SessionManager) Detach(id SessionID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	sess, ok := m.byID[id]
	if !ok {
		return fmt.Errorf("session %q not found", id)
	}
	if sess.state == SessionStateClosed || sess.state == SessionStateDetached {
		return nil
	}

	if sess.userID != "" {
		delete(m.byUser, sess.userID)
	}
	sess.roomInstanceID = ""
	sess.userID = ""
	sess.state = SessionStateDetached
	return nil
}

// Close terminates the transport connection and removes the session from all
// indexes. Idempotent — calling Close on an already-Closed session returns an
// error (session has already been removed).
func (m *SessionManager) Close(id SessionID) error {
	m.mu.Lock()
	sess, ok := m.byID[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("session %q not found", id)
	}
	if sess.state == SessionStateClosed {
		m.mu.Unlock()
		return nil
	}

	userID := sess.userID
	delete(m.byID, id)
	if userID != "" {
		delete(m.byUser, userID)
	}
	sess.state = SessionStateClosed
	m.mu.Unlock()

	return sess.conn.Close()
}

// ActiveCount returns the number of registered sessions.
func (m *SessionManager) ActiveCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.byID)
}
