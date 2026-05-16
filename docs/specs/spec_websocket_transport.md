# Spec: WebSocket Transport Layer

## Scope

WebSocket/WSS transport for Unity WebGL clients. This spec defines the
production design for the sibling transport to KCP. It does not replace KCP
for Unity native clients.

This is a **transport skeleton specification only**. WebSocket server
implementation, WebGL client implementation, and room runtime integration are
intentionally deferred.

---

## 1. Goal

- Support Unity WebGL/browser clients in production.
- Use WSS/WebSocket because browsers cannot open raw UDP/KCP sockets.
- Carry the same MessagePack Protocol v1 binary payloads used by KCP native
  clients.
- Normalize WebSocket sessions into the same `RealtimeSession` abstraction used
  by KCP sessions so the room loop sees no transport difference.

---

## 2. Non-Goals

- Do not replace KCP for Unity native clients.
- Do not create a separate WebGL gameplay protocol.
- Do not use JSON for realtime gameplay packets.
- Do not implement WebRTC or WebTransport at this stage.
- Do not implement Protobuf or Protocol v2 at this stage.

---

## 3. Why WebSocket/WSS Is Required for WebGL

Browsers and the Unity WebGL runtime cannot open raw UDP sockets. KCP runs
over UDP and is therefore not available in the WebGL environment.

WSS (TLS WebSocket) is the only standards-compliant realtime transport
available to browser-based Unity WebGL clients.

| Context         | Transport    | Reason                                   |
|-----------------|--------------|------------------------------------------|
| Unity native    | KCP/UDP      | Low latency, no TCP head-of-line block   |
| Unity WebGL     | WSS          | Browser sandbox — raw UDP not available  |

Rules:

- WS (unencrypted) may be used only for local development.
- WSS is required for all public production endpoints.
- WebSocket is not an upgrade from KCP; it is a parallel transport path for
  a different client platform.

---

## 4. Relationship to MessagePack Protocol v1

WebSocket is a transport adapter. The gameplay and application protocol is
**identical** to the KCP transport.

| Property              | KCP/UDP              | WSS/WebSocket        |
|-----------------------|----------------------|----------------------|
| Payload format        | MessagePack          | MessagePack          |
| Envelope struct       | `Envelope`           | `Envelope`           |
| Message types         | Identical            | Identical            |
| Protocol version      | Same negotiation     | Same negotiation     |
| Frame type            | Raw bytes            | Binary WebSocket frame |
| JSON gameplay packets | Not allowed          | Not allowed          |

Every WebSocket binary frame must contain MessagePack Protocol v1 envelope
bytes. The `Envelope` struct, message type constants, and all message wire
formats defined in `docs/protocol.md` apply unchanged on the WebSocket
transport.

WebSocket does not define new message types or envelope fields. Any change to
the shared protocol must follow the versioning policy in `docs/protocol.md`.

---

## 5. Listener Design

The game server exposes two independent listeners:

```
Game Server
├── KCP listener     :9000/UDP   — Unity native clients
└── WebSocket listener :9001/TCP — Unity WebGL clients (TLS in production)
```

Both listeners normalize incoming connections into a common `RealtimeSession`
abstraction before the connection is handed to the room runtime. The room
loop and delta broadcaster never see transport-specific types.

Port `:9001` is the initial default for the WebSocket listener. The address
must be configurable via `ServerConfig`.

---

## 6. RealtimeSession Abstraction

Both KCP sessions and WebSocket sessions must satisfy the same interface:

```go
type RealtimeSession interface {
    UserID() string
    Transport() TransportType // kcp | websocket
    Send(packet []byte) error
    Close() error
}
```

`TransportType` is an enum/constant used only for observability (logging,
metrics). It must not be used to branch game logic or message routing in the
room loop.

The KCP transport implementation and the WebSocket transport implementation
each provide their own concrete type that satisfies this interface.

---

## 7. Binary Frame Requirements

- Only binary WebSocket frames are valid for realtime gameplay packets.
- Text frames must be rejected with an error close code and the session must
  be closed.
- The `MaxPayloadSize` limit from `docs/protocol.md` (60 KB) applies to the
  frame body.
- Frames exceeding `MaxPayloadSize` must be rejected and the session closed.
- When `FullSnapshot` is implemented, if a snapshot exceeds `MaxPayloadSize`,
  it must either be chunked per the snapshot chunking design or rejected with
  an `Error(PayloadTooLarge)` until chunking is implemented.

---

## 8. Authentication and Session Token Validation

- The WebSocket connection must present the session token obtained from the
  Gateway HTTP `/join` response.
- Token validation must complete before the session is added to any room or
  session registry.
- Token delivery mechanism (query parameter, first binary frame, HTTP header
  during upgrade) must be defined before implementation begins. HTTP header
  or first binary frame are preferred over query parameters to avoid token
  appearing in server access logs.
- No hardcoded tokens or secrets.
- Production token hardening (signing, expiry, revocation) follows the same
  requirements as the KCP transport.

---

## 9. Backpressure Handling

WebSocket clients over TCP are more susceptible to slow consumer behavior than
KCP over UDP. The transport layer must not allow a slow WebSocket client to
stall the room loop or block other sessions.

Rules:

- Each WebSocket session has a bounded outbound send queue (channel or ring
  buffer).
- The queue size must be configurable (`SendQueueSize`, default 256).
- When the send queue is full, the transport evaluates the message type:
  - Movement delta updates (`PlayerDelta`, `ObjectDelta`): drop the stale
    packet. Only the latest state matters. Document this drop in metrics.
  - Control messages (`JoinAccepted`, `Error`, `Pong`): do not drop. If the
    queue cannot accept control messages, the session is considered unhealthy.
- A session that cannot drain its send queue within `WriteTimeout` is closed
  with a documented disconnect reason.
- Send queue depth must be exposed as a metric for each session.

---

## 10. Slow Client Handling

| Parameter        | Default   | Description                                    |
|------------------|-----------|------------------------------------------------|
| `SendQueueSize`  | 256       | Max outbound packets queued per session        |
| `WriteTimeout`   | 5s        | Max time to complete a single frame write      |
| `ReadTimeout`    | 10s       | Max time between incoming frames               |

A slow client is defined as a session whose send queue remains at or above
`SendQueueSize` for longer than one broadcast interval. Such sessions must be
disconnected with close code `1001 Going Away` and the disconnect reason logged
as `slow_client`.

Slow WebSocket clients must never:

- Block the room loop goroutine.
- Block the send path of other sessions.
- Consume unbounded memory.

---

## 11. Origin, TLS, and Security Requirements

- WSS is required for all production endpoints. The WebSocket listener must
  refuse to start without a valid TLS configuration unless `dev` mode is
  explicitly set.
- Allowed origins must be configurable as an allowlist. Connections from
  unlisted origins must be rejected during the HTTP upgrade handshake.
- WS (unencrypted) is permitted only when `deployment.mode = dev`.
- Session tokens must be validated before room join (see section 8).
- Connection rate limiting per source IP is recommended and must be
  configurable. Rate limiting is enforced at the upgrade handler, before a
  session is created.
- No CORS bypass or wildcard origin in production.

---

## 12. Tests Required

| Test                                     | Description                                               |
|------------------------------------------|-----------------------------------------------------------|
| Connect success                          | WebSocket client completes upgrade and receives Welcome   |
| Reject invalid token                     | Session closed before room join on bad token              |
| Reject text frame                        | Text frame received → session closed, error logged        |
| Decode MessagePack binary frame          | Binary frame decoded to valid `Envelope`                  |
| Send MessagePack binary frame            | Server sends binary frame; client decodes valid `Envelope` |
| Enforce max payload size                 | Frame > MaxPayloadSize → session closed                   |
| Close session idempotently               | `Close()` called twice does not panic or error            |
| Backpressure / slow client disconnect    | Full send queue triggers disconnect within timeout        |
| No import of room runtime from transport | `internal/transport/websocket` must not import `internal/game` |
| WS rejected in non-dev mode              | Unencrypted listener does not start in production config  |

---

## 13. Acceptance Criteria

- WebSocket transport carries the same MessagePack Protocol v1 packets as KCP.
- WebSocket sessions satisfy the `RealtimeSession` interface.
- A KCP native client and a WebSocket WebGL client can coexist in the same
  room instance after room runtime integration.
- WebSocket transport does not mutate room state directly.
- WebSocket transport does not decode gameplay message bodies (envelope decode
  and version check only, per the transport adapter rule).
- `internal/transport/websocket` has no import of `internal/game`.

---

## 14. Configuration

`ServerConfig` fields for the WebSocket transport:

| Field             | Default      | Description                                |
|-------------------|--------------|--------------------------------------------|
| `ListenAddr`      | required     | TCP address (e.g. `:9001`)                 |
| `TLSCertFile`     | required     | TLS certificate (empty allowed in dev only)|
| `TLSKeyFile`      | required     | TLS private key (empty allowed in dev only)|
| `AllowedOrigins`  | required     | Allowlist of permitted WebSocket origins   |
| `MaxPayloadSize`  | 60 KB        | Max inbound frame payload size             |
| `ReadTimeout`     | 10s          | Per-read deadline                          |
| `WriteTimeout`    | 5s           | Per-write deadline                         |
| `SendQueueSize`   | 256          | Outbound channel depth per session         |
| `Logger`          | slog.Default | Structured logger                          |

---

## 15. Files (When Implemented)

- `internal/transport/websocket/handler.go` — `RealtimeSession`, upgrade handler
- `internal/transport/websocket/server.go` — `ServerConfig`, `WSSServer`, lifecycle
- `internal/transport/websocket/session.go` — `wssSession` (internal), read/write loops
- `internal/transport/websocket/server_test.go` — config validation, lifecycle, session tests

---

## 16. Not Yet Implemented

- WebSocket server implementation.
- Unity WebGL client implementation.
- Room runtime integration (WebSocket session ↔ room loop wiring).
- WebRTC or WebTransport.
- Protobuf Protocol v2.
- Snapshot chunking for large payloads.
- Metrics integration.
- Concurrent session stress test.

---

## 17. Relationship to KCP Transport

| Property                        | KCP Transport              | WebSocket Transport       |
|---------------------------------|----------------------------|---------------------------|
| Client platform                 | Unity native               | Unity WebGL               |
| Transport protocol              | KCP over UDP               | WSS/WebSocket over TCP    |
| Application payload format      | MessagePack Protocol v1    | MessagePack Protocol v1   |
| Envelope                        | `Envelope` struct          | `Envelope` struct         |
| Message types                   | Identical                  | Identical                 |
| Session abstraction             | `RealtimeSession`          | `RealtimeSession`         |
| Room loop visibility            | Transport-agnostic         | Transport-agnostic        |
| Status                          | Implemented (skeleton)     | Reserved — spec only      |

Both transports are required. Neither replaces the other. Native and WebGL
clients may coexist in the same room instance after room runtime integration.

See `docs/specs/spec_kcp_transport.md` for the KCP transport specification.
See `docs/protocol.md` for the shared Protocol v1 message formats.
See `docs/architecture.md` for the full dual-transport architecture.
