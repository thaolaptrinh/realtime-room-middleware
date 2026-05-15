# Full Production Architecture & Workflow Blueprint

**Project:** Custom realtime middleware server for Unity / Normcore replacement layer  
**Target:** 200 CCU per room instance, multi-room, production-ready single VPS now, distributed scale path ready  
**Primary runtime language:** Go  
**Current production infra constraint:** 1 Sakura Cloud Tokyo VPS  
**Scale target:** K3s + Redis + KEDA + container registry  
**Document status:** Full blueprint for implementation, operations, load testing, and Claude Code workflows

---

## 0. Source Context and Assumptions

This blueprint is based on the provided project notes and conversation decisions.

The existing server is experimental/demo-only, hard-coded for a single room, not production-ready, not staging-ready, does not support multi-room, has outdated clustering/channel allocation logic, and does not yet implement scalability for traffic spikes.

The target system must support:

- 200 concurrent users per room instance.
- Multi-room management.
- Player position and rotation synchronization.
- Room object synchronization.
- Object locking to prevent concurrent interactions.
- Proximity-based synchronization and culling.
- Unity client integration.
- Reduced reliance on Normcore for room-scale synchronization.
- Production-ready deployment on one VPS first.
- A clean migration path to distributed K3s/KEDA scale without rewriting core logic.

Important assumptions:

- Voice media transport itself may still be handled externally or by a separate voice layer. This server manages voice/proximity grouping metadata unless explicitly expanded later.
- The 15–20 KB/s/user bandwidth estimate is a planning assumption, not a validated benchmark.
- The 30m radius is a configurable default, not a hard-coded constant.
- The current infra budget only provides a single VPS, but the codebase must be scale-ready from day one.

---

## 1. Executive Summary

The recommended architecture is a **spatial-interest-driven realtime server**.

It is not a K-Means-driven architecture. K-Means can exist as one implementation of voice/proximity grouping, but it should not be the foundation of realtime visibility or synchronization.

Core design:

```txt
HTTP/TCP Gateway        = control plane
KCP over UDP            = realtime data plane
MessagePack             = realtime payload serialization
Spatial Hashing         = interest management core
Delta Broadcast         = bandwidth optimization core
Room Loop               = single authority for room state mutation
Command Queue + Lease   = object locking consistency model
Voice Allocator         = pluggable policy layer
```

Deployment strategy:

```txt
One codebase
One shared realtime core
Three runtime modes

dev:
  Docker Compose

single-vps:
  2 Go binaries + systemd
  no Docker
  no Redis required
  no K3s required

distributed-k3s:
  Docker images
  K3s
  Redis
  KEDA
  ECR/container registry
```

The single VPS mode is not a throwaway prototype. It is the initial production deployment mode. Distributed mode is built as a separate deployment implementation and adapter set, while sharing the same protocol, room logic, object locking, spatial indexing, delta broadcast, and Unity client contract.

---

## 2. Final Technical Decisions

### 2.1 Transport

```txt
Control plane:
- HTTP/TCP
- JSON
- Gateway port :8080

Realtime data plane:
- KCP over UDP
- MessagePack
- Game server port :9000
```

### 2.2 Why KCP

KCP is the best practical fit for the realtime sync layer because:

- It avoids TCP/WebSocket head-of-line blocking behavior under packet loss.
- It is more production-practical than raw UDP because reliability, ordering, retransmission, and congestion behavior are already handled at the KCP layer.
- It is suitable for movement, rotation, object deltas, lock commands/results, full snapshots, and proximity metadata.
- It gives a better engineering-cost/performance tradeoff than implementing a custom UDP transport.

### 2.3 Why MessagePack

MessagePack is selected because:

- It is more compact than JSON.
- It is easier to iterate with than Protobuf while the protocol is still evolving.
- It has support in Go and Unity/C#.
- It is suitable for binary realtime payloads.

Mandatory guardrail:

```txt
MessagePack must be versioned from day one.
MessagePack is final for Protocol v1.
```

### 2.4 Protocol v2 Future Candidate: Protobuf

Protobuf is not rejected forever. It is the preferred future candidate if the protocol needs stronger schema governance. It is deferred intentionally.

```txt
No .proto files now.
No protobuf dependencies now.
No generated Go/C# protobuf code now.
```

MessagePack remains the production Protocol v1. Any Protobuf migration must be treated as a Protocol v2 migration, not a silent codec swap.

Protocol v2 may be reconsidered only when:

1. Protocol v1 MessagePack schema has stabilized.
2. Unity client contract has been validated in production.
3. Production load tests provide packet size, bandwidth, and CPU data.
4. Multiple Unity client versions must be supported long-term.
5. The team accepts Go + Unity/C# code generation workflow.
6. A backward compatibility and migration plan exists.
7. There is measurable benefit over MessagePack v1.

### 2.4 What Not to Use for Realtime Core

Rejected for realtime data plane:

```txt
WebSocket/TCP:
- easier to implement
- but poorer latency behavior under packet loss for movement sync

Raw UDP:
- best theoretical performance
- but too high engineering cost/risk for this project stage
```

Still use HTTP/TCP for:

- Auth/session.
- Room join.
- Room discovery.
- Healthcheck.
- Admin/debug.
- Deployment/ops endpoints.

---

## 3. System Architecture

### 3.1 High-Level Flow

```txt
Unity Client
   |
   | HTTP :8080
   v
Gateway
   |
   | join response:
   | - game node UDP address
   | - room instance id
   | - session token
   | - protocol version
   v
Unity Client
   |
   | KCP/UDP :9000 + MessagePack
   v
Game Server
   |
   | Room loop
   | Spatial hash
   | Interest manager
   | Object lock manager
   | Delta broadcaster
   v
Room State
```

### 3.2 Control Plane Responsibilities

The Gateway owns:

- Join room request.
- Create room request.
- Resolve logical room to physical room instance.
- Return game server endpoint to Unity.
- Session token generation.
- Health/readiness endpoints.
- Optional admin/debug endpoints.
- In distributed mode: Redis-backed node and room lookup.

The Gateway must not own high-frequency realtime state.

### 3.3 Realtime Data Plane Responsibilities

The Game Server owns:

- KCP sessions.
- Authentication of realtime session token.
- Room membership.
- Player state.
- Object state.
- Object locking.
- Spatial indexing.
- Per-client interest sets.
- Delta broadcast.
- Voice/proximity grouping metadata.
- Room lifecycle and cleanup.

---

## 4. Deployment Modes

## 4.1 Development Mode

Development uses Docker Compose even though single VPS production does not.

Purpose:

- Consistent local environment.
- Fast local startup.
- Optional Redis to test distributed path.
- Optional Prometheus/Grafana.
- Repeatable load test environment.

Services:

```txt
gateway
game-server
redis optional
prometheus optional
grafana optional
loadtest optional
```

Development can test both runtime styles:

```txt
dev + single-node resolver + memory registry
dev + redis resolver + redis registry
```

## 4.2 Single VPS Production Mode

Current production deployment mode.

```txt
Cloud: Sakura Cloud Tokyo
Runtime: 2 Go binaries
Process manager: systemd
Gateway: HTTP :8080
Game server: KCP/UDP :9000
Resolver: SingleNodeResolver
Registry: InMemoryRoomRegistry
CI/CD: CodeCommit → CodeBuild → scp binary → systemd restart
```

No required dependencies:

```txt
- Docker
- Redis
- K3s
- KEDA
- ECR
```

This mode must be production-hardened with:

- systemd restart policy.
- Firewall.
- Healthchecks.
- KCP smoke test.
- Load test.
- Rollback.
- Logs.
- CPU/memory/network monitoring.
- Secret hygiene.

## 4.3 Distributed K3s Production Mode

Scale mode, built separately but sharing the same core.

```txt
Cloud: Sakura Cloud Tokyo
Runtime: K3s
Gateway: always-on pod
Redis: always-on pod
Game nodes: spawn on demand
Resolver: RedisNodeResolver
Registry: RedisRoomRegistry metadata
Autoscale: KEDA watches Redis pending-room queue
CI/CD: CodeCommit → CodePipeline → CodeBuild → ECR → kubectl/helm
```

Distributed dependencies:

```txt
- Docker image
- Container registry/ECR
- K3s
- Redis
- KEDA
- Prometheus/Grafana
```

Important rule:

```txt
Live room state remains owned by the game node memory.
Redis stores routing/metadata, not authoritative live simulation state.
No live room migration in the initial design.
```

---

## 5. Shared Core Runtime Design

Shared across development, single VPS, and distributed K3s.

```txt
internal/game
internal/protocol
internal/transport/kcp
internal/gateway
internal/config
internal/observability
```

Mode-specific logic belongs only in adapters and deployment folders.

### 5.1 Core Components

```txt
RoomManager
Room
SessionManager
PlayerStateStore
ObjectStateStore
ObjectLockManager
SpatialIndex
InterestManager
VoiceGroupAllocator
DeltaBroadcaster
ProtocolCodec
KCPTransport
```

### 5.2 Single Authority Mutation Rule

```txt
Network goroutines must not mutate room state directly.
Only the room loop mutates room state.
```

Network goroutines may:

- Read packets.
- Decode envelope.
- Validate basic structure.
- Push input/commands into room queues.
- Enqueue outbound packets.

Room loop may:

- Mutate player state.
- Mutate object state.
- Grant/release locks.
- Update spatial index.
- Build deltas.
- Update snapshot caches.

---

## 6. Core Interfaces

### 6.1 NodeResolver

```go
type NodeResolver interface {
    ResolveRoom(ctx context.Context, logicalRoomID string, userID string) (NodeAssignment, error)
    AssignRoom(ctx context.Context, logicalRoomID string, opts AssignOptions) (NodeAssignment, error)
    ReportNodeHealth(ctx context.Context, health NodeHealth) error
}
```

Single VPS implementation:

```txt
SingleNodeResolver
- returns configured local game server address
- no Redis
- no distributed health lookup
```

Distributed implementation:

```txt
RedisNodeResolver
- reads room→node metadata from Redis
- reads node health heartbeat
- assigns new room to healthy node
- pushes pending-room request when capacity is unavailable
- routes reconnects to original room instance when possible
```

### 6.2 RoomRegistry

```go
type RoomRegistry interface {
    CreateRoom(ctx context.Context, spec RoomSpec) (*RoomInstance, error)
    GetRoom(ctx context.Context, instanceID string) (*RoomInstance, error)
    ListInstances(ctx context.Context, logicalRoomID string) ([]RoomInstance, error)
    MarkClosed(ctx context.Context, instanceID string) error
}
```

Single VPS:

```txt
InMemoryRoomRegistry
- logicalRoomID → room instances
- instanceID → live room pointer
```

Distributed:

```txt
RedisRoomRegistry
- metadata only
- live room remains in owning game node memory
```

### 6.3 SpatialIndex

```go
type SpatialIndex interface {
    UpdatePlayer(playerID string, pos Vec2)
    RemovePlayer(playerID string)
    UpdateObject(objectID string, pos Vec2)
    RemoveObject(objectID string)
    QueryPlayersRadius(pos Vec2, radius float32) []PlayerID
    QueryObjectsRadius(pos Vec2, radius float32) []ObjectID
}
```

Implementation:

```txt
GridSpatialHash
```

### 6.4 InterestManager

```go
type InterestManager interface {
    BuildInterestSet(room *Room, viewerID string) InterestSet
}
```

```go
type InterestSet struct {
    VisiblePlayers  []PlayerID
    VisibleObjects  []ObjectID
    VoiceCandidates []PlayerID
}
```

### 6.5 VoiceGroupAllocator

```go
type VoiceGroupAllocator interface {
    Allocate(players []PlayerState, cfg VoiceConfig) []VoiceGroup
}
```

Implementations:

```txt
ProximityVoiceAllocator
KMeansVoiceAllocator optional
```

Recommended initial default:

```txt
ProximityVoiceAllocator
```

K-Means should stay behind this interface.

### 6.6 ObjectLockManager

```go
type ObjectLockManager interface {
    RequestLock(cmd ObjectCommand, now time.Time) LockResult
    RefreshLock(cmd ObjectCommand, now time.Time) LockResult
    ReleaseLock(cmd ObjectCommand, now time.Time) LockResult
    ReleaseExpired(now time.Time) []ObjectID
    ReleaseByUser(userID string, now time.Time) []ObjectID
}
```

Consistency model:

```txt
server-authoritative command queue + lease TTL
```

---

## 7. Domain Model

### 7.1 Room Identity

Use two levels of room identity.

```txt
LogicalRoomID:
- user/product-facing room id
- e.g. "expo-room-a"

RoomInstanceID:
- physical runtime instance
- e.g. "expo-room-a-1"
- e.g. "expo-room-a-2"
```

Why:

- Supports overflow.
- Supports distributed assignment.
- Avoids migrating live rooms.
- Lets the Gateway choose an instance while Unity still thinks in logical room terms.

### 7.2 Room

```go
type Room struct {
    InstanceID     string
    LogicalRoomID  string
    Players        map[PlayerID]*PlayerState
    Objects        map[ObjectID]*ObjectState
    Sessions       map[SessionID]*Session
    SpatialIndex   SpatialIndex
    SnapshotCaches map[SessionID]*ClientSnapshotCache
    Commands       chan RoomCommand
    Inputs         chan PlayerInput
}
```

### 7.3 Player State

```go
type PlayerState struct {
    ID           string
    Position     Vec2
    Rotation     float32
    AnimState    uint16
    Version      uint32
    Dirty        DirtyMask
    LastInputSeq uint32
    LastSeenAt   time.Time
}
```

### 7.4 Object State

```go
type ObjectState struct {
    ID        string
    Type      string
    Position  Vec2
    State     []byte
    Version   uint32
    LockedBy  string
    LockUntil int64
}
```

---

## 8. Runtime Flow

### 8.1 Join Flow

```txt
1. Unity calls Gateway /join over HTTP.
2. Gateway validates request/session.
3. Gateway resolves logical room:
   - single-vps: SingleNodeResolver
   - distributed: RedisNodeResolver
4. Gateway returns:
   - game node UDP address
   - room instance id
   - session token
   - protocol version
5. Unity opens KCP connection to game-server :9000.
6. Unity sends Hello/JoinRoom via KCP + MessagePack.
7. Game server validates token.
8. Game server attaches session to room.
9. Server sends FullSnapshot.
10. Server starts sending deltas.
```

### 8.2 Room Tick Loop

```txt
Room Tick Loop
├─ drain input queues
├─ drain object command queue
├─ validate movement and commands
├─ update player state
├─ process object lock/interact commands
├─ release expired object locks
├─ update spatial hash
├─ compute interest set per client
├─ allocate voice/proximity groups
├─ compute player delta
├─ compute object delta
├─ compute voice delta
├─ encode MessagePack
└─ enqueue KCP packets
```

### 8.3 Tick and Broadcast Rates

Initial suggested config:

```yaml
game:
  tick_rate_hz: 20
  broadcast_rate_hz: 10
```

Meaning:

- Room simulation/input processing runs at 20 Hz.
- Network deltas are sent at 10 Hz unless load tests show a better value.
- Lock results and important object commands may be sent immediately or at next broadcast tick depending on UX needs.

---

## 9. Protocol Architecture

### 9.1 Envelope

```go
type Envelope struct {
    Version uint16 `msgpack:"v"`
    Type    uint16 `msgpack:"t"`
    Seq     uint32 `msgpack:"s"`
    Tick    uint32 `msgpack:"k"`
    Body    []byte `msgpack:"b"`
}
```

### 9.2 Client to Server Messages

```txt
Hello
JoinRoom
Reconnect
PlayerInput
PlayerTransform
ObjectCommand
Ping
LeaveRoom
```

### 9.3 Server to Client Messages

```txt
Welcome
JoinAccepted
ReconnectAccepted
ReconnectRejected
FullSnapshot
PlayerDelta
ObjectDelta
VoiceGroupDelta
LockAccepted
LockRejected
Error
Pong
```

### 9.4 Protocol Rules

```txt
- Every packet has a protocol version.
- Every message type is documented.
- No wire-format change without docs/protocol.md update.
- No removal of Unity-used fields without explicit migration.
- FullSnapshot is used for join/reconnect/resync.
- Normal operation uses deltas.
- Packet size must be measured in load tests.
```

### 9.5 Versioning Policy

```txt
Major version:
- breaking wire-format change

Minor version:
- backward-compatible field addition

Patch:
- server-only behavior change
```

---

## 10. Player Synchronization

### 10.1 Sync Policy

Nearby players:

```txt
- full avatar
- position/rotation
- animation state
- higher update frequency
```

Far players:

```txt
- blue avatar or low LOD metadata
- basic movement only
- lower update frequency
- possibly no object-level detail
```

### 10.2 Radius Configuration

```yaml
interest:
  visual_radius_m: 30
  object_radius_m: 30
  voice_radius_m: 30
  full_avatar_radius_m: 30
  low_lod_radius_m: 30
```

All are independently configurable even if initially set to 30m.

### 10.3 Client Rendering Responsibility

Unity should handle:

- Interpolation.
- Smoothing.
- Animation playback.
- Blue avatar rendering.
- Low-poly/LOD model switching.
- Object culling.
- Resync after FullSnapshot.

Server should not send unnecessary full state.

---

## 11. Object Synchronization and Locking

### 11.1 Problem

Multiple users can interact with the same room object such as:

- Chair.
- Speaker.
- Projector.
- Monitor.
- Other interactive objects.

Need to prevent conflicting interactions.

### 11.2 Chosen Model

```txt
Server-authoritative command queue + lease lock
```

Not chosen:

```txt
Optimistic locking only:
- worse UX because client may think interaction succeeded then get rejected

Permanent lock:
- can get stuck on disconnect/crash
```

### 11.3 Lock Flow

```txt
Client sends LockObject(objectID)
Server validates:
  - object exists
  - player exists
  - player within object radius
  - player has permission
  - object unlocked or lock expired
Server grants lock:
  - LockedBy = userID
  - LockUntil = now + TTL
  - object version++
Server broadcasts ObjectDelta to interested clients
```

### 11.4 Refresh Flow

```txt
Client sends RefreshLock(objectID)
Server validates:
  - current user owns lock
  - lock not expired
Server extends LockUntil
```

### 11.5 Release Flow

```txt
Client sends ReleaseLock(objectID)
Server validates ownership
Server clears lock
Server increments version
Server broadcasts ObjectDelta
```

### 11.6 Disconnect Flow

```txt
On client disconnect:
- release locks owned by user
- increment affected object versions
- broadcast deltas to interested clients
```

### 11.7 Lock Config

```yaml
object_lock:
  lease_ttl_ms: 10000
  refresh_interval_ms: 3000
  max_locks_per_user: 3
```

---

## 12. Spatial Hashing and Interest Management

### 12.1 Why Spatial Hashing

With 200 users/room, full broadcast is too expensive. Spatial hashing provides a deterministic, testable, low-cost way to find nearby players and objects.

### 12.2 Cell Config

```yaml
spatial:
  cell_size_m: 10
  max_query_radius_m: 50
```

### 12.3 Query Flow

```txt
viewer position
→ compute grid cell
→ query neighboring cells
→ filter by exact distance
→ produce visible player/object candidates
```

### 12.4 Interest Set

For each client:

```txt
InterestSet:
- visible players
- visible objects
- voice candidates
```

### 12.5 Hard Rule

```txt
Interest management must be deterministic and testable.
K-Means must not be the only source of truth for visibility.
```

---

## 13. Delta Broadcast

### 13.1 Per-Client Snapshot Cache

```go
type ClientSnapshotCache struct {
    VisiblePlayers map[PlayerID]uint32
    VisibleObjects map[ObjectID]uint32
    VoiceVersion   uint32
}
```

### 13.2 Delta Semantics

```txt
Enter:
- entity newly visible
- send compact snapshot

Update:
- entity still visible
- version changed
- send changed fields

Leave:
- entity no longer visible
- tell client hide/remove/degrade
```

### 13.3 PlayerDelta

```txt
PlayerDelta:
- tick
- enters
- updates
- leaves
```

### 13.4 ObjectDelta

```txt
ObjectDelta:
- tick
- object enters
- object updates
- object leaves
- lock state changes
```

### 13.5 VoiceGroupDelta

```txt
VoiceGroupDelta:
- group id
- joined users
- left users
- blue avatar mode
- group metadata version
```

### 13.6 Hard Rule

```txt
No full-room full-state broadcast during normal operation.
```

---

## 14. Voice / Proximity Grouping

### 14.1 Recommended Initial Policy

Use proximity-based grouping first.

```txt
Spatial hash → voice candidates → max N participants per group
```

### 14.2 K-Means Policy

K-Means can be implemented behind the same interface:

```txt
VoiceGroupAllocator = KMeansVoiceAllocator
```

Do not wire K-Means directly into the room loop as a foundation.

### 14.3 Config

```yaml
voice:
  allocator: proximity # proximity | kmeans
  radius_m: 30
  max_participants_per_group: 8
  recompute_interval_ms: 250
```

### 14.4 Why Pluggable

K-Means may cause:

- Flicker.
- Debug complexity.
- Unstable group switching.
- Extra CPU cost.
- No natural max-size guarantee.

Keeping it pluggable lets the team benchmark and switch policy without rewriting core logic.

---

## 15. Configuration Design

### 15.1 Shared Config Example

```yaml
deployment:
  mode: single-vps # dev | single-vps | distributed-k3s

gateway:
  http_addr: ":8080"

game:
  kcp_addr: ":9000"
  tick_rate_hz: 20
  broadcast_rate_hz: 10
  max_players_per_room: 200

protocol:
  version: 1
  serialization: msgpack
  transport: kcp

resolver:
  type: single-node # single-node | redis
  single_node_addr: "127.0.0.1:9000"
  redis_addr: ""

registry:
  type: memory # memory | redis

spatial:
  cell_size_m: 10

interest:
  visual_radius_m: 30
  object_radius_m: 30
  voice_radius_m: 30
  full_avatar_radius_m: 30
  low_lod_radius_m: 30

voice:
  allocator: proximity
  max_participants_per_group: 8
  recompute_interval_ms: 250

object_lock:
  lease_ttl_ms: 10000
  refresh_interval_ms: 3000
  max_locks_per_user: 3

metrics:
  type: log # log | prometheus
```

### 15.2 Mode Mapping

```txt
dev:
- Docker Compose
- resolver = single-node or redis
- registry = memory or redis

single-vps:
- resolver = single-node
- registry = memory
- metrics = log or prometheus local

distributed-k3s:
- resolver = redis
- registry = redis
- metrics = prometheus
```

---

## 16. Single VPS Production Design

### 16.1 Runtime Layout

```txt
/opt/realtime-server/
├─ releases/
│  ├─ 2026-xx-xx-001/
│  │  ├─ gateway
│  │  └─ game-server
│  └─ 2026-xx-xx-002/
├─ current -> releases/2026-xx-xx-002
├─ config/
│  └─ production.yaml
└─ logs/
```

### 16.2 systemd Services

```txt
gateway.service
game-server.service
```

Required settings:

```txt
Restart=always
RestartSec=3
LimitNOFILE=1048576
WorkingDirectory=/opt/realtime-server/current
Environment=CONFIG_PATH=/opt/realtime-server/config/production.yaml
```

### 16.3 Firewall

Allow:

```txt
TCP :8080
UDP :9000
SSH from admin IP only
```

Deny:

```txt
all other inbound traffic
```

### 16.4 Single VPS CI/CD

```txt
CodeCommit
→ CodeBuild
→ go test ./...
→ go test -race ./...
→ go build ./cmd/gateway
→ go build ./cmd/game-server
→ package release
→ scp to VPS /opt/realtime-server/releases/{release_id}
→ update symlink
→ systemctl restart gateway
→ systemctl restart game-server
→ healthcheck
→ KCP smoke test
```

### 16.5 Single VPS Rollback

```txt
1. Find previous release.
2. Point /opt/realtime-server/current to previous release.
3. Restart gateway.
4. Restart game-server.
5. Run HTTP healthcheck.
6. Run KCP smoke test.
7. Confirm logs and metrics.
```

### 16.6 Single VPS Readiness Checklist

```txt
[ ] systemd services installed
[ ] config file exists
[ ] firewall configured
[ ] TCP :8080 reachable
[ ] UDP :9000 reachable
[ ] HTTP healthcheck passes
[ ] KCP smoke test passes
[ ] rollback tested
[ ] logs available
[ ] CPU monitored
[ ] memory monitored
[ ] bandwidth monitored
[ ] 50 CCU load test passed
[ ] 100 CCU load test passed
[ ] 200 CCU load test passed or bottleneck documented
```

---

## 17. Distributed K3s Production Design

### 17.1 Runtime Architecture

```txt
K3s Cluster
├─ Gateway deployment, min replicas 1
├─ Redis, min replicas 1
├─ Game node deployment, scale 0..N
├─ KEDA ScaledObject
├─ Prometheus
└─ Grafana
```

### 17.2 Redis Responsibilities

Redis stores:

```txt
room metadata
node heartbeat
user session routing metadata
pending room queue
KEDA trigger data
```

Redis does not store:

```txt
authoritative live simulation state
full player state per tick
full object state per tick
```

### 17.3 Redis Key Design

```txt
room:logical:{logicalRoomID}:instances
room:instance:{instanceID}
node:{nodeID}:health
user:{userID}:session
queue:pending_rooms
```

### 17.4 Scale Flow

```txt
1. Gateway receives join request.
2. RedisNodeResolver checks existing room instances.
3. If no healthy capacity:
   - push pending room request to Redis queue.
4. KEDA sees queue length.
5. KEDA scales game-node deployment.
6. New game node starts.
7. Game node registers heartbeat.
8. Gateway assigns room instance to new node.
9. Unity connects via KCP to assigned game node.
```

### 17.5 Distributed CI/CD

```txt
CodeCommit
→ CodePipeline
→ CodeBuild
→ go test ./...
→ go test -race ./...
→ docker build gateway
→ docker build game-server
→ push images to ECR
→ kubectl/helm upgrade
→ rollout status
→ smoke test HTTP
→ smoke test KCP
→ scale test
```

### 17.6 Distributed Readiness Checklist

```txt
[ ] K3s cluster ready
[ ] Redis running
[ ] Gateway running
[ ] Game node image pushed
[ ] KEDA installed
[ ] KEDA ScaledObject configured
[ ] Redis pending-room queue works
[ ] Game node heartbeat works
[ ] Gateway resolver uses Redis
[ ] UDP service reachable
[ ] Prometheus metrics scraped
[ ] Grafana dashboard exists
[ ] scale from zero tested
[ ] scale down idle tested
[ ] rollback image tested
[ ] Redis failure runbook tested
```

---

## 18. Development Environment

### 18.1 Docker Compose

Development folder:

```txt
deployments/dev/
├─ docker-compose.yml
├─ gateway.Dockerfile
├─ game-server.Dockerfile
└─ config/dev.yaml
```

### 18.2 Dev Commands

```bash
make dev-up
make dev-down
make dev-logs
make dev-restart
make dev-redis
```

### 18.3 Dev Scenarios

```txt
Scenario A:
- resolver=single-node
- registry=memory
- simulates single VPS mode

Scenario B:
- resolver=redis
- registry=redis
- tests distributed path locally
```

---

## 19. Load Test Strategy

Load testing is mandatory because the 15–20 KB/s/user estimate is only a guess.

### 19.1 Load Test Goals

Validate:

- 50 CCU.
- 100 CCU.
- 200 CCU.
- Join storm.
- Movement sync.
- Object locking.
- Delta packet size.
- CPU under load.
- Memory under load.
- Bandwidth under load.
- KCP packet loss behavior.
- Reconnect behavior.
- Room cleanup.

### 19.2 Shared Load Test Client

Build a Go load test client using the same protocol:

```txt
loadtest/shared/kcp_client.go
```

It should support:

```txt
- connect via HTTP /join
- open KCP session
- send Hello/JoinRoom
- receive FullSnapshot
- send movement updates
- send object lock commands
- collect packet stats
- collect latency stats
```

### 19.3 Scenarios

```txt
scenario_join:
- clients join gradually

scenario_join_storm:
- many clients join at once

scenario_200ccu_movement:
- 200 clients move randomly
- validate delta broadcast

scenario_object_lock:
- many clients compete for same objects
- validate lease lock correctness

scenario_packet_loss:
- simulate packet loss if supported

scenario_reconnect:
- disconnect/reconnect clients
- validate session recovery

scenario_idle_cleanup:
- clients leave
- room cleanup happens
```

### 19.4 Metrics to Capture

```txt
server CPU %
server memory MB
network in/out Mbps
packets per second
bytes per second per client
p50/p95/p99 latency
KCP retransmits
dropped packets
room tick duration
delta build duration
spatial query duration
object lock success/reject rate
snapshot size
delta size
GC pauses
goroutine count
```

### 19.5 Acceptance Targets

Initial targets for single VPS:

```txt
50 CCU:
- must pass comfortably

100 CCU:
- should pass with CPU < 60%

200 CCU:
- target CPU < 75%
- memory stable
- bandwidth < 100Mbps
- no goroutine leak
- p95 visible update latency acceptable for Unity UX
```

If 200 CCU does not pass:

```txt
- document bottleneck
- identify whether CPU, bandwidth, KCP, serialization, spatial, delta, or Unity-side behavior is limiting
- do not guess
```

---

## 20. Observability

### 20.1 Single VPS Observability

Minimum:

```txt
journald logs
structured JSON logs
health endpoints
basic metrics endpoint optional
node CPU/memory/network monitoring
log rotation
```

Recommended:

```txt
Prometheus node exporter
Prometheus local or external scrape
Grafana dashboard
```

### 20.2 Distributed Observability

Required:

```txt
Prometheus
Grafana
KEDA metrics
Redis metrics
Gateway metrics
Game node metrics
```

### 20.3 Required Metrics

Gateway:

```txt
http_requests_total
join_requests_total
join_errors_total
resolver_latency_ms
room_assignments_total
```

Game server:

```txt
active_rooms
active_sessions
active_players
room_tick_duration_ms
delta_build_duration_ms
spatial_query_duration_ms
bytes_sent_total
bytes_received_total
packets_sent_total
packets_received_total
kcp_retransmits_total
object_locks_active
object_lock_rejects_total
```

Distributed:

```txt
redis_latency_ms
node_heartbeat_age_seconds
pending_rooms_queue_length
keda_scaled_replicas
game_node_ready_count
```

---

## 21. Failure Scenarios

### 21.1 Single VPS High CPU

Symptoms:

```txt
CPU > 75%
tick duration grows
latency increases
```

Actions:

```txt
1. Check active rooms/sessions.
2. Check tick duration metrics.
3. Check delta size.
4. Check load test pattern.
5. Reduce broadcast rate if needed.
6. Enable overflow room for new joins.
7. Do not migrate live users.
```

### 21.2 High Bandwidth

Actions:

```txt
1. Check bytes/client.
2. Check whether full snapshots are being sent too often.
3. Check delta enter/update/leave logic.
4. Check object sync radius.
5. Check broadcast rate.
6. Check MessagePack payload size.
```

### 21.3 UDP/KCP Connectivity Issue

Actions:

```txt
1. Confirm firewall allows UDP :9000.
2. Run KCP smoke test.
3. Check server logs.
4. Check packet receive counters.
5. Confirm Gateway returns correct node address.
```

### 21.4 Redis Failure in Distributed Mode

Actions:

```txt
1. Gateway should fail readiness if Redis unavailable.
2. Existing game rooms continue if already running.
3. New room assignment may fail or degrade.
4. Do not kill active game nodes automatically.
5. Follow Redis recovery runbook.
```

### 21.5 KEDA Scale Failure

Actions:

```txt
1. Check pending-room queue.
2. Check KEDA operator logs.
3. Check ScaledObject.
4. Check image pull.
5. Check game-node readiness.
6. Manually scale game-node deployment if needed.
```

---

## 22. Repo Structure

```txt
repo/
├─ README.md
├─ CLAUDE.md
├─ Makefile
├─ go.work
├─ go.mod
├─ go.sum
├─ cmd/
│  ├─ gateway/
│  └─ game-server/
├─ internal/
│  ├─ config/
│  ├─ protocol/
│  ├─ transport/
│  ├─ gateway/
│  ├─ game/
│  ├─ adapters/
│  └─ observability/
├─ deployments/
│  ├─ dev/
│  ├─ single-vps/
│  └─ distributed-k3s/
├─ infra/
│  ├─ single-vps/
│  └─ distributed-k3s/
├─ ci/
│  ├─ single-vps/
│  └─ distributed-k3s/
├─ loadtest/
│  ├─ shared/
│  ├─ single-vps/
│  └─ distributed-k3s/
├─ tests/
├─ docs/
└─ .claude/
```

Rule:

```txt
Shared realtime logic goes under internal/.
Mode-specific deployment goes under deployments/, infra/, ci/.
Do not duplicate room/protocol/delta/object logic per mode.
```

---

## 23. Documentation Structure

```txt
docs/
├─ architecture.md
├─ protocol.md
├─ room-lifecycle.md
├─ object-locking.md
├─ interest-management.md
├─ delta-broadcast.md
├─ voice-grouping.md
├─ load-testing.md
├─ modes/
│  ├─ dev.md
│  ├─ single-vps.md
│  └─ distributed-k3s.md
├─ decisions/
│  ├─ 0001-kcp-msgpack.md
│  ├─ 0002-single-vps-and-distributed-modes.md
│  ├─ 0003-spatial-hash-interest-management.md
│  └─ 0004-object-lock-command-queue-lease.md
└─ runbooks/
   ├─ deploy-single-vps.md
   ├─ rollback-single-vps.md
   ├─ high-cpu.md
   ├─ high-bandwidth.md
   ├─ packet-loss.md
   ├─ room-overflow.md
   ├─ redis-failure.md
   └─ keda-scale-failure.md
```

---

## 24. Makefile Targets

```makefile
build:
	go build ./cmd/gateway
	go build ./cmd/game-server

test:
	go test ./...

test-race:
	go test -race ./...

lint:
	golangci-lint run

smoke-gateway:
	go test ./tests/integration -run TestGatewaySmoke

smoke-kcp:
	go test ./tests/integration -run TestKCPSmoke

bench-spatial:
	go test ./internal/game/spatial -bench=.

bench-delta:
	go test ./internal/game/delta -bench=.

loadtest-50:
	./loadtest/single-vps/run_50ccu.sh

loadtest-100:
	./loadtest/single-vps/run_100ccu.sh

loadtest-200:
	./loadtest/single-vps/run_200ccu.sh

dev-up:
	docker compose -f deployments/dev/docker-compose.yml up

dev-down:
	docker compose -f deployments/dev/docker-compose.yml down
```

---

## 25. Test Matrix

### 25.1 Unit Tests

Protocol:

```txt
- envelope encode/decode
- message type compatibility
- invalid version
- unknown message type
```

Spatial:

```txt
- player enters cell
- player leaves cell
- boundary position
- negative coordinate if map supports it
- query radius correctness
```

Interest:

```txt
- visible players within radius
- hidden players outside radius
- visible objects within radius
- object culling outside radius
```

Delta:

```txt
- enter
- update
- leave
- no-change no packet
- full snapshot fallback
```

Object lock:

```txt
- lock success
- lock reject when owned
- expired lock can be acquired
- refresh by owner
- refresh reject by non-owner
- release on disconnect
```

Voice:

```txt
- max group size
- proximity allocation
- KMeans allocation if enabled
- stable grouping under small movement
```

### 25.2 Integration Tests

```txt
- Gateway join returns node address.
- Unity-like client connects via KCP.
- JoinRoom returns FullSnapshot.
- Movement produces PlayerDelta.
- Object lock produces ObjectDelta.
- Disconnect releases locks.
- Reconnect gets FullSnapshot or recovery.
```

### 25.3 Load Tests

```txt
- 50 CCU movement
- 100 CCU movement
- 200 CCU movement
- object lock contention
- join storm
- reconnect storm
- idle cleanup
```

---

## 26. Claude Code Workflow

## 26.1 `.claude/` Structure

```txt
.claude/
├─ settings.json
├─ commands/
│  ├─ plan.md
│  ├─ implement.md
│  ├─ protocol-change.md
│  ├─ gateway-change.md
│  ├─ room-change.md
│  ├─ spatial-change.md
│  ├─ delta-change.md
│  ├─ object-lock-change.md
│  ├─ voice-change.md
│  ├─ infra-single-vps-change.md
│  ├─ infra-distributed-change.md
│  ├─ loadtest.md
│  ├─ release-single-vps.md
│  └─ review.md
├─ agents/
│  ├─ go-network-reviewer.md
│  ├─ protocol-compat-reviewer.md
│  ├─ concurrency-reviewer.md
│  ├─ realtime-sync-reviewer.md
│  ├─ infra-reviewer.md
│  └─ loadtest-reviewer.md
└─ specs/
   ├─ single-vps-production/
   └─ distributed-k3s-scale/
```

## 26.2 CLAUDE.md

```md
# Project Context

## Product
Custom realtime middleware server for Unity, replacing part of Normcore synchronization for 200 CCU room instances.

## Deployment Modes
- dev: Docker Compose
- single-vps: Go binaries + systemd, no Docker, no Redis required
- distributed-k3s: K3s + Redis + KEDA + container registry

## Transport
- Control plane: HTTP/TCP JSON Gateway :8080
- Realtime data plane: KCP over UDP :9000
- Realtime payload: MessagePack

## Core Architecture
- Spatial hashing for interest management
- Delta broadcast for bandwidth reduction
- Room loop is the only writer of room state
- Network goroutines push inputs/commands into queues
- Object locking uses server command queue + lease TTL
- Voice grouping is pluggable; K-Means is optional, not foundational

## Hard Rules
- Do not full-broadcast room state in normal ticks.
- Do not mutate room state from network goroutines.
- Do not change protocol format without updating docs/protocol.md and tests.
- Do not duplicate core logic between single-vps and distributed modes.
- Do not introduce Redis dependency into single-vps runtime unless explicitly requested.
- Do not introduce Docker dependency into single-vps production.
- Do not run destructive infra commands.
- Do not edit secrets or .env files.
- Do not deploy or restart production services unless explicitly approved.

## Verification
Gateway changes:
- make test
- make smoke-gateway

Game server changes:
- make test
- make test-race
- make smoke-kcp

Protocol changes:
- update docs/protocol.md
- run protocol compatibility tests
- run smoke-kcp

Spatial/delta changes:
- run unit tests
- run benchmark if performance-sensitive
- run loadtest if behavior affects bandwidth

Infra changes:
- plan/diff only unless explicitly approved
- update runbook
```

---

## 27. Claude Commands

### 27.1 plan.md

```md
---
description: Create an implementation plan without editing files
argument-hint: [task]
---

Task: $ARGUMENTS

Rules:
1. Do not edit files.
2. Identify affected mode: dev, single-vps, distributed-k3s, or shared core.
3. Identify affected area: protocol, gateway, room, spatial, delta, object, voice, infra, CI/CD, loadtest.
4. Read relevant docs and code.
5. Produce:
   - goal
   - affected files
   - implementation steps
   - risks
   - tests to run
   - rollback notes
6. For protocol or infra changes, require explicit approval before implementation.
```

### 27.2 protocol-change.md

```md
---
description: Safely change MessagePack/KCP protocol
argument-hint: [change description]
---

Protocol change: $ARGUMENTS

Rules:
1. Inspect docs/protocol.md first.
2. Preserve backward compatibility unless explicitly approved.
3. Update docs/protocol.md.
4. Update protocol tests and fixtures.
5. Run:
   - make test
   - make smoke-kcp
6. Report Unity client impact.
```

### 27.3 room-change.md

```md
---
description: Change room lifecycle, membership, overflow, or cleanup
argument-hint: [change description]
---

Room change: $ARGUMENTS

Rules:
1. Inspect docs/room-lifecycle.md.
2. Do not migrate live rooms.
3. Preserve logical room vs room instance distinction.
4. Add tests for join, leave, reconnect, cleanup.
5. Run:
   - make test
   - make test-race
   - make smoke-kcp
```

### 27.4 object-lock-change.md

```md
---
description: Change object synchronization or locking logic
argument-hint: [change description]
---

Object lock change: $ARGUMENTS

Rules:
1. Inspect docs/object-locking.md.
2. Preserve command queue + lease TTL model.
3. Add tests for lock, reject, refresh, release, expiration, disconnect.
4. Run:
   - make test
   - make test-race
   - make smoke-kcp
5. Report impact on object state versioning and ObjectDelta.
```

### 27.5 delta-change.md

```md
---
description: Change delta broadcast or snapshot cache
argument-hint: [change description]
---

Delta broadcast change: $ARGUMENTS

Rules:
1. Inspect docs/delta-broadcast.md.
2. Preserve enter/update/leave semantics.
3. Do not introduce full-room broadcast in normal ticks.
4. Add tests for enter, update, leave, no-op, full snapshot fallback.
5. Run:
   - make test
   - make bench-delta
   - make smoke-kcp
6. Report packet size impact if measurable.
```

### 27.6 infra-single-vps-change.md

```md
---
description: Change single VPS deployment, systemd, scripts, or CI/CD
argument-hint: [change description]
---

Single VPS infra change: $ARGUMENTS

Rules:
1. Do not run production commands automatically.
2. Do not restart services without explicit approval.
3. Do not edit secrets.
4. Update deployments/single-vps and docs/runbooks.
5. Include rollback steps.
6. Prefer dry-run or script validation.
```

### 27.7 infra-distributed-change.md

```md
---
description: Change distributed K3s, Redis, KEDA, ECR, or Kubernetes manifests
argument-hint: [change description]
---

Distributed infra change: $ARGUMENTS

Rules:
1. Do not run kubectl apply unless explicitly approved.
2. Do not run terraform apply unless explicitly approved.
3. Update deployments/distributed-k3s and runbooks.
4. Validate Redis/KEDA/Gateway/Game node interactions.
5. Include rollback plan.
```

### 27.8 loadtest.md

```md
---
description: Create or run load test plan
argument-hint: [scenario]
---

Load test: $ARGUMENTS

Rules:
1. Identify target mode: dev, single-vps, or distributed-k3s.
2. Define CCU, movement pattern, object interaction, duration.
3. Capture CPU, memory, bandwidth, latency, packet stats.
4. Do not claim capacity without measured results.
5. Save findings in docs/load-testing.md.
```

---

## 28. Claude Agents

### 28.1 go-network-reviewer.md

```md
---
name: go-network-reviewer
description: Reviews Go KCP/UDP networking code for timeout, packet handling, reconnect, and production safety
tools: Read, Grep, Bash
---

Review networking changes only.

Focus:
- KCP session lifecycle
- timeout/deadline behavior
- goroutine leaks
- packet parsing
- reconnect behavior
- send queue backpressure
- error handling
- logging volume

Do not modify files.

Output:
1. Blocking issues
2. Race/leak risks
3. Protocol compatibility risks
4. Missing tests
5. Suggested verification commands
```

### 28.2 protocol-compat-reviewer.md

```md
---
name: protocol-compat-reviewer
description: Reviews MessagePack protocol changes and Unity compatibility risks
tools: Read, Grep, Bash
---

Focus:
- protocol versioning
- message type changes
- schema compatibility
- full snapshot vs delta correctness
- Unity client impact
- docs/protocol.md consistency

Do not modify files.
```

### 28.3 concurrency-reviewer.md

```md
---
name: concurrency-reviewer
description: Reviews Go concurrency, room loop, queues, locks, and race risks
tools: Read, Grep, Bash
---

Focus:
- room loop single-writer rule
- goroutine lifecycle
- channel close behavior
- lock ordering
- sync.Map usage
- race detector coverage
- disconnect cleanup

Require make test-race for approval.
Do not modify files.
```

### 28.4 realtime-sync-reviewer.md

```md
---
name: realtime-sync-reviewer
description: Reviews spatial hashing, interest management, delta broadcast, and object sync correctness
tools: Read, Grep, Bash
---

Focus:
- spatial query correctness
- interest set correctness
- enter/update/leave delta semantics
- object lock delta
- packet size growth
- no full broadcast regression
- load test coverage

Do not modify files.
```

### 28.5 infra-reviewer.md

```md
---
name: infra-reviewer
description: Reviews systemd, CI/CD, K3s, Redis, KEDA, Terraform, and deployment safety
tools: Read, Grep, Bash
---

Rules:
- Never apply infra changes.
- Never access secrets.
- Never run destructive commands.
- Prefer dry-run, diff, validate, and plan.

Output:
1. Deployment risks
2. Rollback risks
3. Secret/config risks
4. Observability gaps
5. Manual verification checklist
```

### 28.6 loadtest-reviewer.md

```md
---
name: loadtest-reviewer
description: Reviews load test scenarios, metrics, acceptance targets, and benchmark validity
tools: Read, Grep, Bash
---

Focus:
- realistic 200 CCU behavior
- movement patterns
- object interaction patterns
- packet size measurement
- CPU/memory/bandwidth capture
- clear pass/fail criteria
- no unsupported capacity claims

Do not modify files.
```

---

## 29. Claude Hooks and Safety Rules

### 29.1 Deny Dangerous Commands

Deny:

```txt
rm -rf
terraform apply
kubectl apply
kubectl delete
systemctl restart
systemctl stop
scp to production
ssh production
docker push
editing .env
editing secrets
```

### 29.2 Post-Edit Hooks

Recommended:

```txt
- gofmt changed Go files
- optional go test for touched package
- secret pattern scan
```

### 29.3 Stop Hook Reminder

Every completion should include:

```txt
- changed files
- tests run
- tests not run
- risks
- rollback notes if relevant
```

### 29.4 Example `.claude/settings.json`

```json
{
  "permissions": {
    "deny": [
      "Bash(rm -rf*)",
      "Bash(terraform apply*)",
      "Bash(kubectl apply*)",
      "Bash(kubectl delete*)",
      "Bash(systemctl restart*)",
      "Bash(systemctl stop*)",
      "Bash(scp *production*)",
      "Bash(ssh *production*)",
      "Bash(docker push*)",
      "Edit(.env*)",
      "Edit(*secret*)"
    ]
  },
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Edit|Write",
        "hooks": [
          {
            "type": "command",
            "command": "gofmt -w $(git diff --name-only -- '*.go') 2>/dev/null || true"
          }
        ]
      }
    ],
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "echo 'Before final response: include changed files, tests run, tests not run, risks, rollback notes.'"
          }
        ]
      }
    ]
  }
}
```

---

## 30. Implementation Roadmap

## 30.1 Milestone 0 — Foundation

Deliver:

```txt
- repo structure
- config loader
- Makefile
- docs skeleton
- CLAUDE.md
- .claude commands/agents/settings
- Docker Compose dev
```

Acceptance:

```txt
- make build works
- make test works
- dev compose boots
```

## 30.2 Milestone 1 — Protocol and KCP Session

Deliver:

```txt
- KCP server
- MessagePack envelope
- Hello/Welcome
- JoinRoom/JoinAccepted
- Ping/Pong
- smoke-kcp
```

Acceptance:

```txt
- Unity-like test client connects
- MessagePack roundtrip works
- invalid version rejected
```

## 30.3 Milestone 2 — Room Manager and Multi-Room

Deliver:

```txt
- logical room id
- room instance id
- room manager
- in-memory registry
- join/leave
- cleanup
```

Acceptance:

```txt
- multiple rooms supported
- no single-room hardcode
- room cleanup tested
```

## 30.4 Milestone 3 — Player Sync

Deliver:

```txt
- player state
- movement input
- spatial hash
- interest set
- FullSnapshot
- PlayerDelta
```

Acceptance:

```txt
- 50 simulated clients move
- no full broadcast in normal tick
- spatial tests pass
```

## 30.5 Milestone 4 — Object Sync and Locking

Deliver:

```txt
- object state
- object command queue
- lease lock
- ObjectDelta
- lock expiration
- disconnect release
```

Acceptance:

```txt
- lock contention tests pass
- race tests pass
- object deltas received only by interested clients
```

## 30.6 Milestone 5 — Voice/Proximity Grouping

Deliver:

```txt
- VoiceGroupAllocator interface
- ProximityVoiceAllocator
- optional KMeansVoiceAllocator
- VoiceGroupDelta
```

Acceptance:

```txt
- max group size enforced
- grouping stable enough for Unity
- config switch works
```

## 30.7 Milestone 6 — Single VPS Production

Deliver:

```txt
- systemd units
- deploy script
- rollback script
- healthcheck
- CodeBuild buildspec
- release layout
```

Acceptance:

```txt
- deploy to VPS succeeds
- rollback works
- healthcheck passes
- KCP smoke test passes
```

## 30.8 Milestone 7 — Load Test and Optimization

Deliver:

```txt
- 50/100/200 CCU scenarios
- movement scenario
- object lock scenario
- packet stats
- benchmark reports
```

Acceptance:

```txt
- capacity measured
- bottlenecks documented
- 200 CCU target validated or remediation planned
```

## 30.9 Milestone 8 — Distributed Scale Skeleton

Deliver:

```txt
- RedisNodeResolver
- RedisRoomRegistry metadata
- heartbeat
- pending room queue
- K3s manifests
- KEDA ScaledObject
- Dockerfiles
- distributed CI buildspec
```

Acceptance:

```txt
- dev compose can run redis mode
- distributed manifests validate
- scale path documented
```

## 30.10 Milestone 9 — Distributed Scale Production

Deliver when infra is available:

```txt
- K3s bootstrap
- ECR pipeline
- Redis deployed
- Gateway deployed
- Game node scale from zero
- Prometheus/Grafana
```

Acceptance:

```txt
- pending room triggers scale
- node heartbeat works
- Gateway assigns room to new node
- scale down idle works
```

---

## 31. What Not to Automate

Do not let Claude or scripts automatically:

```txt
- restart production services without explicit approval
- run terraform apply
- run kubectl apply to production
- push Docker images to production registry
- edit secrets
- migrate live rooms
- change protocol without docs/tests
- claim 200 CCU capacity without measured load test
```

---

## 32. Production Acceptance Checklist

### Shared Core

```txt
[ ] Protocol versioning implemented
[ ] KCP smoke test implemented
[ ] FullSnapshot implemented
[ ] PlayerDelta implemented
[ ] ObjectDelta implemented
[ ] VoiceGroupDelta implemented
[ ] Spatial hashing implemented
[ ] Interest manager implemented
[ ] Object lock manager implemented
[ ] Room loop single-writer rule followed
[ ] Race tests pass
```

### Single VPS

```txt
[ ] Build binary pipeline works
[ ] systemd services installed
[ ] deploy script works
[ ] rollback script works
[ ] healthcheck works
[ ] firewall configured
[ ] logs available
[ ] CPU/memory/network monitored
[ ] 200 CCU load test completed or bottleneck documented
```

### Distributed

```txt
[ ] Redis resolver implemented
[ ] Redis registry metadata implemented
[ ] heartbeat implemented
[ ] pending room queue implemented
[ ] KEDA config exists
[ ] K3s manifests exist
[ ] Dockerfiles exist
[ ] distributed CI/CD exists
[ ] scale-from-zero tested when infra is available
```

---

## 33. Final Architecture Invariants

These should remain true throughout implementation:

```txt
1. One shared realtime core.
2. Two production deployment modes.
3. Development uses Docker Compose.
4. Single VPS production does not require Docker, Redis, or K3s.
5. Distributed production uses Redis, K3s, KEDA, and images.
6. HTTP Gateway handles control plane.
7. KCP handles realtime data plane.
8. MessagePack handles realtime payload.
9. Room loop is the only writer of room state.
10. Spatial hashing is the interest management foundation.
11. Delta broadcast prevents full-room full-state spam.
12. Object locking uses command queue + lease TTL.
13. Voice grouping is pluggable.
14. K-Means is optional, not foundational.
15. Bandwidth and 200 CCU capacity must be measured, not assumed.
```

---

## 34. Recommended First Implementation Order

Start here:

```txt
1. Repo skeleton
2. Config profiles
3. Protocol envelope
4. KCP smoke server/client
5. Gateway /join
6. RoomManager multi-room
7. Player state + spatial hash
8. FullSnapshot + PlayerDelta
9. Object state + lock manager
10. ObjectDelta
11. Loadtest 50 CCU
12. Loadtest 100 CCU
13. Loadtest 200 CCU
14. Single VPS deploy
15. Redis resolver skeleton
16. K3s/KEDA manifests
```

Do not start with:

```txt
- KEDA
- K-Means
- complex autoscaling
- Kubernetes production
- full admin panel
```

The production path is:

```txt
Build core correctly
Ship single VPS safely
Measure limits
Enable distributed mode when infra exists
```
