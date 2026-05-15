# Spec: Protocol Envelope and MessagePack Codec

## Status

**Implemented** (Milestone 1, Stage 1 Task 1)

## Scope

Defines the MessagePack envelope structure, version field, foundation message types (Hello, JoinRoom, Ping, Welcome, JoinAccepted, Error, Pong), and encode/decode contract.

## Key Decisions

- Envelope carries version, type, seq, tick, body.
- Body is opaque bytes; message type determines decode schema.
- Protocol version is checked on every encode and decode.
- Message type IDs are partitioned: client→server (1-99), server→client (1000-1999).
- MaxPacketSize = 64 KB, MaxPayloadSize = 60 KB.
- Uses `github.com/vmihailenco/msgpack/v5` for MessagePack serialization.
- All struct fields use short `msgpack` tags (`v`, `t`, `s`, `k`, `b`, etc.) for compact wire format.

## Files

- `internal/protocol/protocol.go` — version constants, message type constants, Envelope struct, error types, validation functions
- `internal/protocol/messages.go` — Hello, JoinRoom, Ping, Welcome, JoinAccepted, ServerError, Pong structs
- `internal/protocol/codec.go` — EncodeEnvelope, DecodeEnvelope, EncodeMessage, DecodeMessage, BuildEnvelope, EncodeAndWrap, DecodeAndUnwrap
- `internal/protocol/codec_test.go` — 27 tests covering all foundation types and edge cases

## Tests Implemented

- Envelope encode/decode roundtrip (with and without zero fields)
- Version validation: reject version 0, reject version 99, accept current version
- Decode raw envelope with invalid version (bypasses encode validation)
- Payload size: reject oversized, accept max size
- Packet size: reject oversized, accept max size
- MessageType direction helpers (IsClientToServer, IsServerToClient)
- MessageType String() rendering
- Hello message roundtrip
- JoinRoom message roundtrip
- Ping message roundtrip
- Pong message roundtrip
- Welcome message roundtrip
- JoinAccepted message roundtrip
- ServerError message roundtrip
- ProtocolError implements error interface
- Full flow: EncodeAndWrap + DecodeAndUnwrap for Hello
- Full flow: EncodeAndWrap + manual decode for Ping
- Deterministic wire format for identical envelopes
- Decode garbage data returns error
- Decode empty data returns error
- Error codes are unique (no duplicates)

## Protocol v2 Future Candidate: Protobuf

Protobuf is not rejected forever. It is the preferred candidate if the protocol needs stronger schema governance. It is deferred intentionally.

MessagePack remains the production Protocol v1. No `.proto` files, protobuf dependencies, or generated Go/C# code exist or will be added at this stage.

Protocol v2 (Protobuf) may be considered only when:

1. Protocol v1 MessagePack schema has stabilized.
2. Unity client contract has been validated in production.
3. Production load tests provide packet size, bandwidth, and CPU data.
4. Multiple Unity client versions must be supported long-term.
5. The team accepts Go + Unity/C# code generation workflow.
6. A backward compatibility and migration plan exists.
7. There is measurable benefit over MessagePack v1.

Any Protobuf migration must be treated as a Protocol v2 migration, not a silent codec swap. Protocol v2 must support explicit compatibility and migration rules.

## Intentionally Not Implemented

- KCP transport (Stage 1 Task 2)
- Reconnect, PlayerInput, FullSnapshot, PlayerDelta, ObjectDelta, VoiceGroupDelta, LockAccepted, LockRejected messages (later milestones)
- Gateway HTTP /join endpoint (Stage 1 Task 3)
- Room manager and game runtime (Milestone 2+)
- Protobuf codec (deferred to Protocol v2, not currently needed)
