package http

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/thaonguyen/realtime-room-middleware/internal/gateway/resolver"
)

const (
	platformNative = "native"
	platformWebGL  = "webgl"
	transportKCP   = "kcp"
	transportWSS   = "websocket"
)

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

func (s *Server) handleJoin(w http.ResponseWriter, r *http.Request) {
	var req JoinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "bad_request")
		return
	}

	if req.UserID == "" {
		writeError(w, http.StatusBadRequest, "user_id is required", "missing_user_id")
		return
	}
	if req.LogicalRoomID == "" {
		writeError(w, http.StatusBadRequest, "logical_room_id is required", "missing_logical_room_id")
		return
	}

	minV, maxV := supportedVersionRange()
	if req.ClientProtocolVersion < minV || req.ClientProtocolVersion > maxV {
		writeError(w, http.StatusBadRequest, "unsupported protocol version", "unsupported_version")
		return
	}

	if req.ClientPlatform == "" {
		writeError(w, http.StatusBadRequest, "client_platform is required", "missing_client_platform")
		return
	}
	if req.ClientPlatform != platformNative && req.ClientPlatform != platformWebGL {
		writeError(w, http.StatusBadRequest, "unsupported client platform", "unsupported_platform")
		return
	}

	transport, err := resolveTransport(req.ClientPlatform, req.RequestedTransport)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "unsupported_transport")
		return
	}

	assignment, err := s.resolver.ResolveRoom(r.Context(), req.LogicalRoomID, resolver.AssignOptions{
		UserID: req.UserID,
	})
	if err != nil {
		s.logger.Error("resolver failed",
			slog.String("logical_room_id", req.LogicalRoomID),
			slog.String("error", err.Error()),
		)
		writeError(w, http.StatusInternalServerError, "internal resolver error", "resolver_error")
		return
	}

	if transport == transportWSS && assignment.WebSocketURL == "" {
		writeError(w, http.StatusBadRequest, "websocket transport not available", "unsupported_transport")
		return
	}

	tok, err := s.tokenGen.Generate(req.UserID, assignment.RoomInstanceID, assignment.ExpiresAt)
	if err != nil {
		s.logger.Error("token generation failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal token error", "token_error")
		return
	}

	resp := JoinResponse{
		RoomInstanceID:  assignment.RoomInstanceID,
		GameNodeAddr:    assignment.GameNodeAddr,
		ProtocolVersion: assignment.ProtocolVersion,
		SessionToken:    tok,
		Transport:       transport,
		ExpiresAt:       assignment.ExpiresAt,
	}
	switch transport {
	case transportKCP:
		resp.KCPAddr = assignment.KCPAddr
	case transportWSS:
		resp.WebSocketURL = assignment.WebSocketURL
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// resolveTransport determines the assigned transport from client_platform and optional requested_transport.
// WebGL clients may not request KCP (browsers cannot open UDP sockets).
// Native clients may request websocket if the server has it configured.
func resolveTransport(platform, requested string) (string, error) {
	if requested != "" {
		if requested != transportKCP && requested != transportWSS {
			return "", fmt.Errorf("unsupported requested_transport %q: must be kcp or websocket", requested)
		}
		if platform == platformWebGL && requested == transportKCP {
			return "", fmt.Errorf("webgl clients cannot use kcp transport")
		}
		return requested, nil
	}
	switch platform {
	case platformNative:
		return transportKCP, nil
	case platformWebGL:
		return transportWSS, nil
	}
	return "", fmt.Errorf("unknown platform %q", platform)
}

func writeError(w http.ResponseWriter, status int, msg, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErrorResponse{Error: msg, Code: code})
}
