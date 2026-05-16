// Package realtime composes the transport receive path: raw bytes from
// KCP or WSS transport goroutines through protocol decoding to the
// game-side packet handler and room command enqueue.
//
// This is the sole package that imports both transport and game/handler.
// Transport packages (kcp, websocket) never import game packages.
// Protocol never imports game packages. The composition happens here.
//
// Boundary rules:
//   - May import: protocol, transport, game/handler, game/room (for type aliases).
//   - Must not be imported by transport or protocol packages.
//   - Does not mutate room state — it only enqueues commands.
//   - Does not decode gameplay message bodies beyond what protocol provides.
package realtime
