# ADR 0001: KCP over UDP + MessagePack

## Status

Accepted

## Context

The realtime data plane needs a transport that avoids TCP head-of-line blocking
under packet loss while providing reliability and ordering. Payload serialization
must be compact and iteratable while the protocol evolves.

## Decision

- Transport: KCP over UDP on port :9000.
- Serialization: MessagePack with protocol versioning from day one.
- Control plane remains HTTP/TCP + JSON on port :8080.

## Consequences

- Better latency under packet loss vs TCP/WebSocket.
- Less engineering cost than custom raw UDP reliability layer.
- MessagePack is more compact than JSON and easier to iterate than Protobuf.
- KCP has fewer production references than TCP/WebSocket — requires load test validation.
- Protocol versioning adds a mandatory check on every packet.

## Protocol v2 Future Candidate: Protobuf

Protobuf is the preferred future candidate if the protocol needs stronger schema governance. It is deferred intentionally, not rejected.

MessagePack is final for Protocol v1. No `.proto` files, protobuf dependencies, or generated code will be added at this stage.

Protocol v2 (Protobuf) may be reconsidered only when:

1. Protocol v1 MessagePack schema has stabilized.
2. Unity client contract has been validated in production.
3. Production load tests provide packet size, bandwidth, and CPU data.
4. Multiple Unity client versions must be supported long-term.
5. The team accepts Go + Unity/C# code generation workflow.
6. A backward compatibility and migration plan exists.
7. There is measurable benefit over MessagePack v1.

Any Protobuf migration must be treated as a Protocol v2 migration, not a silent codec swap. The KCP transport layer is codec-agnostic; only the serialization layer changes.
