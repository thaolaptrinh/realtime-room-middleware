# Protocol Specification

## Current Version

**Protocol version: 1**

Supported range: `[1, 1]`

## Transport

- Realtime data plane: KCP over UDP `:9000`
- Serialization: MessagePack (vmihailenco/msgpack/v5)
- Control plane: HTTP/TCP JSON Gateway `:8080` (not covered here)

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

| Type | ID | Struct       | Status      |
|------|----|--------------|-------------|
| Hello | 1 | `Hello`       | Implemented |
| JoinRoom | 2 | `JoinRoom`    | Implemented |
| Reconnect | 3 | —             | Reserved    |
| PlayerInput | 4 | —         | Reserved    |
| Ping | 5 | `Ping`         | Implemented |

### Server → Client (range 1000-1999)

| Type | ID | Struct              | Status      |
|------|----|---------------------|-------------|
| Welcome | 1001 | `Welcome`        | Implemented |
| JoinAccepted | 1002 | `JoinAccepted` | Implemented |
| ReconnectAccepted | 1003 | —         | Reserved    |
| ReconnectRejected | 1004 | —         | Reserved    |
| FullSnapshot | 1005 | —             | Reserved    |
| PlayerDelta | 1006 | —              | Reserved    |
| ObjectDelta | 1007 | —               | Reserved    |
| VoiceGroupDelta | 1008 | —           | Reserved    |
| LockAccepted | 1009 | —              | Reserved    |
| LockRejected | 1010 | —               | Reserved    |
| Error | 1100 | `ServerError`      | Implemented |
| Pong | 1101 | `Pong`             | Implemented |

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
in the KCP `JoinRoom` message. Token validation on the game server is not yet
implemented.

## Not Yet Implemented

The following are reserved and documented but not yet coded:

- Reconnect / ReconnectAccepted / ReconnectRejected
- PlayerInput
- FullSnapshot
- PlayerDelta
- ObjectDelta
- VoiceGroupDelta
- LockAccepted / LockRejected
- LeaveRoom (client → server)

These will be implemented in later milestones.

## Protocol v2 Future Candidate: Protobuf

Protobuf is not rejected forever. It is the preferred future candidate if the protocol needs stronger schema governance. However, it is deferred intentionally.

**MessagePack remains the production Protocol v1.** No Protobuf implementation, `.proto` files, or protobuf dependencies exist or will be added at this stage.

### Why Protobuf is deferred

Protocol v1 uses MessagePack because the protocol is still evolving and MessagePack allows faster iteration. Protobuf adds value when schemas stabilize and multiple client versions must be maintained long-term.

### When Protocol v2 (Protobuf) may be reconsidered

Protocol v2 may be considered only when all of the following are true:

1. Protocol v1 MessagePack schema has stabilized.
2. Unity client contract has been validated in production.
3. Production load tests provide packet size, bandwidth, and CPU data.
4. Multiple Unity client versions must be supported long-term.
5. The team accepts Go + Unity/C# code generation workflow.
6. A backward compatibility and migration plan exists.
7. There is measurable benefit over MessagePack v1.

### Migration rules

- Any Protobuf migration must be treated as a **Protocol v2 migration**, not a silent codec swap.
- Protocol v2 must support explicit compatibility and migration rules.
- Protocol v1 and v2 may need to coexist during migration.
- The KCP transport layer is codec-agnostic; only the serialization layer changes.
