# Architecture Overview

## High-Level System

```
Unity Client (native)                Unity Client (WebGL)
        |                                     |
        | HTTP :8080                           | HTTP :8080
        v                                     v
                  Gateway (control plane)
        |                                     |
        | KCP/UDP :9000 + MessagePack          | WSS + MessagePack
        v                                     v
                  Game Server (realtime data plane)
                         |
                  Room Loop (single writer)
                         |
           ┌─────────────┴─────────────┐
     Spatial Hash               Delta Broadcaster
```

## Control Plane vs Realtime Data Plane

**Control plane** (HTTP/TCP JSON, Gateway `:8080`):
- Room join and discovery
- Session token issuance
- Health and readiness checks
- Admin and debug endpoints
- Logical room → physical instance resolution

**Realtime data plane** (MessagePack, Game Server):
- KCP/UDP `:9000` — Unity native clients
- WSS/WebSocket — Unity WebGL clients (TLS required)
- Session authentication (token issued by Gateway)
- Player and object state, delta broadcast
- Spatial interest management, object locking, proximity grouping

JSON is used only on the control plane. Realtime gameplay packets always use MessagePack, regardless of transport.

## Dual Realtime Transport Architecture

Protocol v1 defines a single shared application-layer protocol (MessagePack envelope + message types). Two transport adapters deliver it:

```
RealtimeSession (interface)
    |
    ├── KCPSession   — wraps a KCP/UDP connection (Unity native)
    └── WSSSession   — wraps a WSS/WebSocket connection (Unity WebGL)
```

The `RealtimeSession` abstraction provides:
- `ReadPacket() ([]byte, error)`
- `WritePacket([]byte) error`
- `Close() error`
- `RemoteAddr() string`

### Transport Adapter Rules

Transport adapters must not:
- Mutate room state directly
- Bypass the room command queue
- Decode gameplay message bodies (envelope decode and version check only)

The room loop and delta broadcaster are transport-agnostic. They operate on encoded MessagePack payloads and push packets to whichever transport adapter owns the session.

## Room Loop Rule

```
Network goroutine (KCP or WSS)
    ↓
Read packet → decode envelope → validate version and type
    ↓
Push to room command queue
    ↓
Room loop (single goroutine per room)
    ↓
Mutate player/object state → compute delta → enqueue outbound packets
    ↓
Transport adapter → send encoded MessagePack packet
```

**Only the room loop may mutate room state.**
**Network goroutines may not call any room mutation method directly.**

## Component Interaction

```
KCPTransport / WSSTransport
    → SessionManager (maps session ID → RealtimeSession)
    → RoomManager (maps room instance ID → Room)
    → Room.Commands (channel, queues input to room loop)
    → Room loop (single goroutine per room)
    → SpatialIndex → InterestManager → DeltaBroadcaster
    → ObjectLockManager
    → VoiceGroupAllocator
```

## Data Flow: Join

1. Client calls Gateway `POST /join` over HTTP.
2. Gateway validates request and resolves logical room to physical instance.
3. Gateway returns: game server address, transport endpoint (`kcp_addr` or
   `websocket_url`), room instance ID, session token, protocol version.
4. Client opens transport connection:
   - Native: KCP/UDP to `kcp_addr` (`:9000`)
   - WebGL: WSS to `websocket_url` (TLS required)
5. Client sends `Hello` (version negotiation) — same MessagePack envelope on both transports.
6. Server sends `Welcome` or `Error(InvalidVersion)`.
7. Client sends `JoinRoom` with session token and room instance ID — same wire format on both transports.
8. Server validates token and attaches session to room.
9. Server sends `JoinAccepted` and `FullSnapshot`.
10. Server begins sending `PlayerDelta`, `ObjectDelta`, `VoiceGroupDelta` at broadcast rate.

## Data Flow: Tick and Broadcast

```
Room tick (20 Hz):
  drain input queues
  drain command queues
  update player and object state
  release expired locks
  update spatial hash

Broadcast (10 Hz):
  compute interest sets per client
  compute deltas per client
  encode MessagePack
  enqueue packets to transport adapters (KCP or WSS per session)
```

## Data Flow: Disconnect

```
Transport adapter detects close/error
  → session removed from SessionManager
  → RoomCommand{Disconnect, sessionID} pushed to room queue
  → room loop: release all locks owned by session
  → room loop: remove player from spatial index
  → room loop: emit player leave in next delta
```

## Position Cluster Sync Architecture

**Phase 1 gameplay implementation target.** Voice grouping, object locking, and object sync are deferred to future phases.

### Data Flow: Player Transform Sync

```
KCP or WSS packet arrives at transport goroutine
  → MessagePack envelope decode
  → protocol version and message type validation
  → room.Enqueue(RoomCommand{Kind: CmdPlayerInput, ...})
  → (transport goroutine exits — no room state touched)

Room loop (single goroutine per room):
  → drain player input queue
  → update PlayerState (position, rotation, animation state)
  → set dirty mask on changed fields
  → update spatial hash (player position → grid cell)
  → run ClusterAllocator.Compute(players) → cluster assignments
  → update per-player cluster membership

Broadcast tick:
  → for each session: compute interest set from cluster membership
  → compute PlayerDelta (enter/update/leave) vs ClientSnapshotCache
  → encode MessagePack payload
  → enqueue outbound packet to RealtimeSession (KCP or WSS per session)
```

### Component Roles — Phase 1

| Component | Role |
|---|---|
| `KCPTransport` / `WSSTransport` | Read packets, decode envelope, push to room queue. No room state mutation. |
| `Room loop` | Only writer of `PlayerState`, `SpatialIndex`, and cluster assignments. |
| `SpatialIndex` (GridSpatialHash) | Fast cell-based lookup of players by position. Updated by room loop. |
| `ClusterAllocator` (KMeans) | Groups players by position into clusters. Called by room loop. Output drives interest sets. |
| `InterestManager` | Builds per-client visible player sets from cluster membership. |
| `DeltaBroadcaster` | Computes per-client PlayerDelta using `ClientSnapshotCache`. Sends to `RealtimeSession`. |
| `RealtimeSession` | Transport abstraction. KCP or WSS session. Delivers encoded MessagePack packets. |

### Phase 1 Cluster Allocator Rule

The `ClusterAllocator` interface is the Phase 1 grouping primitive. K-Means is the first implementation. The interface is pluggable so future policies can be substituted without changing the room loop.

```
ClusterAllocator interface
  └── KMeansClusterAllocator   (Phase 1)
  └── ProximityClusterAllocator (future candidate)
```

Cluster allocation must not happen in transport goroutines. It is scheduled by the room loop at the room tick rate.

### Deferred Features

The following are defined in the architecture but are not Phase 1 implementation targets:

```txt
Voice grouping:      VoiceGroupAllocator, VoiceGroupDelta     — Deferred / Future Scope
Object locking:      ObjectLockManager, lease TTL model        — Deferred / Future Scope
Object sync:         ObjectState, ObjectDelta, object commands  — Deferred / Future Scope
```

Do not wire runtime behavior for deferred features until a future phase is explicitly started.

## Mixed Transport Room Semantics

Native and WebGL clients may coexist in the same room instance.

- The room loop, delta broadcaster, and all game logic are transport-agnostic.
- Transport differences (latency, jitter) do not change gameplay semantics or message structure.
- A client's transport type does not determine which room events it receives.
- The same `FullSnapshot`, `PlayerDelta`, `ObjectDelta`, and `VoiceGroupDelta` messages are sent to all interested clients, regardless of their transport.

## Deployment Mode Summary

| Property         | dev             | single-vps           | distributed-k3s    |
|------------------|-----------------|----------------------|--------------------|
| Gateway          | Docker Compose  | systemd binary       | K3s pod            |
| Game server      | Docker Compose  | systemd binary       | K3s pod            |
| KCP transport    | Yes             | Yes                  | Yes                |
| WSS transport    | Yes             | Yes (when enabled)   | Yes (when enabled) |
| Redis            | Optional        | No                   | Required           |
| Resolver         | single-node     | single-node          | Redis              |
| Registry         | memory          | memory               | Redis              |

## Reference

See `docs/full_production_architecture_workflow_blueprint.md` for source-of-truth decisions.
See `docs/protocol.md` for message types and wire formats.
See `docs/decisions/0001-kcp-msgpack.md` for transport decision record.
