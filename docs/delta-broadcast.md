# Delta Broadcast

## Status: Skeleton implemented (Milestone 3 / Stage 2 Task 5)

Object delta, voice group delta, and transport send are deferred to later milestones.

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
    // Future: ObjectDelta, VoiceGroupDelta
}
```

`DeltaBatch.IsEmpty()` returns true if `PlayerDelta` is nil or empty.

## Dirty Tracking

The room marks a player as dirty (in `r.dirtyPlayers`) when `CmdPlayerInput` or `CmdUpdatePlayerTransform` updates their transform. The dirty set is cleared after each broadcast tick.

Dirty tracking exists as a foundation for future optimization (e.g., skip delta computation when no players in an interest range are dirty). The delta builder itself already handles no-change suppression via version comparison.

## Broadcast Rate

Broadcast runs at `RoomConfig.BroadcastRateHz` (default 10 Hz), which is a sub-rate of `TickRateHz` (default 20 Hz). For TickRate=20 / BroadcastRate=10, broadcast fires every 2nd tick.

```
Room tick (20 Hz):
  drain command queue
  [every 2nd tick] broadcast(tick):
    sessionMu.Lock
    buildDeltaBatches — interest query + delta computation per session
    clearDirtyPlayers
    sessionMu.Unlock
    [batches discarded — transport send is a future milestone]
```

## Transport Separation

- `DeltaBuilder` has no transport-specific fields or logic.
- `DeltaBatch` has no KCP/WebSocket metadata.
- The delta builder outputs domain data only; encoding and sending are deferred.
- Transport packages must not import `internal/game`.
- Native and WebGL clients receive semantically identical deltas.

## Interest Integration

`buildDeltaBatches` uses `InterestManager.QueryVisiblePlayers` (from `internal/game/interest`) to determine each viewer's visible player set. The spatial hash provides the proximity query. Both calls happen under `sessionMu.Lock` inside the room loop.

## Hard Rules

- No full-room full-state broadcast during normal operation.
- No transport-specific fields in delta types.
- No object or voice delta yet (deferred to later milestones).
- No Redis/KEDA dependency.

## Not Yet Implemented

- MessagePack encoding of delta packets (deferred)
- Transport send (KCP or WSS) of encoded packets (deferred)
- `ObjectDelta` and `ObjectEnterDelta/ObjectUpdateDelta/ObjectLeaveDelta` (Milestone 4)
- `VoiceGroupDelta` (Milestone 5)
- LOD/blue-avatar thresholds in interest set (deferred)
- Full snapshot fallback on join/reconnect (deferred — FullSnapshot is a separate message type)
- Per-client bandwidth accounting (deferred)

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
