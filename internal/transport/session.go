package transport

// TransportType identifies the realtime transport protocol used by a session.
type TransportType string

const (
	// KCP is the KCP/UDP transport used by Unity native clients.
	KCP TransportType = "kcp"
	// WebSocket is the WSS/WebSocket transport used by Unity WebGL clients.
	WebSocket TransportType = "websocket"
)

// RealtimeSession is the shared session abstraction presented to the room runtime.
// Both KCP and WebSocket transport adapters implement this interface, making
// the room loop transport-agnostic.
//
// Transport adapters must not mutate room state through this interface.
// All state mutation goes through the room command queue.
type RealtimeSession interface {
	// ID returns the unique session identifier assigned by the transport adapter.
	ID() string

	// UserID returns the authenticated player ID. Empty string until session token
	// validation is implemented in a later milestone.
	UserID() string

	// Transport reports which realtime transport protocol this session uses.
	// Used for observability (logging, metrics) only — must not branch game logic.
	Transport() TransportType

	// Send queues a MessagePack Protocol v1 packet for delivery to the client.
	// Safe to call from any goroutine. Returns an error if the session is closed
	// or the send queue is full.
	Send(packet []byte) error

	// Close terminates the session. Idempotent.
	Close() error
}

// PacketReceiver is the shared handler boundary between transport adapters and
// the room runtime. It receives raw inbound packet bytes for decoding and dispatch.
//
// Implementations must not block; all work must be pushed onto a queue.
// Protocol decoding (MessagePack envelope) is the receiver's responsibility.
// Room state mutation must not happen here.
type PacketReceiver interface {
	HandlePacket(sess RealtimeSession, data []byte)
}

// PacketReceiverFunc is an adapter to allow ordinary functions as PacketReceiver.
type PacketReceiverFunc func(sess RealtimeSession, data []byte)

func (f PacketReceiverFunc) HandlePacket(sess RealtimeSession, data []byte) {
	f(sess, data)
}
