# Spec: KCP Transport Layer

> Implementation spec placeholder.

## Scope

Milestone 1 deliverable. KCP over UDP server lifecycle, session management,
packet I/O, configurable timeouts.

## Key Decisions

- Listen on configurable UDP port (default :9000).
- One KCP session per connected client.
- Session has read/write deadlines.
- Network goroutines push to room queues; do not mutate room state.

## Files

- `internal/transport/kcp/server.go`
- `internal/transport/kcp/session.go`
- `tests/integration/kcp_smoke_test.go`

## Tests Required

- KCP connect/disconnect
- MessagePack message roundtrip over KCP
- Session timeout behavior
- Concurrent session handling
