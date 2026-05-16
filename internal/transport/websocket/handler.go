package websocket

import "net"

// Session represents a connected WebSocket client session.
type Session interface {
	// ID returns the unique session identifier.
	ID() string

	// RemoteAddr returns the remote network address.
	RemoteAddr() net.Addr

	// Send queues data for delivery to the client as a binary WebSocket frame.
	// It is safe to call from any goroutine.
	Send(data []byte) error

	// Close terminates the session. Subsequent calls are no-ops.
	Close() error

	// IsClosed reports whether the session has been closed.
	IsClosed() bool
}

// PacketHandler receives raw binary payloads from WebSocket sessions.
// Implementations must not block; push work onto a queue instead.
// All payloads are MessagePack Protocol v1 bytes carried in binary WebSocket frames.
// JSON must not be used on the realtime data plane.
type PacketHandler interface {
	// HandlePacket is called for each inbound binary frame payload.
	// Protocol decoding is the handler's responsibility.
	HandlePacket(sess Session, data []byte)
}

// HandlerFunc is an adapter to allow ordinary functions as PacketHandler.
type HandlerFunc func(sess Session, data []byte)

func (f HandlerFunc) HandlePacket(sess Session, data []byte) {
	f(sess, data)
}
