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
