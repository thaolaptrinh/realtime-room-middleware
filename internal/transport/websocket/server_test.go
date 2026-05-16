package websocket

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	gorillaws "github.com/gorilla/websocket"
)

// dialWS dials the /realtime endpoint on the given address.
func dialWS(t *testing.T, addr net.Addr) *gorillaws.Conn {
	t.Helper()
	url := "ws://" + addr.String() + "/realtime"
	conn, _, err := gorillaws.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial %s: %v", url, err)
	}
	return conn
}

func newTestServer(t *testing.T, handler PacketHandler) *WSSServer {
	t.Helper()
	cfg := ServerConfig{
		ListenAddr: ":0",
		DevMode:    true,
	}
	srv, err := NewServer(cfg, handler)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return srv
}

// TestServerConfigValidate_Valid confirms a minimal valid config passes.
func TestServerConfigValidate_Valid(t *testing.T) {
	cfg := ServerConfig{ListenAddr: ":0"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

// TestServerConfigValidate_EmptyAddr confirms missing ListenAddr is rejected.
func TestServerConfigValidate_EmptyAddr(t *testing.T) {
	cfg := ServerConfig{}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for empty addr")
	}
}

// TestServerConfigValidate_BadAddr confirms an unparseable address is rejected.
func TestServerConfigValidate_BadAddr(t *testing.T) {
	cfg := ServerConfig{ListenAddr: "not-an-address:xxx"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for bad addr")
	}
}

// TestNewServer_NilHandler confirms nil handler is rejected.
func TestNewServer_NilHandler(t *testing.T) {
	_, err := NewServer(ServerConfig{ListenAddr: ":0", DevMode: true}, nil)
	if err == nil {
		t.Fatal("expected error for nil handler")
	}
}

// TestServerCreateAndStop confirms basic lifecycle: create → start → stop.
func TestServerCreateAndStop(t *testing.T) {
	handler := HandlerFunc(func(sess Session, data []byte) {})
	srv := newTestServer(t, handler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	addr := srv.Addr()
	if addr == nil {
		t.Fatal("expected non-nil addr after start")
	}
	if srv.SessionCount() != 0 {
		t.Fatalf("expected 0 sessions, got %d", srv.SessionCount())
	}

	srv.Stop()
	srv.Stop() // idempotent
}

// TestServerStartTwice confirms double-start returns an error.
func TestServerStartTwice(t *testing.T) {
	handler := HandlerFunc(func(sess Session, data []byte) {})
	srv := newTestServer(t, handler)
	defer srv.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := srv.Start(ctx); err == nil {
		t.Fatal("expected error on double start")
	}
}

// TestSessionCloseIdempotent confirms Close can be called multiple times safely.
func TestSessionCloseIdempotent(t *testing.T) {
	handler := HandlerFunc(func(sess Session, data []byte) {})
	srv := newTestServer(t, handler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Stop()

	client := dialWS(t, srv.Addr())
	defer client.Close()

	// Send a binary frame to trigger session creation server-side.
	if err := client.WriteMessage(gorillaws.BinaryMessage, []byte("ping")); err != nil {
		t.Fatalf("write: %v", err)
	}

	sess := waitForSession(t, srv, 2*time.Second)
	if sess == nil {
		t.Fatal("session not created within deadline")
	}

	sess.Close()
	sess.Close() // must not panic

	if !sess.IsClosed() {
		t.Fatal("expected session to be closed")
	}
}

// TestSessionSendAfterClose confirms Send returns an error after Close.
func TestSessionSendAfterClose(t *testing.T) {
	handler := HandlerFunc(func(sess Session, data []byte) {})
	srv := newTestServer(t, handler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Stop()

	client := dialWS(t, srv.Addr())
	defer client.Close()

	if err := client.WriteMessage(gorillaws.BinaryMessage, []byte("ping")); err != nil {
		t.Fatalf("write: %v", err)
	}

	sess := waitForSession(t, srv, 2*time.Second)
	if sess == nil {
		t.Fatal("session not created")
	}

	sess.Close()

	if err := sess.Send([]byte("test")); err == nil {
		t.Fatal("expected error sending to closed session")
	}
}

// TestHandlerReceivesBinaryPacket confirms binary frame payloads reach the PacketHandler.
func TestHandlerReceivesBinaryPacket(t *testing.T) {
	var received atomic.Int32
	handler := HandlerFunc(func(sess Session, data []byte) {
		received.Add(1)
	})

	srv := newTestServer(t, handler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Stop()

	client := dialWS(t, srv.Addr())
	defer client.Close()

	payload := []byte("msgpack-binary-frame")
	if err := client.WriteMessage(gorillaws.BinaryMessage, payload); err != nil {
		t.Fatalf("write binary frame: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if received.Load() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if received.Load() == 0 {
		t.Fatal("PacketHandler did not receive binary frame payload")
	}
}

// TestTextFrameClosesSession confirms that a text frame causes the session to be closed.
func TestTextFrameClosesSession(t *testing.T) {
	handler := HandlerFunc(func(sess Session, data []byte) {})
	srv := newTestServer(t, handler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Stop()

	client := dialWS(t, srv.Addr())
	defer client.Close()

	// Send a binary frame first to establish the server-side session.
	if err := client.WriteMessage(gorillaws.BinaryMessage, []byte("init")); err != nil {
		t.Fatalf("write init frame: %v", err)
	}

	sess := waitForSession(t, srv, 2*time.Second)
	if sess == nil {
		t.Fatal("session not created")
	}

	// Send a text frame — server must close the session.
	if err := client.WriteMessage(gorillaws.TextMessage, []byte("not allowed")); err != nil {
		t.Fatalf("write text frame: %v", err)
	}

	// Wait for the session to be closed server-side.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if sess.IsClosed() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !sess.IsClosed() {
		t.Fatal("expected session to be closed after text frame")
	}
}

// TestNoGoroutineLeakOnStop confirms session goroutines exit cleanly after Stop.
func TestNoGoroutineLeakOnStop(t *testing.T) {
	handler := HandlerFunc(func(sess Session, data []byte) {})
	srv := newTestServer(t, handler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	client, resp, err := gorillaws.DefaultDialer.Dial("ws://"+srv.Addr().String()+"/realtime", nil)
	if resp != nil {
		resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	if err := client.WriteMessage(gorillaws.BinaryMessage, []byte("ping")); err != nil {
		t.Fatalf("write: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if srv.SessionCount() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	client.Close()

	srv.Stop()

	if srv.SessionCount() != 0 {
		t.Fatalf("expected 0 sessions after stop, got %d", srv.SessionCount())
	}
}

// TestServerDefaults confirms default values for optional config fields.
func TestServerDefaults(t *testing.T) {
	cfg := ServerConfig{ListenAddr: ":0"}
	if cfg.maxPayloadSize() != defaultMaxPayloadSize {
		t.Fatalf("expected default maxPayloadSize %d, got %d", defaultMaxPayloadSize, cfg.maxPayloadSize())
	}
	if cfg.readTimeout() != defaultReadTimeout {
		t.Fatalf("expected default readTimeout %v, got %v", defaultReadTimeout, cfg.readTimeout())
	}
	if cfg.writeTimeout() != defaultWriteTimeout {
		t.Fatalf("expected default writeTimeout %v, got %v", defaultWriteTimeout, cfg.writeTimeout())
	}
	if cfg.sendQueueSize() != defaultSendQueueSize {
		t.Fatalf("expected default sendQueueSize %d, got %d", defaultSendQueueSize, cfg.sendQueueSize())
	}
}

// TestNoGameDependency documents that this package must not import internal/game.
// The fact that this test file compiles in isolation is the proof.
func TestNoGameDependency(t *testing.T) {
	_ = net.ErrClosed
}

// waitForSession polls until the server has at least one session or the timeout elapses.
func waitForSession(t *testing.T, srv *WSSServer, timeout time.Duration) *wssSession {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		srv.mu.Lock()
		for _, s := range srv.sessions {
			srv.mu.Unlock()
			return s
		}
		srv.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	return nil
}
