# Delta Broadcast

## Phase 1 Focus

Phase 1 delta broadcast covers player position sync only.

| In scope | Status |
|---|---|
| PlayerEnterDelta | Implemented (skeleton) |
| PlayerUpdateDelta | Implemented (skeleton) |
| PlayerLeaveDelta | Implemented (skeleton) |
| Cluster-based interest (replaces radius query as primary path) | **Implemented (Stage 2 Task 9)** |
| Position dirty tracking | Implemented |
| Per-client snapshot cache | Implemented |
| Protocol wire structs and MessagePack roundtrip for PlayerDelta | Implemented (Stage 2 Task 10) |
| Transport send (KCP/WSS) of PlayerDelta | Implemented (Stage 2 Task 15 skeleton) |

Deferred from Phase 1:

| Feature | Status |
|---|---|
| ObjectDelta | Deferred / Future Scope |
| ObjectLockDelta | Deferred / Future Scope |
| VoiceGroupDelta | Deferred / Future Scope |

## Status: Cluster-based delta building and dispatch integrated (Stage 2 Task 15)

Player delta skeleton is complete. Cluster-based interest management is integrated with delta building, and the room loop can dispatch encoded `PlayerDelta` packets through the app-layer bridge when a broadcaster is installed.

- `Room.buildDeltaBatches` uses `ClusterOutput` to determine visible players when `cluster_enabled=true`
- Falls back to `InterestManager` radius query when `cluster_enabled=false`
- `Room.broadcast` calls `room.Broadcaster.BroadcastDelta` after building per-session batches
- `internal/app/realtime.RoomDeltaBroadcaster` adapts room session IDs to `internal/game/bridge.DeltaBroadcaster`
- The bridge encodes MessagePack Protocol v1 `PlayerDelta` envelopes and sends them through `transport.RealtimeSession`
- Object delta placeholder types are defined but not yet wired into interest management or transport send
- Voice group delta is deferred

## Components

| Component | Package | Purpose |
|-----------|---------|---------|
| DeltaBuilder | `internal/game/delta` | Computes per-client PlayerDelta from interest set and snapshot |
| ClientSnapshot | `internal/game/delta` | Per-session last-sent state (PlayerID → version) |
| SnapshotCache | `internal/game/delta` | Room-owned map of SessionID → ClientSnapshot |
| Room broadcast | `internal/game/room` | Calls DeltaBuilder at broadcast rate; clears dirty state |

## Per-Client Snapshot Cache

Each session has a `ClientSnapshot` in the room's `SnapshotCache`:

```go
type ClientSnapshot struct {
    VisiblePlayers map[player.PlayerID]uint32 // playerID → last-sent version
}
```

The snapshot is created on `CmdJoin` and removed on `CmdLeave` or `CmdDisconnect`.

Only the room loop reads and writes `ClientSnapshot` and `SnapshotCache`. Not goroutine-safe by design.

## Delta Semantics

For each broadcast tick, `DeltaBuilder.BuildPlayerDelta` compares the current interest set against the viewer's snapshot:

| Case | Delta type | Action |
|------|-----------|--------|
| Player in visible set, not in snapshot | Enter | Add full transform; add to snapshot |
| Player in visible set, version changed | Update | Send new transform; update snapshot version |
| Player in visible set, version same | — | No entry (no change) |
| Player in snapshot, not in visible set | Leave | Emit leave; remove from snapshot |

The snapshot is updated in place as part of the same `BuildPlayerDelta` call.

## PlayerDelta

```go
type PlayerDelta struct {
    Tick    uint32
    Enters  []PlayerEnterDelta  // players newly visible
    Updates []PlayerUpdateDelta // visible players with changed transforms
    Leaves  []PlayerLeaveDelta  // players that left the interest range
}
```

`PlayerDelta.IsEmpty()` returns true when all three slices are empty. The room skips no-op batches.

## DeltaBatch

```go
type DeltaBatch struct {
    Tick        uint32
    PlayerDelta *PlayerDelta
    ObjectDelta *ObjectDelta // nil until object transport wiring (Milestone 4)
    // Future: VoiceGroupDelta
}
```

`DeltaBatch.IsEmpty()` returns true if all contained deltas are nil or empty.

## Dirty Tracking

The room marks a player as dirty (in `r.dirtyPlayers`) when `CmdPlayerInput` or `CmdUpdatePlayerTransform` updates their transform. The dirty set is cleared after each broadcast tick.

Dirty tracking exists as a foundation for future optimization (e.g., skip delta computation when no players in an interest range are dirty). The delta builder itself already handles no-change suppression via version comparison.

## Broadcast Rate

Broadcast runs at `RoomConfig.BroadcastRateHz` (default 10 Hz), which is a sub-rate of `TickRateHz` (default 20 Hz). For TickRate=20 / BroadcastRate=10, broadcast fires every 2nd tick.

```
Room tick (20 Hz):
  drain command queue
  update player transform state (position, rotation, animation state)
  update spatial hash
  [on cluster recompute trigger]: run ClusterAllocator.Compute → update ClusterOutput
  [every 2nd tick] broadcast(tick):
    sessionMu.Lock
    buildDeltaBatches — cluster interest (or radius fallback) + delta computation per session
    clearDirtyPlayers
    sessionMu.Unlock
    if broadcaster is installed: dispatch batches to RealtimeSession (KCP or WSS)
```

## Transport Separation

- `DeltaBuilder` has no transport-specific fields or logic.
- `DeltaBatch` has no KCP/WebSocket metadata.
- The delta builder outputs domain data only; encoding and sending happen in the app/bridge layer.
- Transport packages must not import `internal/game`.
- Native (KCP) and WebGL (WSS) clients receive semantically identical `PlayerDelta` payloads.

## Stage 2 Task 15 Skeleton Flow

The E2E skeleton keeps the runtime boundaries explicit:

```txt
receive adapter
  -> realtime packet handler
  -> room command queue
  -> room tick
  -> cluster/interest delta build
  -> app-layer bridge dispatch
  -> fake KCP/WSS sessions receive MessagePack Protocol v1 PlayerDelta payloads
```

This is not a full production network loop. It verifies the receive-to-dispatch path with fake sessions while preserving the rule that transport packages do not import game packages.

## Interest Integration

Phase 1 primary path: `buildDeltaBatches` uses cluster membership from `ClusterOutput` to determine each viewer's visible player set when `cluster_enabled = true`.

Fallback path: `InterestManager.QueryVisiblePlayers` (radius query) is used when `cluster_enabled = false`.

Both paths feed `DeltaBuilder.BuildPlayerDelta` with the same interface — a slice of visible player IDs. The delta builder is interest-source-agnostic.

```
broadcast(tick) [holds sessionMu.Lock]
  → for each active session:
      if cluster_enabled:
        visiblePlayers = ClusterOutput.Clusters[viewerCluster] \ {viewerID}
      else:
        visiblePlayers = interestMgr.QueryVisiblePlayers(r.spatial, viewerPos, viewerID)
      → DeltaBuilder.BuildPlayerDelta(tick, visiblePlayers, snapshot, playerStates)
```

## Hard Rules

- No full-room full-state broadcast during normal operation.
- No transport-specific fields in delta types.
- No object delta or voice delta in Phase 1. Both are deferred.
- No Redis/KEDA dependency.
- Cluster computation must not happen inside `buildDeltaBatches`. The cluster output is computed by the room loop tick and is read-only during broadcast.

## Object Delta (Placeholder — Deferred / Future Scope)

Types are defined in `internal/game/delta/types.go`:

```go
type ObjectEnterDelta   // object newly visible
type ObjectUpdateDelta  // transform or lock changed; nil pointer fields = no change
type ObjectLeaveDelta   // object left interest range
type ObjectLockDelta    // lock granted / released / expired (may fire outside normal broadcast)
type ObjectDelta        // aggregates all of the above for one tick
```

Not wired to interest management, snapshot cache, or transport send. Not a Phase 1 implementation target.

## Not Yet Implemented — Phase 1 Remaining

Phase 1 items not yet implemented:

```txt
- FullSnapshot on join/reconnect (deferred — FullSnapshot is a separate message type)
- Production transport loop wiring for live KCP/WSS sessions beyond the current skeleton
```

Protocol wire structs and codec roundtrip coverage for `PlayerDelta`, `PlayerEnterDelta`, `PlayerUpdateDelta`, `PlayerLeaveDelta`, and `FullSnapshot` were added in Stage 2 Task 10. Stage 2 Task 15 added the skeleton room-loop-to-bridge dispatch path for `PlayerDelta`.

ClusterAllocator and cluster-based delta building completed in Stage 2 Tasks 7-9:

```txt
internal/game/cluster/types.go       — ClusterID, ClusterInput, ClusterOutput, ClusterConfig
internal/game/cluster/allocator.go   — ClusterAllocator interface
internal/game/cluster/kmeans.go      — KMeansClusterAllocator
internal/game/cluster/kmeans_test.go — unit tests
internal/game/room/room.go           — ClusterConfig in RoomConfig, cluster integration in buildDeltaBatches
internal/game/room/cluster_delta_test.go — cluster-based delta building tests
```

## Deferred / Future Scope

The following are not Phase 1 implementation targets:

```txt
ObjectDelta:
- Object interest management integration (spatial hash for objects)
- Object snapshot cache (per-session last-seen object version)
- ObjectLockDelta
- Deferred / Future Scope

VoiceGroupDelta:
- VoiceGroupAllocator integration
- Voice snapshot cache
- Deferred / Future Scope

LOD/blue-avatar thresholds in interest set    — Deferred / Future Scope
Per-client bandwidth accounting               — Deferred / Future Scope
```

## Files

```
internal/game/delta/doc.go        — package documentation
internal/game/delta/types.go      — DeltaType, PlayerEnterDelta, PlayerUpdateDelta,
                                    PlayerLeaveDelta, PlayerDelta, DeltaBatch
internal/game/delta/snapshot.go   — ClientSnapshot, SnapshotCache
internal/game/delta/builder.go    — DeltaBuilder
internal/game/delta/delta_test.go — unit tests
```

## Tests

- DeltaBuilder: initial snapshot emits all enters, enter on new player, update on version change, leave on exit from interest, no-op when unchanged, far player excluded, missing state skipped, multiple leaves, version tracking, transport-agnostic types
- SnapshotCache: GetOrCreate creates once, returns same pointer, Remove cleans up, Len tracks count
- Room integration: snapshot created on join, removed on leave/disconnect, dirty count updated on transform, multi-session tracking, broadcast runs without panic
