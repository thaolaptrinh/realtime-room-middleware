package session_test

import (
	"testing"

	"github.com/thaonguyen/realtime-room-middleware/internal/game/session"
	"github.com/thaonguyen/realtime-room-middleware/internal/transport"
)

// ---- mock transport.RealtimeSession -----------------------------------------

type mockConn struct {
	id        string
	closed    bool
	closeErr  error
	transport transport.TransportType
}

func (m *mockConn) ID() string                         { return m.id }
func (m *mockConn) UserID() string                     { return "" }
func (m *mockConn) Transport() transport.TransportType { return m.transport }
func (m *mockConn) Send(_ []byte) error                { return nil }
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
	if sess.UserID() != "" {
		t.Errorf("UserID before Attach: got %q, want empty", sess.UserID())
	}
	if sess.RoomInstanceID() != "" {
		t.Errorf("RoomInstanceID before Attach: got %q, want empty", sess.RoomInstanceID())
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

	// GetByUser should no longer find the session after detach.
	_, ok := mgr.GetByUser("user-42")
	if ok {
		t.Error("GetByUser should return not-found after Detach")
	}
}

func TestDetach_Idempotent(t *testing.T) {
	mgr := session.NewSessionManager()
	mgr.Register("sess-1", transport.KCP, newMockConn("sess-1", transport.KCP))
	mgr.Attach("sess-1", "user-1", "room-0001")
	mgr.Detach("sess-1")

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

	_, ok := mgr.Get("sess-1")
	if ok {
		t.Error("Get should return not-found after Close")
	}
	_, ok = mgr.GetByUser("user-42")
	if ok {
		t.Error("GetByUser should return not-found after Close")
	}
}

func TestClose_DoubleClose_ReturnsError(t *testing.T) {
	mgr := session.NewSessionManager()
	conn := newMockConn("sess-1", transport.KCP)
	mgr.Register("sess-1", transport.KCP, conn)
	mgr.Close("sess-1")

	// Second close: session removed from byID, so returns "not found".
	err := mgr.Close("sess-1")
	if err == nil {
		t.Fatal("expected error on second Close after session removed")
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

// ---- SessionState.String ----------------------------------------------------

func TestSessionState_String(t *testing.T) {
	cases := []struct {
		state session.SessionState
		want  string
	}{
		{session.SessionStatePending, "pending"},
		{session.SessionStateAttached, "attached"},
		{session.SessionStateDetached, "detached"},
		{session.SessionStateClosed, "closed"},
		{session.SessionState(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.state.String(); got != tc.want {
			t.Errorf("SessionState(%d).String(): got %q, want %q", tc.state, got, tc.want)
		}
	}
}
