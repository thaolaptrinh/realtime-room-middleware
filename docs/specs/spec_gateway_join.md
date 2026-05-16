# Spec: Gateway Join Flow

## Scope

Milestone 1 deliverable. HTTP gateway with `/healthz`, `/readyz`, and `/join` endpoints.
SingleNodeResolver returns local game server address. No Redis dependency.

## Key Decisions

- HTTP on `:8080`, JSON request/response (control plane only).
- Realtime KCP data plane uses MessagePack, not JSON.
- `/join` resolves logical room via `NodeResolver` interface.
- Returns game node UDP address, room instance ID, session token, protocol version.
- No external auth integration yet (opaque random token placeholder).
- Single-vps mode uses `SingleNodeResolver`; distributed mode will use `RedisNodeResolver` (future).

## HTTP Routes

| Method | Path       | Purpose                    |
|--------|------------|----------------------------|
| GET    | `/healthz` | Liveness check             |
| GET    | `/readyz`  | Readiness check            |
| POST   | `/join`    | Resolve room and get token |

## Join Request

```json
{
  "user_id": "user-123",
  "logical_room_id": "expo-room-a",
  "client_protocol_version": 1
}
```

All fields required.

## Join Response (200)

```json
{
  "room_instance_id": "expo-room-a-0042",
  "game_node_addr": "127.0.0.1:9000",
  "kcp_addr": "127.0.0.1:9000",
  "session_token": "<64-char hex string>",
  "protocol_version": 1,
  "expires_at": "2026-05-16T12:00:00Z"
}
```

## Error Responses

All errors return JSON with `error` and `code` fields:

| Code                  | HTTP Status | Condition                        |
|-----------------------|-------------|----------------------------------|
| `bad_request`         | 400         | Malformed JSON body              |
| `missing_user_id`     | 400         | `user_id` is empty               |
| `missing_logical_room_id` | 400     | `logical_room_id` is empty       |
| `unsupported_version` | 400         | Protocol version out of range    |
| `resolver_error`      | 500         | Internal resolver failure        |
| `token_error`         | 500         | Token generation failure         |

## Resolver

`NodeResolver` interface in `internal/gateway/resolver/`:

- `SingleNodeResolver`: returns configured single-node KCP address. Used by `dev` and `single-vps` modes.
- `RedisNodeResolver`: future, distributed-k3s only. No Redis dependency in current implementation.

## Session Token

Current implementation (`internal/gateway/token/`):

- Generates 32-byte opaque random hex tokens (64 chars).
- No HMAC signing or validation yet.
- Token is not validated by the game server yet.

Security hardening needed before production:

- Sign tokens with HMAC using config-driven secret.
- Validate tokens on game server side during KCP Hello/JoinRoom.
- Add expiry enforcement and revocation.
- Rotate signing keys without downtime.

## Join Flow

```
1. Unity client calls POST /join with user_id, logical_room_id, client_protocol_version.
2. Gateway validates required fields and protocol version.
3. Gateway calls NodeResolver.ResolveRoom(logicalRoomID, userID).
4. SingleNodeResolver returns configured KCP address and generated instance ID.
5. Gateway generates opaque session token.
6. Gateway returns JoinResponse with assignment details.
7. Unity client opens KCP connection to returned game_node_addr.
8. Unity client sends Hello + JoinRoom via KCP/MessagePack.
```

## Files

- `internal/gateway/http/server.go` — HTTP server, routes, lifecycle
- `internal/gateway/http/handlers.go` — handler implementations
- `internal/gateway/http/models.go` — request/response types
- `internal/gateway/resolver/resolver.go` — NodeResolver interface, SingleNodeResolver
- `internal/gateway/token/token.go` — session token generation
- `cmd/gateway/main.go` — gateway binary, wiring, graceful shutdown
- `internal/gateway/http/server_test.go` — handler unit tests
- `internal/gateway/resolver/resolver_test.go` — resolver unit tests
- `internal/gateway/token/token_test.go` — token unit tests
- `tests/integration/gateway_smoke_test.go` — smoke tests

## Tests

- `/healthz` returns 200 OK
- `/readyz` returns 200 OK
- `/join` returns valid assignment for single-node resolver
- `/join` rejects unsupported protocol version
- `/join` rejects missing `user_id`
- `/join` rejects missing `logical_room_id`
- `/join` rejects invalid JSON body
- `/join` rejects protocol version 0
- Single-vps mode does not require Redis
- Gateway package does not import game room runtime
