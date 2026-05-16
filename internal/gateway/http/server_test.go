package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/thaonguyen/realtime-room-middleware/internal/gateway/resolver"
	"github.com/thaonguyen/realtime-room-middleware/internal/gateway/token"
	"github.com/thaonguyen/realtime-room-middleware/internal/observability"
)

const testWebSocketURL = "ws://localhost:9001/realtime"

func testServer(t *testing.T) *Server {
	t.Helper()
	r := resolver.NewSingleNodeResolver("127.0.0.1:9000", testWebSocketURL, 1)
	logger := observability.InitDefaultLogger("warn")
	return NewServer(ServerConfig{
		Addr:           ":0",
		Resolver:       r,
		TokenGenerator: token.NewGenerator(),
		Logger:         logger,
	})
}

func testServerNoWebSocket(t *testing.T) *Server {
	t.Helper()
	r := resolver.NewSingleNodeResolver("127.0.0.1:9000", "", 1)
	logger := observability.InitDefaultLogger("warn")
	return NewServer(ServerConfig{
		Addr:           ":0",
		Resolver:       r,
		TokenGenerator: token.NewGenerator(),
		Logger:         logger,
	})
}

func doJoin(t *testing.T, srv *Server, req JoinRequest) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(req)
	r := httptest.NewRequest(http.MethodPost, "/join", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, r)
	return w
}

// --- health/readiness ---

func TestHealthz(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %q, want %q", body["status"], "ok")
	}
}

func TestReadyz(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ready" {
		t.Errorf("status = %q, want %q", body["status"], "ready")
	}
}

// --- transport assignment: native/KCP ---

func TestJoinNativeReturnsKCPAssignment(t *testing.T) {
	srv := testServer(t)
	w := doJoin(t, srv, JoinRequest{
		UserID:                "user-1",
		LogicalRoomID:         "expo-room-a",
		ClientProtocolVersion: 1,
		ClientPlatform:        "native",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp JoinResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.Transport != "kcp" {
		t.Errorf("Transport = %q, want %q", resp.Transport, "kcp")
	}
	if resp.KCPAddr != "127.0.0.1:9000" {
		t.Errorf("KCPAddr = %q, want %q", resp.KCPAddr, "127.0.0.1:9000")
	}
	if resp.WebSocketURL != "" {
		t.Errorf("WebSocketURL should be empty for kcp transport, got %q", resp.WebSocketURL)
	}
	if resp.RoomInstanceID == "" {
		t.Error("RoomInstanceID should not be empty")
	}
	if resp.GameNodeAddr != "127.0.0.1:9000" {
		t.Errorf("GameNodeAddr = %q, want %q", resp.GameNodeAddr, "127.0.0.1:9000")
	}
	if resp.SessionToken == "" {
		t.Error("SessionToken should not be empty")
	}
	if resp.ProtocolVersion != 1 {
		t.Errorf("ProtocolVersion = %d, want 1", resp.ProtocolVersion)
	}
	if resp.ExpiresAt.IsZero() {
		t.Error("ExpiresAt should not be zero")
	}
}

func TestJoinNativeExplicitKCPTransport(t *testing.T) {
	srv := testServer(t)
	w := doJoin(t, srv, JoinRequest{
		UserID:                "user-1",
		LogicalRoomID:         "expo-room-a",
		ClientProtocolVersion: 1,
		ClientPlatform:        "native",
		RequestedTransport:    "kcp",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp JoinResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Transport != "kcp" {
		t.Errorf("Transport = %q, want kcp", resp.Transport)
	}
	if resp.KCPAddr == "" {
		t.Error("KCPAddr should not be empty")
	}
}

// --- transport assignment: WebGL/WebSocket ---

func TestJoinWebGLReturnsWebSocketAssignment(t *testing.T) {
	srv := testServer(t)
	w := doJoin(t, srv, JoinRequest{
		UserID:                "user-1",
		LogicalRoomID:         "expo-room-a",
		ClientProtocolVersion: 1,
		ClientPlatform:        "webgl",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp JoinResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.Transport != "websocket" {
		t.Errorf("Transport = %q, want %q", resp.Transport, "websocket")
	}
	if resp.WebSocketURL != testWebSocketURL {
		t.Errorf("WebSocketURL = %q, want %q", resp.WebSocketURL, testWebSocketURL)
	}
	if resp.KCPAddr != "" {
		t.Errorf("KCPAddr should be empty for websocket transport, got %q", resp.KCPAddr)
	}
	if resp.RoomInstanceID == "" {
		t.Error("RoomInstanceID should not be empty")
	}
	if resp.SessionToken == "" {
		t.Error("SessionToken should not be empty")
	}
}

// Native clients may request websocket (spec §8: allowed if server supports it).
func TestJoinNativeCanRequestWebSocket(t *testing.T) {
	srv := testServer(t)
	w := doJoin(t, srv, JoinRequest{
		UserID:                "user-1",
		LogicalRoomID:         "expo-room-a",
		ClientProtocolVersion: 1,
		ClientPlatform:        "native",
		RequestedTransport:    "websocket",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp JoinResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Transport != "websocket" {
		t.Errorf("Transport = %q, want websocket", resp.Transport)
	}
	if resp.WebSocketURL != testWebSocketURL {
		t.Errorf("WebSocketURL = %q, want %q", resp.WebSocketURL, testWebSocketURL)
	}
	if resp.KCPAddr != "" {
		t.Errorf("KCPAddr should be empty for websocket transport, got %q", resp.KCPAddr)
	}
}

// --- platform validation ---

func TestJoinRejectsMissingClientPlatform(t *testing.T) {
	srv := testServer(t)
	w := doJoin(t, srv, JoinRequest{
		UserID:                "user-1",
		LogicalRoomID:         "expo-room-a",
		ClientProtocolVersion: 1,
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	var errResp ErrorResponse
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "missing_client_platform" {
		t.Errorf("code = %q, want %q", errResp.Code, "missing_client_platform")
	}
}

func TestJoinRejectsUnsupportedClientPlatform(t *testing.T) {
	srv := testServer(t)
	w := doJoin(t, srv, JoinRequest{
		UserID:                "user-1",
		LogicalRoomID:         "expo-room-a",
		ClientProtocolVersion: 1,
		ClientPlatform:        "console",
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	var errResp ErrorResponse
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "unsupported_platform" {
		t.Errorf("code = %q, want %q", errResp.Code, "unsupported_platform")
	}
}

// --- transport validation ---

func TestJoinRejectsUnsupportedRequestedTransport(t *testing.T) {
	srv := testServer(t)
	w := doJoin(t, srv, JoinRequest{
		UserID:                "user-1",
		LogicalRoomID:         "expo-room-a",
		ClientProtocolVersion: 1,
		ClientPlatform:        "native",
		RequestedTransport:    "webrtc",
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	var errResp ErrorResponse
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "unsupported_transport" {
		t.Errorf("code = %q, want %q", errResp.Code, "unsupported_transport")
	}
}

func TestJoinRejectsWebGLWithKCPTransport(t *testing.T) {
	srv := testServer(t)
	w := doJoin(t, srv, JoinRequest{
		UserID:                "user-1",
		LogicalRoomID:         "expo-room-a",
		ClientProtocolVersion: 1,
		ClientPlatform:        "webgl",
		RequestedTransport:    "kcp",
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	var errResp ErrorResponse
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "unsupported_transport" {
		t.Errorf("code = %q, want %q", errResp.Code, "unsupported_transport")
	}
}

// WebSocket transport unavailable when server has no websocket_url configured.
func TestJoinRejectsWebSocketWhenNotConfigured(t *testing.T) {
	srv := testServerNoWebSocket(t)
	w := doJoin(t, srv, JoinRequest{
		UserID:                "user-1",
		LogicalRoomID:         "expo-room-a",
		ClientProtocolVersion: 1,
		ClientPlatform:        "webgl",
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	var errResp ErrorResponse
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "unsupported_transport" {
		t.Errorf("code = %q, want %q", errResp.Code, "unsupported_transport")
	}
}

// --- existing request validation (must still pass) ---

func TestJoinRejectsMissingUserID(t *testing.T) {
	srv := testServer(t)
	w := doJoin(t, srv, JoinRequest{
		LogicalRoomID:         "expo-room-a",
		ClientProtocolVersion: 1,
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	var errResp ErrorResponse
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "missing_user_id" {
		t.Errorf("code = %q, want %q", errResp.Code, "missing_user_id")
	}
}

func TestJoinRejectsMissingLogicalRoomID(t *testing.T) {
	srv := testServer(t)
	w := doJoin(t, srv, JoinRequest{
		UserID:                "user-1",
		ClientProtocolVersion: 1,
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	var errResp ErrorResponse
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "missing_logical_room_id" {
		t.Errorf("code = %q, want %q", errResp.Code, "missing_logical_room_id")
	}
}

func TestJoinRejectsUnsupportedProtocolVersion(t *testing.T) {
	srv := testServer(t)
	w := doJoin(t, srv, JoinRequest{
		UserID:                "user-1",
		LogicalRoomID:         "expo-room-a",
		ClientProtocolVersion: 99,
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	var errResp ErrorResponse
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Code != "unsupported_version" {
		t.Errorf("code = %q, want %q", errResp.Code, "unsupported_version")
	}
}

func TestJoinRejectsInvalidBody(t *testing.T) {
	srv := testServer(t)

	req := httptest.NewRequest(http.MethodPost, "/join", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestJoinRejectsVersionZero(t *testing.T) {
	srv := testServer(t)
	w := doJoin(t, srv, JoinRequest{
		UserID:                "user-1",
		LogicalRoomID:         "expo-room-a",
		ClientProtocolVersion: 0,
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}
