# Spec: KCP Transport Layer

## Scope

Milestone 1 deliverable. KCP over UDP server lifecycle, session management,
packet I/O, configurable timeouts.

This is a **transport skeleton only**. Room runtime integration, protocol
decoding in the handler, and business logic are intentionally deferred to later
milestones.

## Key Decisions

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
