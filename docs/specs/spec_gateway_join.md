# Spec: Gateway Join Flow

> Implementation spec placeholder.

## Scope

Milestone 1 deliverable. HTTP gateway with /health and /join endpoints.
SingleNodeResolver returns local game server address.

## Key Decisions

- HTTP on :8080.
- /join resolves logical room via NodeResolver.
- Returns game node UDP address, room instance ID, session token, protocol version.
- No external auth integration yet (placeholder token).

## Files

- `internal/gateway/server.go`
- `internal/gateway/handlers.go`
- `internal/adapters/resolver/single_node.go`
- `tests/integration/gateway_smoke_test.go`

## Tests Required

- /health returns 200
- /join returns valid assignment
- /join with missing fields returns error
