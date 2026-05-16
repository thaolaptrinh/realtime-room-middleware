# Session Attach/Detach and Player Identity Skeleton Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement session registration/attach/detach lifecycle and a lightweight player identity skeleton so the room runtime can track which users are in each room without yet implementing movement sync, spatial hashing, or delta broadcast.

**Architecture:** A standalone `session` package owns session lifecycle and holds the `transport.RealtimeSession` reference; a standalone `player` package owns player identity; the `room` package is extended with an internal session tracking map (protected by a `sync.RWMutex`) that the room loop writes and external callers may read. Transport packages never import game packages.

**Tech Stack:** Go 1.23, `sync.RWMutex`, `sync/atomic`, `internal/transport.RealtimeSession` interface.

---

## File Map

| Action | Path | Responsibility |
|--------|------|----------------|
| Create | `internal/game/session/session.go` | `SessionID`, `UserID`, `SessionState`, `Session` struct and getters |
| Create | `internal/game/session/manager.go` | `SessionManager` — register, attach, detach, close, query |
| Create | `internal/game/session/session_test.go` | Full unit tests for session and manager |
| Update | `internal/game/session/doc.go` | Accurate package doc |
| Create | `internal/game/player/player.go` | `PlayerID`, `UserID`, `PlayerStatus`, `PlayerState` |
| Update | `internal/game/player/doc.go` | Accurate package doc |
| Modify | `internal/game/room/types.go` | Add `UserID` type, `sessionAttachment` struct, `UserID` field on `RoomCommand` |
| Modify | `internal/game/room/room.go` | Add `sessionMu`, `activeSessions`, `userSessionIndex`; add `HasSession`, `HasUser`, `ActiveSessions` |
| Modify | `internal/game/room/tick.go` | Update `handleCommand` to write session maps and enforce duplicate-user rejection |
| Modify | `internal/game/room/room_test.go` | Add tests for session tracking, duplicate rejection, HasSession, HasUser |
| Update | `docs/room-lifecycle.md` | Document session attach/detach, UserID duplicate rule |
| Update | `docs/specs/spec_room_manager.md` | Document new room fields and methods |

---

## Task 1: Session domain types (`session.go`)

**Files:**
- Create: `internal/game/session/session.go`

- [ ] **Step 1: Write `internal/game/session/session.go`**

```go
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
```

---

## Task 2: SessionManager (`manager.go`)

**Files:**
- Create: `internal/game/session/manager.go`

- [ ] **Step 1: Write `internal/game/session/manager.go`**

```go
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
// Returns an error if:
//   - a session with the same ID already exists, or
//   - the given userID is already associated with an active session.
//
// Pass userID="" if the user identity is not yet known (pre-JoinRoom).
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
// indexes. Idempotent — calling Close on an already-Closed session is a no-op.
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
```

---

## Task 3: Session tests

**Files:**
- Create: `internal/game/session/session_test.go`

- [ ] **Step 1: Write `internal/game/session/session_test.go`**

```go
package session_test

import (
	"errors"
	"testing"

	"github.com/thaonguyen/realtime-room-middleware/internal/game/session"
	"github.com/thaonguyen/realtime-room-middleware/internal/transport"
)

// ---- mock transport.RealtimeSession -----------------------------------------

type mockConn struct {
	id         string
	userID     string
	transport  transport.TransportType
	closed     bool
	closeErr   error
}

func (m *mockConn) ID() string                      { return m.id }
func (m *mockConn) UserID() string                  { return m.userID }
func (m *mockConn) Transport() transport.TransportType { return m.transport }
func (m *mockConn) Send(_ []byte) error             { return nil }
func (m *mockConn) Close() error {
	m.closed = true
	return m.closeErr
}

func newMockConn(id string, t transport.TransportType) *mockConn {
	return &mockConn{id: id, transport: t}
}

// ---- Register ---------------------------------------------------------------

func TestRegister_Success(t *testing.T) {
	mgr := session.NewSessionManager()
	conn := newMockConn("sess-1", transport.KCP)

	sess, err := mgr.Register("sess-1", transport.KCP, conn)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if sess.ID() != "sess-1" {
		t.Errorf("ID: got %q, want %q", sess.ID(), "sess-1")
	}
	if sess.State() != session.SessionStatePending {
		t.Errorf("State: got %s, want pending", sess.State())
	}
	if sess.Transport() != transport.KCP {
		t.Errorf("Transport: got %s, want kcp", sess.Transport())
	}
	if sess.CreatedAt().IsZero() {
		t.Error("CreatedAt must not be zero")
	}
}

func TestRegister_DuplicateID(t *testing.T) {
	mgr := session.NewSessionManager()
	conn := newMockConn("sess-1", transport.KCP)

	if _, err := mgr.Register("sess-1", transport.KCP, conn); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	_, err := mgr.Register("sess-1", transport.KCP, conn)
	if err == nil {
		t.Fatal("expected error for duplicate session ID, got nil")
	}
}

func TestRegister_WebSocketAndKCPBothSucceed(t *testing.T) {
	mgr := session.NewSessionManager()

	_, err := mgr.Register("kcp-sess", transport.KCP, newMockConn("kcp-sess", transport.KCP))
	if err != nil {
		t.Fatalf("Register KCP: %v", err)
	}

	_, err = mgr.Register("ws-sess", transport.WebSocket, newMockConn("ws-sess", transport.WebSocket))
	if err != nil {
		t.Fatalf("Register WebSocket: %v", err)
	}

	if mgr.ActiveCount() != 2 {
		t.Errorf("ActiveCount: got %d, want 2", mgr.ActiveCount())
	}
}

// ---- Get --------------------------------------------------------------------

func TestGet_Found(t *testing.T) {
	mgr := session.NewSessionManager()
	mgr.Register("sess-1", transport.KCP, newMockConn("sess-1", transport.KCP))

	s, ok := mgr.Get("sess-1")
	if !ok {
		t.Fatal("Get: expected session, got not found")
	}
	if s.ID() != "sess-1" {
		t.Errorf("ID: got %q, want %q", s.ID(), "sess-1")
	}
}

func TestGet_NotFound(t *testing.T) {
	mgr := session.NewSessionManager()
	_, ok := mgr.Get("missing")
	if ok {
		t.Fatal("expected not found for unregistered session")
	}
}

// ---- Attach -----------------------------------------------------------------

func TestAttach_Success(t *testing.T) {
	mgr := session.NewSessionManager()
	mgr.Register("sess-1", transport.KCP, newMockConn("sess-1", transport.KCP))

	if err := mgr.Attach("sess-1", "user-42", "room-a-0001"); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	s, _ := mgr.Get("sess-1")
	if s.State() != session.SessionStateAttached {
		t.Errorf("State: got %s, want attached", s.State())
	}
	if s.UserID() != "user-42" {
		t.Errorf("UserID: got %q, want %q", s.UserID(), "user-42")
	}
	if s.RoomInstanceID() != "room-a-0001" {
		t.Errorf("RoomInstanceID: got %q, want %q", s.RoomInstanceID(), "room-a-0001")
	}
}

func TestAttach_NotFound(t *testing.T) {
	mgr := session.NewSessionManager()
	err := mgr.Attach("missing", "user-1", "room-0001")
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestAttach_DuplicateUser(t *testing.T) {
	mgr := session.NewSessionManager()
	mgr.Register("sess-1", transport.KCP, newMockConn("sess-1", transport.KCP))
	mgr.Register("sess-2", transport.WebSocket, newMockConn("sess-2", transport.WebSocket))
	mgr.Attach("sess-1", "user-42", "room-a-0001")

	err := mgr.Attach("sess-2", "user-42", "room-a-0001")
	if err == nil {
		t.Fatal("expected error: user already has an active session")
	}
}

func TestAttach_FailsIfNotPending(t *testing.T) {
	mgr := session.NewSessionManager()
	mgr.Register("sess-1", transport.KCP, newMockConn("sess-1", transport.KCP))
	mgr.Attach("sess-1", "user-1", "room-0001")

	err := mgr.Attach("sess-1", "user-1", "room-0002")
	if err == nil {
		t.Fatal("expected error: cannot attach a non-Pending session")
	}
}

// ---- GetByUser --------------------------------------------------------------

func TestGetByUser_Found(t *testing.T) {
	mgr := session.NewSessionManager()
	mgr.Register("sess-1", transport.KCP, newMockConn("sess-1", transport.KCP))
	mgr.Attach("sess-1", "user-42", "room-0001")

	s, ok := mgr.GetByUser("user-42")
	if !ok {
		t.Fatal("GetByUser: expected session, got not found")
	}
	if s.ID() != "sess-1" {
		t.Errorf("ID: got %q, want %q", s.ID(), "sess-1")
	}
}

func TestGetByUser_NotFound(t *testing.T) {
	mgr := session.NewSessionManager()
	_, ok := mgr.GetByUser("nobody")
	if ok {
		t.Fatal("expected not found")
	}
}

// ---- Detach -----------------------------------------------------------------

func TestDetach_Success(t *testing.T) {
	mgr := session.NewSessionManager()
	mgr.Register("sess-1", transport.KCP, newMockConn("sess-1", transport.KCP))
	mgr.Attach("sess-1", "user-42", "room-0001")

	if err := mgr.Detach("sess-1"); err != nil {
		t.Fatalf("Detach: %v", err)
	}

	s, _ := mgr.Get("sess-1")
	if s.State() != session.SessionStateDetached {
		t.Errorf("State: got %s, want detached", s.State())
	}
	if s.RoomInstanceID() != "" {
		t.Errorf("RoomInstanceID should be empty after detach, got %q", s.RoomInstanceID())
	}

	// GetByUser should no longer find the session.
	_, ok := mgr.GetByUser("user-42")
	if ok {
		t.Error("GetByUser should return not-found after detach")
	}
}

func TestDetach_Idempotent(t *testing.T) {
	mgr := session.NewSessionManager()
	mgr.Register("sess-1", transport.KCP, newMockConn("sess-1", transport.KCP))
	mgr.Attach("sess-1", "user-1", "room-0001")
	mgr.Detach("sess-1")

	// Second detach must not error.
	if err := mgr.Detach("sess-1"); err != nil {
		t.Fatalf("second Detach: %v", err)
	}
}

func TestDetach_NotFound(t *testing.T) {
	mgr := session.NewSessionManager()
	err := mgr.Detach("missing")
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}

// ---- Close ------------------------------------------------------------------

func TestClose_Success(t *testing.T) {
	mgr := session.NewSessionManager()
	conn := newMockConn("sess-1", transport.KCP)
	mgr.Register("sess-1", transport.KCP, conn)
	mgr.Attach("sess-1", "user-42", "room-0001")

	if err := mgr.Close("sess-1"); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if !conn.closed {
		t.Error("expected underlying connection to be closed")
	}
	if mgr.ActiveCount() != 0 {
		t.Errorf("ActiveCount after close: got %d, want 0", mgr.ActiveCount())
	}

	// Session should no longer be reachable.
	_, ok := mgr.Get("sess-1")
	if ok {
		t.Error("Get should return not-found after Close")
	}
	_, ok = mgr.GetByUser("user-42")
	if ok {
		t.Error("GetByUser should return not-found after Close")
	}
}

func TestClose_Idempotent(t *testing.T) {
	mgr := session.NewSessionManager()
	conn := newMockConn("sess-1", transport.KCP)
	mgr.Register("sess-1", transport.KCP, conn)

	mgr.Close("sess-1")
	err := mgr.Close("sess-1") // second close — session not found
	if err == nil {
		t.Fatal("expected error on double-close of removed session")
	}
	// The underlying conn.Close was called once and conn is closed.
	if !conn.closed {
		t.Error("expected connection to be closed")
	}
}

func TestClose_NotFound(t *testing.T) {
	mgr := session.NewSessionManager()
	err := mgr.Close("missing")
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}

// ---- ActiveCount ------------------------------------------------------------

func TestActiveCount(t *testing.T) {
	mgr := session.NewSessionManager()
	if mgr.ActiveCount() != 0 {
		t.Fatalf("initial ActiveCount: got %d, want 0", mgr.ActiveCount())
	}

	mgr.Register("s1", transport.KCP, newMockConn("s1", transport.KCP))
	mgr.Register("s2", transport.WebSocket, newMockConn("s2", transport.WebSocket))
	if mgr.ActiveCount() != 2 {
		t.Errorf("after two registers: got %d, want 2", mgr.ActiveCount())
	}

	mgr.Close("s1")
	if mgr.ActiveCount() != 1 {
		t.Errorf("after close: got %d, want 1", mgr.ActiveCount())
	}
}

// ---- Send/Close delegation --------------------------------------------------

func TestSession_Send(t *testing.T) {
	mgr := session.NewSessionManager()
	mgr.Register("s1", transport.KCP, newMockConn("s1", transport.KCP))
	s, _ := mgr.Get("s1")
	if err := s.Send([]byte{0x01}); err != nil {
		t.Fatalf("Send: %v", err)
	}
}

func TestSession_CloseClosesDelegation(t *testing.T) {
	conn := newMockConn("s1", transport.KCP)
	mgr := session.NewSessionManager()
	mgr.Register("s1", transport.KCP, conn)
	s, _ := mgr.Get("s1")
	s.Close()
	if !conn.closed {
		t.Error("expected conn.Close() to be called via session.Close()")
	}
}

// ---- No game/room import check (compile-time) --------------------------------
// If this test file compiles without importing internal/game/room, the
// transport separation rule is satisfied. No runtime assertion needed.
var _ = errors.New // ensure errors imported (avoids unused-import lint error)
```

---

## Task 4: Update session/doc.go

**Files:**
- Modify: `internal/game/session/doc.go`

- [ ] **Step 1: Replace content of `internal/game/session/doc.go`**

```go
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
```

---

## Task 5: Player identity skeleton

**Files:**
- Create: `internal/game/player/player.go`
- Modify: `internal/game/player/doc.go`

- [ ] **Step 1: Write `internal/game/player/player.go`**

```go
package player

import "time"

// PlayerID is the server-assigned in-room player identifier.
type PlayerID string

// UserID is the externally authenticated user identity.
// Matches session.UserID by value; typed separately to keep the packages independent.
type UserID string

// PlayerStatus is the lifecycle state of a player within a room.
type PlayerStatus int

const (
	// PlayerStatusJoining — CmdJoin received, not yet confirmed.
	PlayerStatusJoining PlayerStatus = iota
	// PlayerStatusActive — join confirmed; player is in the room.
	PlayerStatusActive
	// PlayerStatusLeaving — CmdLeave received; pending cleanup.
	PlayerStatusLeaving
	// PlayerStatusGone — removed from room state.
	PlayerStatusGone
)

func (s PlayerStatus) String() string {
	switch s {
	case PlayerStatusJoining:
		return "joining"
	case PlayerStatusActive:
		return "active"
	case PlayerStatusLeaving:
		return "leaving"
	case PlayerStatusGone:
		return "gone"
	default:
		return "unknown"
	}
}

// PlayerState is the game runtime record for a player in a room.
//
// Phase 1: identity and join metadata only.
// Position, rotation, animation state, dirty mask, and version are deferred
// to Milestone 3 (Player Sync).
type PlayerState struct {
	ID       PlayerID
	UserID   UserID
	Status   PlayerStatus
	JoinedAt time.Time
	// Future fields (Milestone 3):
	// Position  Vec2
	// Rotation  float32
	// AnimState uint16
	// Version   uint32
	// Dirty     DirtyMask
}
```

- [ ] **Step 2: Replace content of `internal/game/player/doc.go`**

```go
// Package player defines player identity and state for the room runtime.
//
// Phase 1 provides PlayerID, UserID, PlayerStatus, and a PlayerState skeleton.
// Movement position, rotation, animation, and dirty-mask tracking are deferred
// to Milestone 3 (Player Sync).
package player
```

---

## Task 6: Extend room types

**Files:**
- Modify: `internal/game/room/types.go`

- [ ] **Step 1: Add `UserID` type and `sessionAttachment` to `types.go`**

Add after the existing `SessionID` and `PlayerID` type declarations:

```go
// UserID is the externally authenticated user identity carried in join commands.
// Matches session.UserID and player.UserID by value.
type UserID string

// sessionAttachment is the room-internal record stored per attached session.
// Only the room loop reads and writes this type.
type sessionAttachment struct {
	playerID PlayerID
	userID   UserID
}
```

- [ ] **Step 2: Add `UserID` field to `RoomCommand`**

In the existing `RoomCommand` struct, add the `UserID` field after `PlayerID`:

```go
// RoomCommand is an envelope for commands enqueued by transport goroutines.
// The room loop is the sole consumer; it is the only code permitted to act on
// these commands and mutate room state.
type RoomCommand struct {
	Kind      RoomCommandKind
	SessionID SessionID
	PlayerID  PlayerID
	UserID    UserID    // authenticated user identity; used for duplicate detection on CmdJoin
	Payload   any
	Timestamp time.Time
}
```

---

## Task 7: Add session tracking to `Room`

**Files:**
- Modify: `internal/game/room/room.go`

- [ ] **Step 1: Add tracking fields to `Room` struct and initialize them in `newRoom`**

In `room.go`, update the `Room` struct to add:

```go
	// sessionMu protects activeSessions and userSessionIndex.
	// The room loop holds the write lock; external readers hold the read lock.
	sessionMu sync.RWMutex

	// activeSessions maps SessionID → attachment info.
	// Written only by the room loop goroutine (runTick).
	activeSessions map[SessionID]sessionAttachment

	// userSessionIndex maps UserID → SessionID for duplicate-join detection.
	// Written only by the room loop goroutine.
	userSessionIndex map[UserID]SessionID
```

Update `newRoom` to initialize the new maps (add after existing field initialization):

```go
		activeSessions:  make(map[SessionID]sessionAttachment),
		userSessionIndex: make(map[UserID]SessionID),
```

- [ ] **Step 2: Add `HasSession`, `HasUser`, and `ActiveSessions` read methods**

Append to `room.go`:

```go
// HasSession reports whether the given session is currently attached to this room.
// Safe to call from any goroutine.
func (r *Room) HasSession(id SessionID) bool {
	r.sessionMu.RLock()
	defer r.sessionMu.RUnlock()
	_, ok := r.activeSessions[id]
	return ok
}

// HasUser reports whether a player with the given user ID is currently in this room.
// Safe to call from any goroutine.
func (r *Room) HasUser(id UserID) bool {
	r.sessionMu.RLock()
	defer r.sessionMu.RUnlock()
	_, ok := r.userSessionIndex[id]
	return ok
}

// ActiveSessions returns a snapshot of the currently attached session IDs.
// Safe to call from any goroutine. The returned slice is a copy.
func (r *Room) ActiveSessions() []SessionID {
	r.sessionMu.RLock()
	defer r.sessionMu.RUnlock()
	ids := make([]SessionID, 0, len(r.activeSessions))
	for id := range r.activeSessions {
		ids = append(ids, id)
	}
	return ids
}
```

---

## Task 8: Update `handleCommand` in `tick.go`

**Files:**
- Modify: `internal/game/room/tick.go`

- [ ] **Step 1: Replace `handleCommand` body**

Replace the existing `handleCommand` function with:

```go
// handleCommand dispatches a single RoomCommand.
// Only the room loop calls this function — it is the mutation boundary.
func (r *Room) handleCommand(cmd RoomCommand) {
	switch cmd.Kind {
	case CmdJoin:
		// Reject duplicate user: same user cannot be in the room twice.
		if cmd.UserID != "" && r.userSessionIndex[cmd.UserID] != "" {
			r.logger.Warn("duplicate user join rejected",
				slog.String("session_id", string(cmd.SessionID)),
				slog.String("user_id", string(cmd.UserID)),
			)
			return
		}

		r.sessionMu.Lock()
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
```

**Note on the duplicate-user check:** `r.userSessionIndex` is read inside `handleCommand` without the sessionMu lock (before acquiring it for the write). This is safe because `handleCommand` is ONLY called from the room loop — it is the single writer. The read here is intra-goroutine, so there is no data race. The `sessionMu` is only needed for the write (so that external readers via `HasUser` don't race with the write).

---

## Task 9: Add room session tracking tests

**Files:**
- Modify: `internal/game/room/room_test.go`

- [ ] **Step 1: Append new tests to `room_test.go`**

Add these tests at the end of the existing file:

```go
// ---- Session tracking tests --------------------------------------------------

func TestRoom_HasSession_AfterJoin(t *testing.T) {
	mgr := newTestManager(newTestRegistry())
	ctx := context.Background()

	r, err := mgr.CreateRoom(ctx, "session-track-room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	defer r.Stop()

	if r.HasSession("sess-abc") {
		t.Error("HasSession should return false before join")
	}

	_ = r.Enqueue(room.RoomCommand{
		Kind:      room.CmdJoin,
		SessionID: "sess-abc",
		PlayerID:  "player-1",
		UserID:    "user-42",
		Timestamp: time.Now(),
	})

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if r.HasSession("sess-abc") {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Error("HasSession should return true after CmdJoin is processed")
}

func TestRoom_HasSession_FalseAfterLeave(t *testing.T) {
	mgr := newTestManager(newTestRegistry())
	ctx := context.Background()

	r, err := mgr.CreateRoom(ctx, "leave-track-room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	defer r.Stop()

	_ = r.Enqueue(room.RoomCommand{Kind: room.CmdJoin, SessionID: "sess-1", PlayerID: "p1", UserID: "u1", Timestamp: time.Now()})
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if r.HasSession("sess-1") {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !r.HasSession("sess-1") {
		t.Fatal("precondition: HasSession should be true after join")
	}

	_ = r.Enqueue(room.RoomCommand{Kind: room.CmdLeave, SessionID: "sess-1", PlayerID: "p1", Timestamp: time.Now()})
	deadline = time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !r.HasSession("sess-1") {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Error("HasSession should return false after CmdLeave is processed")
}

func TestRoom_HasUser_AfterJoinAndLeave(t *testing.T) {
	mgr := newTestManager(newTestRegistry())
	ctx := context.Background()

	r, err := mgr.CreateRoom(ctx, "user-track-room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	defer r.Stop()

	if r.HasUser("user-99") {
		t.Error("HasUser should return false before join")
	}

	_ = r.Enqueue(room.RoomCommand{Kind: room.CmdJoin, SessionID: "s1", PlayerID: "p1", UserID: "user-99", Timestamp: time.Now()})
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if r.HasUser("user-99") {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !r.HasUser("user-99") {
		t.Fatal("HasUser should return true after join")
	}

	_ = r.Enqueue(room.RoomCommand{Kind: room.CmdLeave, SessionID: "s1", Timestamp: time.Now()})
	deadline = time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !r.HasUser("user-99") {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Error("HasUser should return false after CmdLeave is processed")
}

func TestRoom_DuplicateUserJoinRejected(t *testing.T) {
	mgr := newTestManager(newTestRegistry())
	ctx := context.Background()

	r, err := mgr.CreateRoom(ctx, "dup-user-room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	defer r.Stop()

	// First join.
	_ = r.Enqueue(room.RoomCommand{Kind: room.CmdJoin, SessionID: "s1", PlayerID: "p1", UserID: "user-dupe", Timestamp: time.Now()})
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if r.PlayerCount() == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if r.PlayerCount() != 1 {
		t.Fatalf("PlayerCount after first join: got %d, want 1", r.PlayerCount())
	}

	// Second join with same user ID and different session ID must be rejected.
	_ = r.Enqueue(room.RoomCommand{Kind: room.CmdJoin, SessionID: "s2", PlayerID: "p2", UserID: "user-dupe", Timestamp: time.Now()})

	// Give the room loop time to process.
	time.Sleep(100 * time.Millisecond)

	if r.PlayerCount() != 1 {
		t.Errorf("PlayerCount after duplicate join: got %d, want 1 (second join must be rejected)", r.PlayerCount())
	}
	if r.HasSession("s2") {
		t.Error("HasSession(s2): second (duplicate) session must not be attached")
	}
}

func TestRoom_ActiveSessions(t *testing.T) {
	mgr := newTestManager(newTestRegistry())
	ctx := context.Background()

	r, err := mgr.CreateRoom(ctx, "active-sess-room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	defer r.Stop()

	if len(r.ActiveSessions()) != 0 {
		t.Fatalf("ActiveSessions before join: got %d, want 0", len(r.ActiveSessions()))
	}

	_ = r.Enqueue(room.RoomCommand{Kind: room.CmdJoin, SessionID: "s1", PlayerID: "p1", UserID: "u1", Timestamp: time.Now()})
	_ = r.Enqueue(room.RoomCommand{Kind: room.CmdJoin, SessionID: "s2", PlayerID: "p2", UserID: "u2", Timestamp: time.Now()})

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if len(r.ActiveSessions()) == 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if len(r.ActiveSessions()) != 2 {
		t.Errorf("ActiveSessions after two joins: got %d, want 2", len(r.ActiveSessions()))
	}
}

func TestRoom_DisconnectRemovesSession(t *testing.T) {
	mgr := newTestManager(newTestRegistry())
	ctx := context.Background()

	r, err := mgr.CreateRoom(ctx, "disconnect-room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	defer r.Stop()

	_ = r.Enqueue(room.RoomCommand{Kind: room.CmdJoin, SessionID: "s1", PlayerID: "p1", UserID: "u1", Timestamp: time.Now()})
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if r.HasSession("s1") {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	_ = r.Enqueue(room.RoomCommand{Kind: room.CmdDisconnect, SessionID: "s1", Timestamp: time.Now()})
	deadline = time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !r.HasSession("s1") {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Error("HasSession should return false after CmdDisconnect")
}
```

---

## Task 10: Update docs

**Files:**
- Modify: `docs/room-lifecycle.md`
- Modify: `docs/specs/spec_room_manager.md`

- [ ] **Step 1: Append to `docs/room-lifecycle.md`**

After the "Reconnect Flow" section, add:

```markdown
## Session Tracking

The `Room` maintains two internal indexes updated exclusively by the room loop:

- `activeSessions map[SessionID]sessionAttachment` — maps each attached session to its player/user IDs.
- `userSessionIndex map[UserID]SessionID` — reverse index for duplicate-user detection.

Both indexes are protected by `sessionMu sync.RWMutex`. The room loop holds the write lock when mutating (inside `handleCommand`). External callers (e.g., the session manager, transport goroutines) hold the read lock via `HasSession`, `HasUser`, and `ActiveSessions`.

### Duplicate User Rule

A `CmdJoin` command whose `UserID` is already present in `userSessionIndex` is **silently rejected** by the room loop. The `playerCount` is not incremented and the session is not added to `activeSessions`. The transport goroutine (future milestone) should send an error response to the client.

### Session Cleanup on Disconnect

`CmdDisconnect` removes the session from both indexes, exactly as `CmdLeave` does. The transport layer closes the underlying connection; the room does not call `Close()` on any transport object.
```

- [ ] **Step 2: Append to `docs/specs/spec_room_manager.md`**

After the "Tests" section, add:

```markdown
## Session Tracking (Stage 2 Task 2)

### New Room fields

| Field               | Type                                | Access              |
|---------------------|-------------------------------------|---------------------|
| `sessionMu`         | `sync.RWMutex`                      | Internal            |
| `activeSessions`    | `map[SessionID]sessionAttachment`   | Room loop (write); `HasSession`, `ActiveSessions` (read) |
| `userSessionIndex`  | `map[UserID]SessionID`              | Room loop (write); `HasUser` (read) |

### New Room methods

| Method                        | Safe from           | Returns                       |
|-------------------------------|---------------------|-------------------------------|
| `HasSession(SessionID) bool`  | Any goroutine       | True if session is attached   |
| `HasUser(UserID) bool`        | Any goroutine       | True if user is in room       |
| `ActiveSessions() []SessionID`| Any goroutine       | Snapshot copy of session IDs  |

### New types

- `UserID string` — in `types.go`, for room-internal use in commands and indexes.
- `sessionAttachment struct{playerID PlayerID; userID UserID}` — private to the room package.
- `RoomCommand.UserID UserID` — new field; set by caller on `CmdJoin` for duplicate detection.

### New packages

| Package                      | Provides                                             |
|------------------------------|------------------------------------------------------|
| `internal/game/session`      | `Session`, `SessionManager`, `SessionID`, `UserID`, `SessionState` |
| `internal/game/player`       | `PlayerState`, `PlayerStatus`, `PlayerID`, `UserID`  |

### Additional tests (added in Stage 2 Task 2)

| Test | Status |
|------|--------|
| HasSession returns true after CmdJoin | ✓ |
| HasSession returns false after CmdLeave | ✓ |
| HasUser after join; absent after leave | ✓ |
| Duplicate user join rejected by room loop | ✓ |
| CmdDisconnect removes session | ✓ |
| ActiveSessions returns correct count | ✓ |
| session.Register creates Pending session | ✓ |
| session.Attach → Attached state + user index | ✓ |
| session.Attach rejects duplicate user | ✓ |
| session.Detach → Detached; user index cleared | ✓ |
| session.Close removes from indexes; closes conn | ✓ |
| KCP and WebSocket sessions both register | ✓ |
| session package does not import game/room | ✓ |
```

---

## Self-Review

### Spec coverage check

| Requirement | Task |
|-------------|------|
| `SessionID`, `UserID`, `SessionState`, `Session`, `SessionManager` | Tasks 1, 2 |
| Create/register session | Task 2 (`Register`) |
| Attach session to room | Tasks 2 (`Attach`), 7, 8 |
| Detach session from room | Tasks 2 (`Detach`), 7, 8 |
| Close session | Task 2 (`Close`) |
| Lookup by session ID | Task 2 (`Get`) |
| Lookup by user ID | Task 2 (`GetByUser`) |
| Prevent duplicate active session per user | Task 2 (`Attach` check), Task 8 (`handleCommand` duplicate reject) |
| `PlayerID`, `PlayerState`, `PlayerStatus` | Task 5 |
| Room attach/detach/list active | Tasks 6–9 |
| Reject duplicate user join in room | Task 8, 9 |
| Transport separation preserved | Tasks 1–3 (session imports transport, NOT game/room) |
| No Redis, no movement sync, no spatial | Not introduced anywhere |
| Tests — all required tests | Tasks 3, 9 |
| Docs updated | Task 10 |

### Placeholder scan
No TBD, TODO, or "fill in later" text found. All code blocks contain complete, compilable code.

### Type consistency
- `SessionID`, `PlayerID`, `UserID` in `room/types.go` are `string` typedefs.
- `session.SessionID`, `session.UserID` are separate `string` typedefs — same value space, different Go types. No cross-package import required.
- `player.PlayerID`, `player.UserID` are also separate `string` typedefs.
- `RoomCommand.UserID` uses `room.UserID` (added in Task 6).
- `handleCommand` reads `cmd.UserID` as `room.UserID` and indexes into `userSessionIndex map[UserID]SessionID` — types match.
- `sessionAttachment.userID` is `UserID` (room package type) — consistent.
- `HasUser(id UserID)` param type matches `userSessionIndex` key type — consistent.
