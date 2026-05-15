package kcp

import "net"

// Session represents a connected KCP client session.
type Session interface {
	// ID returns the unique session identifier.
	ID() string

	// RemoteAddr returns the remote network address.
	RemoteAddr() net.Addr

	// Send queues data for delivery to the client.
	// It is safe to call from any goroutine.
	Send(data []byte) error

	// Close terminates the session. Subsequent calls are no-ops.
	Close() error

	// IsClosed reports whether the session has been closed.
	IsClosed() bool
}

// PacketHandler receives decoded raw payloads from network sessions.
// Implementations must not block; push work onto a queue instead.
type PacketHandler interface {
	// HandlePacket is called for each inbound packet.
	// The handler receives the session that sent the data and the raw payload bytes.
	// Protocol decoding is the handler's responsibility.
	HandlePacket(sess Session, data []byte)
}

// HandlerFunc is an adapter to allow ordinary functions as PacketHandler.
type HandlerFunc func(sess Session, data []byte)

func (f HandlerFunc) HandlePacket(sess Session, data []byte) {
	f(sess, data)
}
