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

func testServer(t *testing.T) *Server {
	t.Helper()
	r := resolver.NewSingleNodeResolver("127.0.0.1:9000", 1)
	logger := observability.InitDefaultLogger("warn")
	return NewServer(ServerConfig{
		Addr:           ":0",
		Resolver:       r,
		TokenGenerator: token.NewGenerator(),
		Logger:         logger,
	})
}

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

func TestJoinReturnsAssignment(t *testing.T) {
	srv := testServer(t)

	body, _ := json.Marshal(JoinRequest{
		UserID:                "user-1",
		LogicalRoomID:         "expo-room-a",
		ClientProtocolVersion: 1,
	})
	req := httptest.NewRequest(http.MethodPost, "/join", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp JoinResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.RoomInstanceID == "" {
		t.Error("RoomInstanceID should not be empty")
	}
	if resp.GameNodeAddr != "127.0.0.1:9000" {
		t.Errorf("GameNodeAddr = %q, want %q", resp.GameNodeAddr, "127.0.0.1:9000")
	}
	if resp.KCPAddr != "127.0.0.1:9000" {
		t.Errorf("KCPAddr = %q, want %q", resp.KCPAddr, "127.0.0.1:9000")
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

func TestJoinRejectsMissingUserID(t *testing.T) {
	srv := testServer(t)

	body, _ := json.Marshal(JoinRequest{
		LogicalRoomID:         "expo-room-a",
		ClientProtocolVersion: 1,
	})
	req := httptest.NewRequest(http.MethodPost, "/join", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

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

	body, _ := json.Marshal(JoinRequest{
		UserID:                "user-1",
		ClientProtocolVersion: 1,
	})
	req := httptest.NewRequest(http.MethodPost, "/join", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

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

	body, _ := json.Marshal(JoinRequest{
		UserID:                "user-1",
		LogicalRoomID:         "expo-room-a",
		ClientProtocolVersion: 99,
	})
	req := httptest.NewRequest(http.MethodPost, "/join", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

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

	body, _ := json.Marshal(JoinRequest{
		UserID:                "user-1",
		LogicalRoomID:         "expo-room-a",
		ClientProtocolVersion: 0,
	})
	req := httptest.NewRequest(http.MethodPost, "/join", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}
