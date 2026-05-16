package http

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/thaonguyen/realtime-room-middleware/internal/gateway/resolver"
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

	tok, err := s.tokenGen.Generate(req.UserID, assignment.RoomInstanceID, assignment.ExpiresAt)
	if err != nil {
		s.logger.Error("token generation failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal token error", "token_error")
		return
	}

	resp := JoinResponse{
		RoomInstanceID:  assignment.RoomInstanceID,
		GameNodeAddr:    assignment.GameNodeAddr,
		KCPAddr:         assignment.KCPAddr,
		SessionToken:    tok,
		ProtocolVersion: assignment.ProtocolVersion,
		ExpiresAt:       assignment.ExpiresAt,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

func writeError(w http.ResponseWriter, status int, msg, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErrorResponse{Error: msg, Code: code})
}
