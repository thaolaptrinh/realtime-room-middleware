# Spec: Protocol Envelope and MessagePack Codec

> Implementation spec placeholder.

## Scope

Milestone 1 deliverable. Defines the MessagePack envelope structure, version field,
all client→server and server→client message types, and encode/decode contract.

## Key Decisions

- Envelope carries version, type, seq, tick, body.
- Body is opaque bytes; message type determines decode schema.
- Protocol version must be checked on every packet.

## Files

- `internal/protocol/envelope.go`
- `internal/protocol/messages.go`
- `internal/protocol/codec.go`
- `internal/protocol/codec_test.go`

## Tests Required

- Envelope encode/decode roundtrip
- Invalid version rejection
- Unknown message type handling
- Backward-compatible field addition
