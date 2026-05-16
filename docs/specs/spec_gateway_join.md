# Spec: Gateway Join Flow

## Scope

Milestone 1 deliverable. HTTP gateway with `/healthz`, `/readyz`, and `/join` endpoints.
SingleNodeResolver returns local game server address. No Redis dependency.

Both Unity native (KCP/UDP) and Unity WebGL (WSS/WebSocket) clients use this same
endpoint. The Gateway assigns a transport-specific endpoint based on the client platform.
Both transports carry identical MessagePack Protocol v1 payloads — there is no separate
WebGL gameplay protocol.

## Key Decisions

- HTTP on `:8080`, JSON request/response (control plane only).
- Realtime KCP data plane uses MessagePack, not JSON.
- Realtime WSS data plane uses MessagePack, not JSON.
- `/join` resolves logical room via `NodeResolver` interface.
- Returns game node address, transport endpoint, room instance ID, session token, protocol version.
- `client_platform` determines the default transport assigned (`kcp` for `native`, `websocket` for `webgl`).
- `requested_transport` is optional and may override the platform default if supported.
- Gateway does not proxy realtime gameplay packets. It is control plane only.
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
  "client_protocol_version": 1,
  "client_platform": "native",
  "requested_transport": "kcp"
}
```

| Field                    | Type   | Required | Values                    | Notes                                          |
|--------------------------|--------|----------|---------------------------|------------------------------------------------|
| `user_id`                | string | yes      | —                         | Identifies the player                          |
| `logical_room_id`        | string | yes      | —                         | Product-facing room ID                         |
| `client_protocol_version`| int    | yes      | 1                         | Must be within supported range `[1,1]`         |
| `client_platform`        | string | yes      | `native`, `webgl`         | Determines default transport                   |
| `requested_transport`    | string | no       | `kcp`, `websocket`        | Override platform default if provided          |

All fields except `requested_transport` are required.

## Join Response (200) — Unity Native (KCP)

```json
{
  "room_instance_id": "expo-room-a-0042",
  "game_node_addr": "example.com",
  "protocol_version": 1,
  "session_token": "<64-char hex string>",
  "transport": "kcp",
  "kcp_addr": "example.com:9000",
  "expires_at": "2026-05-16T12:00:00Z"
}
```

## Join Response (200) — Unity WebGL (WebSocket)

```json
{
  "room_instance_id": "expo-room-a-0042",
  "game_node_addr": "example.com",
  "protocol_version": 1,
  "session_token": "<64-char hex string>",
  "transport": "websocket",
  "websocket_url": "wss://example.com/realtime",
  "expires_at": "2026-05-16T12:00:00Z"
}
```

## Join Response Field Reference

| Field              | Type   | Present when         | Notes                                                      |
|--------------------|--------|----------------------|------------------------------------------------------------|
| `room_instance_id` | string | always               | Physical room instance ID                                  |
| `game_node_addr`   | string | always               | Canonical game node hostname, transport-agnostic           |
| `protocol_version` | int    | always               | Negotiated protocol version                                |
| `session_token`    | string | always               | 64-char hex opaque token, used in KCP/WSS `JoinRoom`       |
| `transport`        | string | always               | Assigned transport: `kcp` or `websocket`                   |
| `kcp_addr`         | string | `transport=kcp`      | `host:port` for KCP/UDP connection                         |
| `websocket_url`    | string | `transport=websocket`| Full WSS URL for WebSocket connection                      |
| `expires_at`       | string | always               | RFC3339 token expiry                                       |

`kcp_addr` and `websocket_url` are mutually exclusive in the response: only the field
matching the assigned `transport` is populated.

## Error Responses

All errors return JSON with `error` and `code` fields:

| Code                       | HTTP Status | Condition                                   |
|----------------------------|-------------|---------------------------------------------|
| `bad_request`              | 400         | Malformed JSON body                         |
| `missing_user_id`          | 400         | `user_id` is empty                          |
| `missing_logical_room_id`  | 400         | `logical_room_id` is empty                  |
| `unsupported_version`      | 400         | Protocol version out of supported range     |
| `unsupported_platform`     | 400         | `client_platform` is not `native` or `webgl`|
| `unsupported_transport`    | 400         | `requested_transport` is provided but not supported |
| `resolver_error`           | 500         | Internal resolver failure                   |
| `token_error`              | 500         | Token generation failure                    |

## Validation Rules

1. `user_id` must be non-empty.
2. `logical_room_id` must be non-empty.
3. `client_protocol_version` must be within the supported range `[1, 1]`.
4. `client_platform` must be `native` or `webgl`.
5. If `requested_transport` is provided, it must be `kcp` or `websocket`.
6. If `client_platform=native`, default transport is `kcp`.
7. If `client_platform=webgl`, default transport is `websocket`.
8. A native client may request `websocket` via `requested_transport` if supported by the server.
9. A WebGL client must not request `kcp` (browsers cannot open UDP sockets).

## Transport Assignment Logic

```
if requested_transport is provided:
    validate requested_transport is supported
    use requested_transport
else:
    if client_platform == "native":  transport = "kcp"
    if client_platform == "webgl":   transport = "websocket"

if transport == "kcp":
    populate kcp_addr
    do not populate websocket_url

if transport == "websocket":
    populate websocket_url
    do not populate kcp_addr
```

## Resolver

`NodeResolver` interface in `internal/gateway/resolver/`:

- `SingleNodeResolver`: returns configured single-node KCP address and WSS URL. Used by `dev` and `single-vps` modes.
- `RedisNodeResolver`: future, distributed-k3s only. No Redis dependency in current implementation.

## Session Token

Current implementation (`internal/gateway/token/`):

- Generates 32-byte opaque random hex tokens (64 chars).
- No HMAC signing or validation yet.
- Token is included in both the KCP `JoinRoom` message and the WSS `JoinRoom` message — the
  wire format is identical on both transports.

Security hardening needed before production:

- Sign tokens with HMAC using config-driven secret.
- Validate tokens on game server side during KCP and WSS Hello/JoinRoom.
- Add expiry enforcement and revocation.
- Rotate signing keys without downtime.

## Join Flow

```
1. Unity client calls POST /join with user_id, logical_room_id,
   client_protocol_version, client_platform.
2. Gateway validates required fields and protocol version.
3. Gateway validates client_platform (and requested_transport if provided).
4. Gateway calls NodeResolver.ResolveRoom(logicalRoomID, userID).
5. SingleNodeResolver returns configured address(es) and generated instance ID.
6. Gateway assigns transport based on client_platform / requested_transport.
7. Gateway generates opaque session token.
8. Gateway returns JoinResponse with transport endpoint and assignment details.
9. Unity native client opens KCP/UDP connection to kcp_addr.
   Unity WebGL client opens WSS connection to websocket_url.
10. Client sends Hello + JoinRoom via its transport, with MessagePack encoding.
```

Both KCP and WSS clients send the same `Hello` and `JoinRoom` message structs,
encoded with the same MessagePack Protocol v1 envelope. Gateway assigns the endpoint;
the game server handles both session types transparently.

## Mixed Room Behavior

Native KCP clients and WebGL WebSocket clients may join the same room instance.

- The Gateway assigns each client a transport-appropriate endpoint, but routes them to
  the same `room_instance_id`.
- The game server's room loop and delta broadcaster are transport-agnostic. They operate
  on encoded MessagePack payloads and push packets to whichever transport adapter owns
  the session.
- The room runtime must not care whether a session originated from KCP or WebSocket.
- All room events (`FullSnapshot`, `PlayerDelta`, `ObjectDelta`, `VoiceGroupDelta`) are
  delivered to all interested clients regardless of their transport.
- Transport differences (latency, jitter) do not affect gameplay semantics or message
  structure.

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
- `/join` returns valid KCP assignment for `client_platform=native`
- `/join` returns valid WSS assignment for `client_platform=webgl`
- `/join` rejects unsupported protocol version
- `/join` rejects missing `user_id`
- `/join` rejects missing `logical_room_id`
- `/join` rejects invalid JSON body
- `/join` rejects protocol version 0
- `/join` rejects unsupported `client_platform`
- `/join` rejects unsupported `requested_transport`
- `/join` for `client_platform=native` populates `kcp_addr`, not `websocket_url`
- `/join` for `client_platform=webgl` populates `websocket_url`, not `kcp_addr`
- Native and WebGL clients assigned to same room instance share `room_instance_id`
- Single-vps mode does not require Redis
- Gateway package does not import game room runtime

## What Remains Intentionally Unimplemented

- WSS/WebSocket server on the game server (reserved for later milestone)
- Token HMAC signing and server-side validation
- `RedisNodeResolver` and distributed room assignment
- `requested_transport` cross-platform override enforcement (beyond validation)
- WebGL TLS certificate provisioning
- Token revocation
