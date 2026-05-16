package handler

import (
	"fmt"
	"time"

	"github.com/thaonguyen/realtime-room-middleware/internal/game/player"
	"github.com/thaonguyen/realtime-room-middleware/internal/game/room"
	"github.com/thaonguyen/realtime-room-middleware/internal/protocol"
)

// PacketContext carries session and transport metadata for a single inbound packet.
// The handler uses this to resolve the target room and player identity without
// touching transport-specific types.
type PacketContext struct {
	SessionID string
	PlayerID  room.PlayerID
	UserID    room.UserID
}

// HandlePacketResult describes the outcome of processing a single inbound packet.
type HandlePacketResult struct {
	MessageType protocol.MessageType
	Handled     bool
	Error       error
}

// String returns a human-readable summary of the result.
func (r HandlePacketResult) String() string {
	if r.Error != nil {
		return fmt.Sprintf("%s: error: %s", r.MessageType, r.Error)
	}
	if !r.Handled {
		return fmt.Sprintf("%s: not handled", r.MessageType)
	}
	return fmt.Sprintf("%s: handled", r.MessageType)
}

// RoomEnqueuer is the narrow interface the handler uses to submit commands to a room.
// The Room type satisfies this interface; the handler never imports room state directly.
type RoomEnqueuer interface {
	Enqueue(cmd room.RoomCommand) error
}

// SessionRoomResolver maps a session ID to the room it is attached to.
// Returns (nil, nil) if the session is not attached to any room.
type SessionRoomResolver func(sessionID string) (RoomEnqueuer, error)

// RealtimePacketHandler accepts decoded Protocol v1 gameplay messages and
// enqueues them as room commands. It does not mutate room state.
//
// Create with NewRealtimePacketHandler.
type RealtimePacketHandler struct {
	resolveRoom SessionRoomResolver
}

// NewRealtimePacketHandler creates a handler that resolves sessions to rooms
// using the provided resolver function.
func NewRealtimePacketHandler(resolver SessionRoomResolver) *RealtimePacketHandler {
	return &RealtimePacketHandler{resolveRoom: resolver}
}

// HandleEnvelope processes a decoded protocol envelope and enqueues the
// appropriate room command. Returns a result describing what happened.
//
// Supported message types:
//   - PlayerInput (type 4): enqueues CmdPlayerInput with player.PlayerInput payload.
//   - PlayerTransformUpdate (type 6): enqueues CmdUpdatePlayerTransform with player.PlayerTransform payload.
//
// All other message types return Handled=false with an appropriate error.
// The handler does not panic on any input.
func (h *RealtimePacketHandler) HandleEnvelope(ctx PacketContext, env *protocol.Envelope) HandlePacketResult {
	result := HandlePacketResult{
		MessageType: env.Type,
	}

	if !env.Type.IsClientToServer() {
		result.Error = fmt.Errorf("message type %s is not client-to-server", env.Type)
		return result
	}

	switch env.Type {
	case protocol.TypePlayerInput:
		var msg protocol.PlayerInput
		if err := protocol.DecodeMessage(env.Body, &msg); err != nil {
			result.Error = fmt.Errorf("decode PlayerInput: %w", err)
			return result
		}
		return h.handlePlayerInput(ctx, env, &msg)

	case protocol.TypePlayerTransformUpdate:
		var msg protocol.PlayerTransformUpdate
		if err := protocol.DecodeMessage(env.Body, &msg); err != nil {
			result.Error = fmt.Errorf("decode PlayerTransformUpdate: %w", err)
			return result
		}
		return h.handlePlayerTransformUpdate(ctx, env, &msg)

	default:
		result.Error = fmt.Errorf("unsupported gameplay message type: %s", env.Type)
		return result
	}
}

// handlePlayerInput maps a PlayerInput wire message to a room CmdPlayerInput command.
// The room loop validates the domain PlayerInput (transform, timestamp) at command processing time.
func (h *RealtimePacketHandler) handlePlayerInput(ctx PacketContext, env *protocol.Envelope, msg *protocol.PlayerInput) HandlePacketResult {
	result := HandlePacketResult{
		MessageType: protocol.TypePlayerInput,
	}

	if ctx.PlayerID == "" {
		result.Error = fmt.Errorf("player ID is required for PlayerInput")
		return result
	}

	gameInput := player.PlayerInput{
		Seq: msg.Seq,
		Transform: player.PlayerTransform{
			Position: player.Vector3{X: msg.MoveX, Z: msg.MoveZ},
			Rotation: player.Quaternion{X: 0, Y: msg.Yaw, Z: 0, W: 1},
		},
		Timestamp: time.Now().UnixMilli(),
	}

	cmd := room.RoomCommand{
		Kind:      room.CmdPlayerInput,
		SessionID: room.SessionID(ctx.SessionID),
		PlayerID:  ctx.PlayerID,
		UserID:    ctx.UserID,
		Payload:   gameInput,
		Timestamp: time.Now(),
	}

	if err := h.enqueue(ctx, cmd); err != nil {
		result.Error = fmt.Errorf("enqueue PlayerInput: %w", err)
		return result
	}

	result.Handled = true
	return result
}

// handlePlayerTransformUpdate maps a PlayerTransformUpdate wire message to a
// room CmdUpdatePlayerTransform command.
func (h *RealtimePacketHandler) handlePlayerTransformUpdate(ctx PacketContext, env *protocol.Envelope, msg *protocol.PlayerTransformUpdate) HandlePacketResult {
	result := HandlePacketResult{
		MessageType: protocol.TypePlayerTransformUpdate,
	}

	if ctx.PlayerID == "" {
		result.Error = fmt.Errorf("player ID is required for PlayerTransformUpdate")
		return result
	}

	gameTransform := player.PlayerTransform{
		Position: player.Vector3{X: msg.X, Z: msg.Z},
		Rotation: player.Quaternion{X: 0, Y: msg.Yaw, Z: 0, W: 1},
	}

	cmd := room.RoomCommand{
		Kind:      room.CmdUpdatePlayerTransform,
		SessionID: room.SessionID(ctx.SessionID),
		PlayerID:  ctx.PlayerID,
		UserID:    ctx.UserID,
		Payload:   gameTransform,
		Timestamp: time.Now(),
	}

	if err := h.enqueue(ctx, cmd); err != nil {
		result.Error = fmt.Errorf("enqueue PlayerTransformUpdate: %w", err)
		return result
	}

	result.Handled = true
	return result
}

// enqueue resolves the session's room and submits the command.
func (h *RealtimePacketHandler) enqueue(ctx PacketContext, cmd room.RoomCommand) error {
	r, err := h.resolveRoom(ctx.SessionID)
	if err != nil {
		return fmt.Errorf("resolve room for session %s: %w", ctx.SessionID, err)
	}
	if r == nil {
		return fmt.Errorf("session %s is not attached to a room", ctx.SessionID)
	}
	return r.Enqueue(cmd)
}
