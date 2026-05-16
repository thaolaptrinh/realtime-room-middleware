package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	gatewayhttp "github.com/thaonguyen/realtime-room-middleware/internal/gateway/http"
	"github.com/thaonguyen/realtime-room-middleware/internal/gateway/resolver"
	"github.com/thaonguyen/realtime-room-middleware/internal/gateway/token"
	"github.com/thaonguyen/realtime-room-middleware/internal/observability"
)

func newTestGateway(t *testing.T) *gatewayhttp.Server {
	t.Helper()
	r := resolver.NewSingleNodeResolver("127.0.0.1:9000", "ws://localhost:9001/realtime", 1)
	logger := observability.InitDefaultLogger("warn")
	return gatewayhttp.NewServer(gatewayhttp.ServerConfig{
		Addr:           ":0",
		Resolver:       r,
		TokenGenerator: token.NewGenerator(),
		Logger:         logger,
	})
}

func TestGatewaySmoke(t *testing.T) {
	srv := newTestGateway(t)
	go srv.Start()
	defer srv.Shutdown(context.Background())
}

func TestGatewaySmokeHealthEndpoint(t *testing.T) {
	srv := newTestGateway(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("healthz status = %d, want %d", w.Code, http.StatusOK)
	}

	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("healthz body = %v, want status=ok", body)
	}
}

func TestGatewaySmokeJoinEndpoint(t *testing.T) {
	srv := newTestGateway(t)

	reqBody := gatewayhttp.JoinRequest{
		UserID:                "smoke-user",
		LogicalRoomID:         "smoke-room",
		ClientProtocolVersion: 1,
		ClientPlatform:        "native",
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/join", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("join status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var joinResp gatewayhttp.JoinResponse
	json.NewDecoder(w.Body).Decode(&joinResp)

	if joinResp.SessionToken == "" {
		t.Error("join response should contain a session token")
	}
	if joinResp.GameNodeAddr != "127.0.0.1:9000" {
		t.Errorf("GameNodeAddr = %q, want 127.0.0.1:9000", joinResp.GameNodeAddr)
	}
}
