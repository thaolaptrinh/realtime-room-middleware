// Package handler accepts decoded Protocol v1 gameplay packets from transport
// goroutines and enqueues them as room commands.
//
// It sits on the game side of the transport/game boundary. Transport packages
// (kcp, websocket) do not import this package; the handler is wired into
// the PacketReceiver interface at server composition time.
//
// Boundary rules:
//   - handler may import protocol, room, player, and transport (for session metadata only).
//   - handler must not import kcp or websocket packages directly.
//   - handler does not mutate room state; it only enqueues room commands.
//   - handler does not decode raw wire bytes; it receives decoded envelopes
//     or individual message structs.
package handler
