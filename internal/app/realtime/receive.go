package realtime

import (
	"fmt"
	"log/slog"

	"github.com/thaonguyen/realtime-room-middleware/internal/game/handler"
	"github.com/thaonguyen/realtime-room-middleware/internal/game/room"
	"github.com/thaonguyen/realtime-room-middleware/internal/protocol"
	"github.com/thaonguyen/realtime-room-middleware/internal/transport"
)

// SessionResolver resolves a transport session ID to its game-level identity.
// Returns (playerID, userID). Both are empty if the session has not yet
// joined a room (e.g., between Hello and JoinRoom).
type SessionResolver func(sessionID string) (playerID string, userID string)

// SessionPacketRouter maps transport sessions to game handler context values.
// It wraps a SessionResolver and provides typed context construction.
type SessionPacketRouter struct {
	resolve SessionResolver
}

// NewSessionPacketRouter creates a router with the given session resolver function.
func NewSessionPacketRouter(resolve SessionResolver) *SessionPacketRouter {
	return &SessionPacketRouter{resolve: resolve}
}

// ResolveContext builds a handler.PacketContext from a transport.RealtimeSession.
func (r *SessionPacketRouter) ResolveContext(sess transport.RealtimeSession) handler.PacketContext {
	playerID, userID := r.resolve(sess.ID())
	return handler.PacketContext{
		SessionID: sess.ID(),
		PlayerID:  room.PlayerID(playerID),
		UserID:    room.UserID(userID),
	}
}

// PacketProcessor processes a single raw MessagePack packet through protocol
// decode and game handler dispatch. It does not mutate room state.
//
// Create with NewPacketProcessor.
type PacketProcessor struct {
	handler  *handler.RealtimePacketHandler
	router   *SessionPacketRouter
	logger   *slog.Logger
}

// NewPacketProcessor creates a processor that decodes envelopes and dispatches
// gameplay messages to the handler via the session router.
func NewPacketProcessor(h *handler.RealtimePacketHandler, router *SessionPacketRouter, logger *slog.Logger) *PacketProcessor {
	if logger == nil {
		logger = slog.Default()
	}
	return &PacketProcessor{
		handler: h,
		router:  router,
		logger:  logger,
	}
}

// Process decodes a raw MessagePack packet and dispatches it to the game handler.
// Returns an error for invalid bytes, unsupported message types, or handler errors.
// Does not panic on any input.
func (p *PacketProcessor) Process(sess transport.RealtimeSession, data []byte) error {
	env, err := protocol.DecodeEnvelope(data)
	if err != nil {
		return fmt.Errorf("decode envelope from session %s: %w", sess.ID(), err)
	}

	ctx := p.router.ResolveContext(sess)

	result := p.handler.HandleEnvelope(ctx, env)
	if result.Error != nil {
		return fmt.Errorf("handle %s from session %s: %w", env.Type, sess.ID(), result.Error)
	}
	return nil
}

// ReceiveLoopAdapter implements transport.PacketReceiver. It receives raw
// inbound packets from transport goroutines (KCP or WSS), processes them
// through PacketProcessor, and logs errors.
//
// Errors are logged but not propagated because transport.PacketReceiver.HandlePacket
// has no return value. The adapter never panics.
type ReceiveLoopAdapter struct {
	processor *PacketProcessor
	logger    *slog.Logger
}

// NewReceiveLoopAdapter creates an adapter that delegates packet processing
// to the given PacketProcessor.
func NewReceiveLoopAdapter(processor *PacketProcessor, logger *slog.Logger) *ReceiveLoopAdapter {
	if logger == nil {
		logger = slog.Default()
	}
	return &ReceiveLoopAdapter{
		processor: processor,
		logger:    logger,
	}
}

// HandlePacket implements transport.PacketReceiver.
func (a *ReceiveLoopAdapter) HandlePacket(sess transport.RealtimeSession, data []byte) {
	if err := a.processor.Process(sess, data); err != nil {
		a.logger.Warn("packet processing error",
			slog.String("session", sess.ID()),
			slog.String("transport", string(sess.Transport())),
			slog.String("err", err.Error()),
		)
	}
}

// Compile-time check: ReceiveLoopAdapter satisfies transport.PacketReceiver.
var _ transport.PacketReceiver = (*ReceiveLoopAdapter)(nil)
