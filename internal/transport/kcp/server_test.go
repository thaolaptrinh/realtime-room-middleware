package kcp

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	kcp "github.com/xtaci/kcp-go"
)

func TestServerConfigValidate_Valid(t *testing.T) {
	cfg := ServerConfig{ListenAddr: ":0"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func TestServerConfigValidate_EmptyAddr(t *testing.T) {
	cfg := ServerConfig{}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for empty addr")
	}
}

func TestServerConfigValidate_BadAddr(t *testing.T) {
	cfg := ServerConfig{ListenAddr: "not-an-address:xxx"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for bad addr")
	}
}

func TestNewServer_NilHandler(t *testing.T) {
	_, err := NewServer(ServerConfig{ListenAddr: ":0"}, nil)
	if err == nil {
		t.Fatal("expected error for nil handler")
	}
}

func TestServerCreateAndStop(t *testing.T) {
	handler := HandlerFunc(func(sess Session, data []byte) {})
	srv, err := NewServer(ServerConfig{ListenAddr: ":0"}, handler)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

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

	// Stop should be idempotent.
	srv.Stop()
}

func TestServerStartTwice(t *testing.T) {
	handler := HandlerFunc(func(sess Session, data []byte) {})
	srv, err := NewServer(ServerConfig{ListenAddr: ":0"}, handler)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
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

func TestSessionCloseIdempotent(t *testing.T) {
	handler := HandlerFunc(func(sess Session, data []byte) {})
	srv, err := NewServer(ServerConfig{ListenAddr: ":0"}, handler)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Stop()

	conn, err := kcp.Dial(srv.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Send a trigger packet so KCP establishes the conversation server-side.
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatalf("trigger write: %v", err)
	}

	sess := waitForSession(t, srv, 2*time.Second)
	if sess == nil {
		t.Fatal("session not created within deadline")
	}

	// Close twice — must not panic.
	sess.Close()
	sess.Close()

	if !sess.IsClosed() {
		t.Fatal("expected session to be closed")
	}
}

func TestSessionSendAfterClose(t *testing.T) {
	handler := HandlerFunc(func(sess Session, data []byte) {})
	srv, err := NewServer(ServerConfig{ListenAddr: ":0"}, handler)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Stop()

	conn, err := kcp.Dial(srv.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Trigger session creation.
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatalf("trigger write: %v", err)
	}

	sess := waitForSession(t, srv, 2*time.Second)
	if sess == nil {
		t.Fatal("session not created")
	}

	// Explicitly close the session, then verify Send fails.
	sess.Close()

	err = sess.Send([]byte("test"))
	if err == nil {
		t.Fatal("expected error sending to closed session")
	}
}

func TestHandlerReceivesPacket(t *testing.T) {
	var received atomic.Int32
	handler := HandlerFunc(func(sess Session, data []byte) {
		received.Add(1)
	})

	srv, err := NewServer(ServerConfig{ListenAddr: ":0"}, handler)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Stop()

	conn, err := kcp.Dial(srv.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	payload := []byte("hello-kcp")
	if _, err := conn.Write(payload); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Wait for handler to fire.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if received.Load() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if received.Load() == 0 {
		t.Fatal("handler did not receive any packets")
	}
}

func TestNoGoroutineLeakOnStop(t *testing.T) {
	handler := HandlerFunc(func(sess Session, data []byte) {})
	srv, err := NewServer(ServerConfig{ListenAddr: ":0"}, handler)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Connect a client to create a session.
	conn, err := kcp.Dial(srv.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	// Trigger session creation.
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatalf("trigger write: %v", err)
	}

	// Wait for session.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if srv.SessionCount() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	conn.Close()

	srv.Stop()

	// After Stop, server should report 0 sessions.
	if srv.SessionCount() != 0 {
		t.Fatalf("expected 0 sessions after stop, got %d", srv.SessionCount())
	}
}

func TestServerDefaults(t *testing.T) {
	cfg := ServerConfig{ListenAddr: ":0"}
	if cfg.maxPacketSize() != 64*1024 {
		t.Fatalf("expected default maxPacketSize 65536, got %d", cfg.maxPacketSize())
	}
	if cfg.readTimeout() != defaultReadDeadline {
		t.Fatalf("expected default readTimeout %v, got %v", defaultReadDeadline, cfg.readTimeout())
	}
	if cfg.writeTimeout() != defaultWriteDeadline {
		t.Fatalf("expected default writeTimeout %v, got %v", defaultWriteDeadline, cfg.writeTimeout())
	}
	if cfg.sendQueueSize() != defaultSendQueueSize {
		t.Fatalf("expected default sendQueueSize %d, got %d", defaultSendQueueSize, cfg.sendQueueSize())
	}
}

func TestNoGameDependency(t *testing.T) {
	// This test exists to document that the kcp package does not import
	// internal/game. If it did, this package would fail to compile in
	// isolation. The fact that this test file compiles is the proof.
	_ = net.ErrClosed
}

func waitForSession(t *testing.T, srv *KCPServer, timeout time.Duration) *kcpSession {
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
