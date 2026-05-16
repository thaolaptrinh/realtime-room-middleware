# Protocol Specification

## Current Version

**Protocol version: 1**

Supported range: `[1, 1]`

## Protocol v1 Summary

Protocol v1 uses MessagePack as the shared application/gameplay payload format across all realtime transports.

Two realtime transport adapters are supported:

| Platform        | Transport         |
|-----------------|-------------------|
| Unity native    | KCP over UDP      |
| Unity WebGL     | WSS/WebSocket     |

The application-layer envelope, message types, and wire formats are **identical** on both transports. Transport selection is determined by the client platform at session open and is invisible to the room loop, delta broadcaster, and all game logic.

The HTTP/JSON Gateway remains the control plane for all clients. JSON is not used for realtime gameplay packets on either transport.

## Transport

- Control plane: HTTP/TCP JSON Gateway `:8080` (not covered here)
- Serialization: MessagePack (vmihailenco/msgpack/v5) — both realtime transports
- Realtime data plane (Unity native): KCP over UDP `:9000`
- Realtime data plane (Unity WebGL): WSS/WebSocket (TLS required, port TBD)

## Client Platform Support

| Platform        | Transport         | Status                   |
|-----------------|-------------------|--------------------------|
| Unity native    | KCP/UDP           | Implemented (skeleton)   |
| Unity WebGL     | WSS/WebSocket     | Implemented (skeleton)   |

## Transport Matrix

| Property                  | KCP/UDP              | WSS/WebSocket        |
|---------------------------|----------------------|----------------------|
| Target clients            | Unity native         | Unity WebGL          |
| Payload format            | MessagePack          | MessagePack          |
| Envelope                  | Identical            | Identical            |
| Message types             | Identical            | Identical            |
| Protocol version          | Same negotiation     | Same negotiation     |
| Latency/jitter            | Lower (no TCP HOL)   | Higher (TCP HOL)     |
| Browser/WebGL compatible  | No                   | Yes                  |
| JSON for gameplay packets  | No                   | No                   |

## Envelope

Every packet is wrapped in an Envelope before transport.

```go
type Envelope struct {
    Version uint16      `msgpack:"v"` // protocol version
    Type    MessageType `msgpack:"t"` // message type constant
    Seq     uint32      `msgpack:"s"` // sequence number (client increments per packet)
    Tick    uint32      `msgpack:"k"` // server tick at time of send (0 for client→server)
    Body    []byte      `msgpack:"b"` // encoded message payload
}
```

### Size Limits

| Limit          | Value   |
|----------------|---------|
| MaxPacketSize  | 64 KB   |
| MaxPayloadSize | 60 KB   |

## Message Types

### Client → Server (range 1-99)

| Type | ID | Struct | Phase | Status |
|------|----|--------|-------|--------|
| Hello | 1 | `Hello` | 1 | Implemented |
| JoinRoom | 2 | `JoinRoom` | 1 | Implemented |
| Reconnect | 3 | — | Future | Reserved |
| PlayerInput | 4 | `PlayerInput` | 1 | Reserved — Phase 1 |
| Ping | 5 | `Ping` | 1 | Implemented |
| PlayerTransformUpdate | 6 | `PlayerTransformUpdate` | 1 | Reserved — Phase 1 |
| LeaveRoom | 7 | — | Future | Reserved |

### Server → Client (range 1000-1999)

| Type | ID | Struct | Phase | Status |
|------|----|--------|-------|--------|
| Welcome | 1001 | `Welcome` | 1 | Implemented |
| JoinAccepted | 1002 | `JoinAccepted` | 1 | Implemented |
| ReconnectAccepted | 1003 | — | Future | Reserved |
| ReconnectRejected | 1004 | — | Future | Reserved |
| FullSnapshot | 1005 | `FullSnapshot` | 1 | Reserved — Phase 1 |
| PlayerDelta | 1006 | `PlayerDelta` | 1 | Reserved — Phase 1 |
| ObjectDelta | 1007 | — | Future | Reserved — Deferred |
| VoiceGroupDelta | 1008 | — | Future | Reserved — Deferred |
| LockAccepted | 1009 | — | Future | Reserved — Deferred |
| LockRejected | 1010 | — | Future | Reserved — Deferred |
| ClusterMembershipDelta | 1011 | — | 1 (optional) | Reserved — optional Phase 1 |
| Error | 1100 | `ServerError` | 1 | Implemented |
| Pong | 1101 | `Pong` | 1 | Implemented |

## Message Wire Formats

### Hello (client → server)

First message after KCP session opens.

```go
type Hello struct {
    Version uint16 `msgpack:"v"` // client protocol version
}
```

### JoinRoom (client → server)

Requests to join a room instance. Session token is obtained from the Gateway HTTP `/join` endpoint.

```go
type JoinRoom struct {
    RoomInstanceID string `msgpack:"ri"` // physical room instance ID
    SessionToken   string `msgpack:"st"` // from Gateway /join
    UserID         string `msgpack:"uid"`
}
```

### Ping (client → server)

Keep-alive probe.

```go
type Ping struct {
    Timestamp int64 `msgpack:"ts"` // client timestamp (epoch ms)
}
```

### Welcome (server → client)

Response to Hello.

```go
type Welcome struct {
    Version   uint16 `msgpack:"v"`  // server protocol version
    ServerID  string `msgpack:"sid"` // server instance ID
    Timestamp int64  `msgpack:"ts"`  // server timestamp (epoch ms)
}
```

### JoinAccepted (server → client)

Confirms room join.

```go
type JoinAccepted struct {
    RoomInstanceID string `msgpack:"ri"` // physical room instance ID
    LogicalRoomID  string `msgpack:"li"` // logical room ID
    PlayerID       string `msgpack:"pid"` // assigned player ID
    Tick            uint32 `msgpack:"tk"`  // current server tick
}
```

### Error (server → client)

Structured error response.

```go
type ServerError struct {
    Code    uint16 `msgpack:"code"`
    Message string `msgpack:"msg"`
}
```

Error codes:

| Code | Name                | Meaning                        |
|------|---------------------|--------------------------------|
| 1    | InvalidVersion      | Unsupported protocol version   |
| 2    | InvalidType         | Unknown or wrong-direction type |
| 3    | AuthFailed          | Session token validation failed |
| 4    | RoomFull            | Room at max capacity           |
| 5    | RoomNotFound        | Room instance does not exist   |
| 6    | PayloadTooLarge     | Body exceeds MaxPayloadSize    |
| 99   | Internal            | Unexpected server error        |

### Pong (server → client)

Response to Ping.

```go
type Pong struct {
    Timestamp  int64  `msgpack:"ts"` // echo of client timestamp
    ServerTick uint32 `msgpack:"tk"` // current server tick
}
```

---

## Phase 1 Message Wire Formats (Reserved — Not Yet Implemented)

The following message types are reserved for Phase 1 implementation. Wire formats are specified here to lock the schema before coding begins. No wire-format change may occur after first Unity client integration without a protocol version bump.

### PlayerInput (client → server, type 4)

Sent by the client at the player's input rate. Contains movement intent (not authoritative position).

```go
type PlayerInput struct {
    Seq      uint32  `msgpack:"s"`   // client input sequence number
    MoveX    float32 `msgpack:"mx"`  // movement direction X (-1.0 to 1.0)
    MoveZ    float32 `msgpack:"mz"`  // movement direction Z (-1.0 to 1.0)
    Yaw      float32 `msgpack:"y"`   // player facing angle (radians)
    AnimState uint16 `msgpack:"a"`   // animation state bitmask
}
```

### PlayerTransformUpdate (client → server, type 6)

Sent by the client when authoritative client-side position is available (e.g., physics result). Supplements or replaces `PlayerInput` depending on server authority model.

```go
type PlayerTransformUpdate struct {
    Seq      uint32  `msgpack:"s"`   // client sequence number
    X        float32 `msgpack:"x"`   // world position X
    Z        float32 `msgpack:"z"`   // world position Z
    Yaw      float32 `msgpack:"y"`   // facing angle (radians)
    AnimState uint16 `msgpack:"a"`   // animation state bitmask
}
```

### FullSnapshot (server → client, type 1005)

Sent once on join and on explicit resync request. Contains the full visible state for the joining session.

```go
type FullSnapshot struct {
    Tick    uint32          `msgpack:"tk"` // server tick at snapshot time
    Players []PlayerSnapshot `msgpack:"pl"` // all players visible to this session
}

type PlayerSnapshot struct {
    PlayerID  string  `msgpack:"pid"` // player identifier
    X         float32 `msgpack:"x"`
    Z         float32 `msgpack:"z"`
    Yaw       float32 `msgpack:"y"`
    AnimState uint16  `msgpack:"a"`
    Version   uint32  `msgpack:"v"`  // state version for delta tracking
}
```

### PlayerDelta (server → client, type 1006)

Sent at broadcast rate (default 10 Hz). Contains only changes since last broadcast for this client's interest set.

```go
type PlayerDelta struct {
    Tick    uint32               `msgpack:"tk"`
    Enters  []PlayerEnterDelta   `msgpack:"en"` // newly visible players
    Updates []PlayerUpdateDelta  `msgpack:"up"` // visible players with changed state
    Leaves  []PlayerLeaveDelta   `msgpack:"lv"` // players that left the interest range
}

type PlayerEnterDelta struct {
    PlayerID  string  `msgpack:"pid"`
    X         float32 `msgpack:"x"`
    Z         float32 `msgpack:"z"`
    Yaw       float32 `msgpack:"y"`
    AnimState uint16  `msgpack:"a"`
    Version   uint32  `msgpack:"v"`
}

type PlayerUpdateDelta struct {
    PlayerID  string  `msgpack:"pid"`
    X         float32 `msgpack:"x"`
    Z         float32 `msgpack:"z"`
    Yaw       float32 `msgpack:"y"`
    AnimState uint16  `msgpack:"a"`
    Version   uint32  `msgpack:"v"`
}

type PlayerLeaveDelta struct {
    PlayerID string `msgpack:"pid"`
}
```

Empty `PlayerDelta` (all slices nil/empty) must not be sent. The room skips encoding and sending if `PlayerDelta.IsEmpty()`.

### ClusterMembershipDelta (server → client, type 1011) — Optional Phase 1

Informs the client of its current cluster assignment. Optional in Phase 1; the client does not need cluster membership to render. Useful for debugging and future voice grouping pre-wiring.

Reserved. Wire format TBD if implemented in Phase 1.

---

## Deferred Protocol Message Areas

The following message types are **reserved** in the type table but are **not Phase 1 implementation targets**. Do not implement wire format or encoding for these until the relevant phase begins.

```txt
ObjectDelta (1007)       — Deferred / Future Scope — object sync
VoiceGroupDelta (1008)   — Deferred / Future Scope — voice grouping
LockAccepted (1009)      — Deferred / Future Scope — object locking
LockRejected (1010)      — Deferred / Future Scope — object locking
Reconnect (3)            — Deferred / Future Scope — reconnect flow
LeaveRoom (7)            — Deferred / Future Scope — graceful leave
```

No JSON realtime gameplay payloads exist for any message type on either transport. All reserved types will use MessagePack when implemented.

---

## Mixed Transport Room Semantics

Native and WebGL clients may coexist in the same room instance.

Rules:

- The application-layer envelope and all message types are identical regardless of transport.
- The room loop and delta broadcaster are transport-agnostic.
- Transport selection is per-client, resolved at session open, and invisible to room logic.
- Transport differences (latency, jitter) do not change gameplay semantics or message structure.
- JSON must not be used for realtime gameplay packets on either transport.
- A client's transport type does not determine which room events it receives.
- `FullSnapshot`, `PlayerDelta`, `ObjectDelta`, and `VoiceGroupDelta` are sent to all interested clients regardless of transport.

## Compatibility Rules

1. **Every packet carries a protocol version.** The server rejects packets with versions outside `[MinVersion, MaxVersion]`.
2. **Message types are stable.** No type ID will be reused for a different message.
3. **Fields are additive.** New fields may be added to message structs with default-zero semantics. Clients must tolerate unknown fields.
4. **No field removal without migration.** Fields used by the Unity client must not be removed without an explicit migration path.
5. **No wire-format change without updating this document.**

### Versioning Policy

- **Major version bump**: breaking wire-format change (field removal, type ID reassignment)
- **Minor version bump**: backward-compatible field addition
- **Patch**: server-only behavior change

## Gateway HTTP Control Plane

The HTTP Gateway (`:8080`) is separate from the KCP realtime data plane documented here.
The Gateway uses JSON for control-plane requests (health checks, room join).

See `docs/specs/spec_gateway_join.md` for Gateway route details.

Key relationship: `POST /join` returns a `session_token` that the client includes
in the `JoinRoom` message — whether the session uses KCP (Unity native) or WSS
(Unity WebGL). The `JoinRoom` wire format is identical on both transports.
Token validation on the game server is not yet implemented.

## Not Yet Implemented

### Phase 1 — Reserved and Specified, Not Yet Coded

Wire formats are specified above. Implementation is the Phase 1 gameplay milestone.

```txt
PlayerInput (4)              — spec above
PlayerTransformUpdate (6)    — spec above
FullSnapshot (1005)          — spec above
PlayerDelta (1006)           — spec above
ClusterMembershipDelta (1011) — optional, wire format TBD
```

### Deferred / Future Scope

Reserved type IDs only. Wire formats will be defined when the relevant phase begins.

```txt
Reconnect (3) / ReconnectAccepted (1003) / ReconnectRejected (1004) — Deferred
LeaveRoom (7)        — Deferred
ObjectDelta (1007)   — Deferred (object sync)
VoiceGroupDelta (1008) — Deferred (voice grouping)
LockAccepted (1009) / LockRejected (1010) — Deferred (object locking)
```

## Protocol v2 Future Candidate: Protobuf

Protobuf is not rejected forever. It is the preferred future candidate if the protocol needs stronger schema governance. However, it is deferred intentionally.

**MessagePack remains the production Protocol v1 on both transports (KCP and WSS).** No Protobuf implementation, `.proto` files, or protobuf dependencies exist or will be added at this stage.

### Why Protobuf is deferred

Protocol v1 uses MessagePack because the protocol is still evolving and MessagePack allows faster iteration. Protobuf adds value when schemas stabilize and multiple client versions must be maintained long-term.

### When Protocol v2 (Protobuf) may be reconsidered

Protocol v2 may be considered only when all of the following are true:

1. Protocol v1 MessagePack schema has stabilized.
2. Unity client contract has been validated in production on both native and WebGL platforms.
3. Production load tests provide packet size, bandwidth, and CPU data.
4. Multiple Unity client versions must be supported long-term.
5. The team accepts Go + Unity/C# code generation workflow.
6. A backward compatibility and migration plan exists.
7. There is measurable benefit over MessagePack v1.

### Migration rules

- Any Protobuf migration must be treated as a **Protocol v2 migration**, not a silent codec swap.
- Protocol v2 must support explicit compatibility and migration rules for both transports.
- Protocol v1 and v2 may need to coexist during migration.
- Both the KCP and WSS transport layers are codec-agnostic; only the serialization layer changes.
- A Protocol v2 migration does not change which transports are supported.
