# Spec: KCP Transport Layer

## Scope

Milestone 1 deliverable. KCP over UDP server lifecycle, session management,
packet I/O, configurable timeouts.

This spec covers the **Unity native transport only**. KCP runs over UDP and
is not available in Unity WebGL or browser environments. The sibling WebSocket
transport for Unity WebGL is documented in
`docs/specs/spec_websocket_transport.md`.

This is a **transport skeleton only**. Room runtime integration, protocol
decoding in the handler, and business logic are intentionally deferred to later
milestones.

## Key Decisions

- KCP/UDP is the Unity **native** transport. It is not used by Unity WebGL or
  browser clients. WebGL clients use the WSS transport (see
  `docs/specs/spec_websocket_transport.md`).
- KCP transport is required for Phase 1 native client production. It is not
  replaced by or deprecated by the WebSocket transport.
- WebSocket transport is a sibling transport, not an upgrade from KCP. Both
  are required. Neither replaces the other.
- Listen on configurable UDP port (default `:9000`).
- One KCP session per connected client.
- Session has read/write deadlines.
- Network goroutines push decoded/validated input upward through `PacketHandler`;
  they do **not** mutate room state.
- The transport layer is payload-agnostic — it passes raw bytes to the handler.
  Protocol decoding is the handler's responsibility.
- No dependency on `internal/game`. Clean interface boundary only.

## Interfaces

### `Session`

Represents a connected KCP client. Provides `ID()`, `RemoteAddr()`, `Send()`,
`Close()`, and `IsClosed()`.

### `PacketHandler`

Receives raw inbound payloads from sessions. Must not block; push work onto a
queue. `HandlerFunc` adapter allows ordinary functions.

### `Server`

`KCPServer` manages the listener, accept loop, and session registry. Created
via `NewServer(cfg, handler)`. Lifecycle: `Start(ctx)` → accept packets →
`Stop()`.

## Files

- `internal/transport/kcp/handler.go` — `Session`, `PacketHandler`, `HandlerFunc`
- `internal/transport/kcp/server.go` — `ServerConfig`, `KCPServer`, lifecycle
- `internal/transport/kcp/session.go` — `kcpSession` (internal), read/write loops
- `internal/transport/kcp/server_test.go` — config validation, lifecycle, session tests

## Configuration

`ServerConfig` fields:

| Field           | Default   | Description                        |
|-----------------|-----------|------------------------------------|
| `ListenAddr`    | required  | UDP address (e.g. `:9000`)         |
| `MaxPacketSize` | 64 KB     | Max inbound packet size            |
| `ReadTimeout`   | 10s       | Per-read deadline                  |
| `WriteTimeout`  | 5s        | Per-write deadline                 |
| `SendQueueSize` | 256       | Buffered outbound channel per session |
| `Logger`        | slog.Default | Structured logger               |

## Tests

- Server config validation (valid, empty addr, bad addr)
- Nil handler rejection
- Server create/close lifecycle
- Double-start rejection
- Session close idempotency
- Send after close returns error
- Handler receives packet from real KCP client
- Goroutine cleanup on stop
- No `internal/game` dependency

## Not Yet Implemented

- Protocol decoding in handler (deferred to room integration milestone)
- Room runtime wiring
- Session authentication/token validation
- Graceful session timeout and cleanup
- Reconnect handling
- Metrics integration
- MessagePack roundtrip over KCP (integration smoke test)
- Concurrent session stress test

## Relationship to WebSocket Transport

KCP (this spec) and WebSocket (`docs/specs/spec_websocket_transport.md`) are
parallel transports sharing one application protocol.

| Property              | KCP Transport           | WebSocket Transport       |
|-----------------------|-------------------------|---------------------------|
| Client platform       | Unity native            | Unity WebGL               |
| Network protocol      | KCP over UDP            | WSS/WebSocket over TCP    |
| Application protocol  | MessagePack Protocol v1 | MessagePack Protocol v1   |
| Listener port         | `:9000` (UDP)           | `:9001` (TCP, TLS)        |
| Status                | Skeleton implemented    | Skeleton implemented      |

Both transports normalize sessions to the same `RealtimeSession` interface.
The room loop is transport-agnostic.
