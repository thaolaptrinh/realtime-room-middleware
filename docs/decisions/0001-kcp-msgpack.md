# ADR 0001: KCP/UDP + WSS/WebSocket Transport with MessagePack Protocol v1

## Status

Accepted (updated: dual realtime transport)

## Context

The realtime data plane needs a transport that:
- Avoids TCP head-of-line blocking under packet loss for Unity native clients.
- Supports Unity WebGL clients, which cannot use raw UDP or KCP because the browser
  and Unity WebGL runtime do not permit opening raw UDP sockets.
- Uses compact, iteratable payload serialization while the protocol schema evolves.

## Decision

**Transports:**
- KCP over UDP on port `:9000` — Unity native clients.
- WSS/WebSocket (TLS WebSocket) — Unity WebGL clients. (Port TBD.)

**Serialization:**
- MessagePack (vmihailenco/msgpack/v5) — shared across both transports.
- Protocol versioning from day one, on both transports.

**Control plane:**
- HTTP/TCP + JSON on port `:8080` — all clients, unchanged.

**Application protocol:**
- One shared Protocol v1 with identical envelope, message types, and wire formats on both transports.
- Transport selection is per-client, resolved at session open, and invisible to room logic.

**Constraints:**
- JSON must not be used for realtime gameplay packets on either transport.
- Native and WebGL clients must not use separate gameplay protocols.
- Transport adapters must not mutate room state.
- The room loop is the only writer of room state.
- Native and WebGL clients may coexist in the same room instance.

## Consequences

**KCP/UDP (Unity native):**
- Better latency under packet loss vs TCP/WebSocket.
- Less engineering cost than a custom raw UDP reliability layer.
- No browser compatibility — Unity native only.
- Requires load test validation (fewer production references than TCP).

**WSS/WebSocket (Unity WebGL):**
- Required for Unity WebGL: browser and WebGL runtime cannot use raw UDP or KCP.
- Higher latency and jitter than KCP under packet loss (TCP head-of-line blocking).
- Standard production transport with wide library and infrastructure support.
- TLS is required for production WebGL delivery.
- Carries the same MessagePack envelope and message types as KCP — no separate WebGL protocol.

**Shared MessagePack protocol:**
- More compact than JSON.
- Easier to iterate than Protobuf while the protocol schema evolves.
- Supported in Go and Unity/C# on both transport paths.
- Protocol versioning is mandatory on both transports from day one.

**Mixed rooms:**
- Native and WebGL clients may coexist in the same room instance.
- Transport differences (latency, jitter) do not change gameplay semantics or message structure.

## Protocol v2 Future Candidate: Protobuf

Protobuf is the preferred future candidate if the protocol needs stronger schema governance. It is deferred intentionally, not rejected.

MessagePack is final for Protocol v1 on both transports. No `.proto` files, protobuf dependencies, or generated code will be added at this stage.

Protocol v2 (Protobuf) may be reconsidered only when:

1. Protocol v1 MessagePack schema has stabilized.
2. Unity client contract has been validated in production.
3. Production load tests provide packet size, bandwidth, and CPU data.
4. Multiple Unity client versions must be supported long-term.
5. The team accepts Go + Unity/C# code generation workflow.
6. A backward compatibility and migration plan exists.
7. There is measurable benefit over MessagePack v1.

Any Protobuf migration must be treated as a Protocol v2 migration, not a silent codec swap. Both the KCP and WSS transport layers are codec-agnostic; only the serialization layer changes.
