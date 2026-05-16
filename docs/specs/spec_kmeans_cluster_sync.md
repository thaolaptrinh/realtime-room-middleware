# Spec: K-Means Position Cluster Sync

## Status: Skeleton implemented (Stage 2 Task 7) — room integration pending

---

## Goal

Group players into spatial clusters using K-Means on their XZ positions. Use cluster membership as the primary grouping for player delta broadcasts. Players in the same cluster receive each other's `PlayerDelta` updates. Players in different clusters do not.

This delivers cluster-scoped delta broadcast instead of a flat radius query, enabling better bandwidth control and predictable sync group sizes at 200 CCU.

---

## Non-Goals

- K-Means does not replace the spatial hash. The spatial hash remains the authoritative fast-proximity index.
- K-Means does not run on every room tick by default. It runs on a configurable cadence.
- K-Means does not determine physics authority, transport routing, or connection assignment.
- K-Means is not used for voice grouping in Phase 1. Voice grouping is deferred.
- K-Means does not affect object sync or object locking. Those are deferred features.
- K-Means output is not authoritative for proximity visibility. Spatial hash queries remain the ground truth for exact distance checks.

---

## Why K-Means

With 200 players in a room, a flat radius query per player (spatial hash alone) sends deltas to all players within 30m of each other. At high density this can still create large overlap zones and unpredictable per-client delta fan-out.

K-Means partitions players into stable groups by position, giving:

- Predictable cluster size (bounded by `target_cluster_size`).
- Cluster-scoped delta broadcast: a player's transform update is sent only to members of their cluster.
- Natural bandwidth ceiling: cluster size × transform size × broadcast rate is bounded.
- Stable groupings that reduce churn compared to continuously moving radius boundaries.

K-Means is chosen over a fixed-radius proximity filter for sync grouping because:

- Fixed radius at high density can include too many players per viewer.
- K-Means naturally finds natural density groupings without requiring per-viewer fan-out analysis.
- Cluster hysteresis makes membership stable near boundaries, reducing delta enter/leave churn.

---

## Why Spatial Hash Is Still Required

The spatial hash (`GridSpatialHash`) remains required as a fast proximity index because:

- Spatial hash O(1) insert/update/remove and O(neighbors) query is used every tick for player state synchronization.
- K-Means runs on a configurable cadence (not every tick) and has O(k × n × iterations) cost. It cannot replace real-time per-tick proximity lookups.
- Spatial hash provides the input positions for K-Means (`QueryRadius` or full position list).
- Future features (LOD thresholds, object culling, object interest) will query the spatial hash directly, independent of cluster membership.

```
Spatial hash:  per-tick, O(1) insert/update, fast radius query, ground truth proximity
K-Means:       periodic, O(k × n × iter), produces stable cluster assignments
```

Both are required. They serve different roles.

---

## Cluster Input Data

K-Means reads the following at recompute time:

```go
type ClusterInput struct {
    Players []ClusterPlayerEntry
}

type ClusterPlayerEntry struct {
    PlayerID player.PlayerID
    Position spatial.EntityPosition // XZ only, Y ignored
    // Transport type is intentionally absent — clusters are transport-agnostic
}
```

Input is collected by the room loop from the current `PlayerState` map immediately before calling `ClusterAllocator.Compute`. No transport metadata is included. KCP and WSS players are indistinguishable at this level.

---

## Cluster Output Data

K-Means produces a stable cluster assignment map:

```go
type ClusterOutput struct {
    Assignments map[player.PlayerID]ClusterID // player → cluster
    Clusters    map[ClusterID][]player.PlayerID // cluster → members
    Centroids   map[ClusterID]spatial.EntityPosition // cluster center (for diagnostics)
    K           int // number of clusters used
}

type ClusterID uint32
```

The room loop stores `ClusterOutput` and uses it at broadcast time to build per-client interest sets.

---

## Config Parameters

All parameters are part of `RoomConfig` or a nested `ClusterConfig` struct.

| Parameter | Type | Default | Description |
|---|---|---|---|
| `cluster_enabled` | bool | `true` | Enables K-Means cluster sync. If false, falls back to flat radius interest. |
| `target_cluster_size` | int | `8` | Target number of players per cluster. K = ceil(n / target_cluster_size). |
| `max_cluster_radius` | float32 | `30.0` | Maximum distance (meters) from centroid to member. Players beyond this radius are not forced into the cluster — they form their own cluster or join a neighbor. |
| `recluster_interval_ticks` | int | `10` | Ticks between full K-Means recomputes (at 20 Hz tick rate, 10 = 0.5s). |
| `movement_threshold` | float32 | `2.0` | Minimum player movement (meters) since last recompute to trigger an early recompute. Applied to the maximum single-player movement delta since last cluster computation. |
| `membership_hysteresis` | float32 | `5.0` | A player must move at least this many meters past the cluster boundary before being reassigned. Prevents flickering near centroid boundaries. |
| `max_iterations` | int | `20` | Maximum K-Means iterations per recompute. Limits CPU budget. |
| `max_players_per_room` | int | `200` | Upper bound used to pre-allocate cluster data structures. |

YAML config example:

```yaml
cluster:
  enabled: true
  target_cluster_size: 8
  max_cluster_radius: 30.0
  recluster_interval_ticks: 10
  movement_threshold: 2.0
  membership_hysteresis: 5.0
  max_iterations: 20
```

---

## Update Cadence

K-Means does **not** run on every room tick. It runs under one of three conditions:

### 1. Interval trigger

Recompute fires every `recluster_interval_ticks` ticks. At 20 Hz with the default of 10, this is every 500ms. This is the baseline cadence for slow-moving or idle players.

### 2. Movement trigger

If any player's position has moved more than `movement_threshold` meters since the last recompute, recompute fires immediately on the next tick. The room loop tracks `maxMovementSinceLastCluster` as a running maximum. It is reset after each recompute.

### 3. Membership change trigger

If the player count changes (join or disconnect) by any amount since the last recompute, recompute fires on the next tick. This ensures clusters are valid after player churn.

### Recompute budget

Each recompute must complete within the room tick duration budget. At 20 Hz, one tick is 50ms. The cluster computation target is < 5ms for 200 players with `max_iterations=20`. If benchmarks show this is exceeded, reduce `max_iterations` or increase `recluster_interval_ticks`.

The room loop measures cluster computation duration each call and emits it as a metric (`cluster_compute_duration_ms`).

---

## Hysteresis / Stability Rules

To prevent cluster membership from flickering as players move near centroid boundaries:

### Hysteresis threshold

A player is only reassigned to a different cluster if their distance to the new centroid is more than `membership_hysteresis` meters less than their distance to the current centroid. Small positional changes near a boundary do not trigger reassignment.

```
reassign if: dist(player, new_centroid) < dist(player, current_centroid) - membership_hysteresis
```

### Assignment priority

On full recompute (not hysteresis check), standard K-Means nearest-centroid assignment applies. The hysteresis check is applied as a post-pass before the room loop commits the new assignment.

### Minimum delta for delta broadcast

The `PlayerDelta` already suppresses no-change updates via version comparison in `DeltaBuilder`. Cluster membership changes (enter/leave events) are limited by hysteresis, so transport-level enter/leave churn is bounded even at cluster boundaries.

---

## Cluster Membership Semantics

- Each player belongs to exactly one cluster at any time.
- A player with no neighbors forms a cluster of size 1 (singleton cluster).
- The number of clusters K is computed as `ceil(n / target_cluster_size)`, clamped to `[1, n]`.
- Players beyond `max_cluster_radius` of all centroids are assigned to the nearest centroid regardless of radius (fallback rule: no player is ever unassigned).
- Cluster IDs are local to the room instance. They are not stable across recomputes; the room loop rebinds IDs after each recompute.

---

## Delta Broadcast Integration

Cluster membership replaces flat radius interest for Phase 1 player delta broadcast.

### Interest set construction

At broadcast tick, for each active session:

```
viewer cluster = ClusterOutput.Assignments[viewerPlayerID]
visible players = ClusterOutput.Clusters[viewer cluster] \ {viewerPlayerID}
```

This replaces `InterestManager.QueryVisiblePlayers(spatialIndex, viewerPos, viewerID)` used in the current spatial-only broadcast path.

The `DeltaBuilder.BuildPlayerDelta` call is unchanged. It receives the visible player list and computes enter/update/leave against `ClientSnapshotCache`.

### Fallback when cluster disabled

If `cluster_enabled = false`, the broadcast path falls back to the existing `InterestManager.QueryVisiblePlayers` radius query. The delta builder is identical in both paths.

### Relationship to existing delta pipeline

```
Broadcast tick:
  if cluster_enabled:
    visible = ClusterOutput.Clusters[viewerCluster] \ {self}
  else:
    visible = InterestManager.QueryVisiblePlayers(spatial, viewerPos, viewerID)

  DeltaBuilder.BuildPlayerDelta(tick, visible, snapshot, playerStates)
  → PlayerDelta{Enters, Updates, Leaves}
  → encode MessagePack
  → RealtimeSession.WritePacket (KCP or WSS per session)
```

The cluster output is read-only at broadcast time. Only the room loop writes cluster assignments.

---

## Mixed KCP/WSS Behavior

Cluster allocation is transport-agnostic. The `ClusterInput` does not include transport type. K-Means computes clusters purely from player positions.

A KCP client and a WSS client at the same position will be placed in the same cluster and will receive each other's `PlayerDelta` updates. Transport type has no effect on cluster membership.

Delta packets for KCP sessions are sent via `KCPSession.WritePacket`. Delta packets for WSS sessions are sent via `WSSSession.WritePacket`. The encoded MessagePack payload is identical in both cases.

---

## Files

To be created during implementation:

```
internal/game/cluster/types.go       — ClusterID, ClusterInput, ClusterOutput, ClusterConfig
internal/game/cluster/kmeans.go      — KMeansClusterAllocator implementing ClusterAllocator
internal/game/cluster/allocator.go   — ClusterAllocator interface
internal/game/cluster/kmeans_test.go — unit tests
```

Room integration:

```
internal/game/room/types.go          — ClusterConfig in RoomConfig
internal/game/room/room.go           — ClusterOutput field, recompute triggers
internal/game/room/tick.go           — movement tracking, cluster recompute scheduling
internal/game/room/broadcast.go      — cluster-based interest set at broadcast tick
```

---

## Tests Required

### Unit tests — ClusterAllocator

- Single player → one singleton cluster
- Two nearby players → same cluster
- Two far players → separate clusters
- Players exceeding `target_cluster_size` → multiple clusters created
- K computed correctly from player count and target size
- Hysteresis: player near boundary not reassigned on small movement
- Hysteresis: player past threshold reassigned
- `max_iterations` limit respected (computation terminates)
- Empty input → zero clusters, no panic
- All players at the same position → one cluster
- Players beyond `max_cluster_radius` are still assigned (fallback rule)
- Deterministic output for same input (centroid initialization must be seeded consistently for tests)
- KCP and WSS player entries produce identical cluster assignments (transport is absent from input)

### Unit tests — Room integration

- Cluster recomputes on join
- Cluster recomputes on disconnect
- Cluster recomputes when movement threshold exceeded
- Cluster does not recompute on interval when no movement
- Cluster recomputes at interval even without movement
- Cluster output drives broadcast interest set (cluster-scoped visible players)
- Broadcast with `cluster_enabled=false` uses radius interest fallback
- Cluster computation duration is measurable (hook for metric emission)

### Integration tests

- KCP client and WSS client in same cluster receive each other's PlayerDelta
- Player leaves cluster → Leave delta emitted to former cluster members
- Player joins room → Enter delta emitted to cluster members
- Player moves far enough → cluster reassignment and Leave/Enter delta round-trip
- No full-room broadcast during normal cluster-based tick

---

## Load Test Requirements

All load tests use the existing KCP/WSS client scaffolding.

| Scenario | CCU | Expectation |
|---|---|---|
| Cluster sync — static players | 200 | CPU < 40%, cluster computation < 5ms per recompute |
| Cluster sync — random movement | 200 | CPU < 75%, bandwidth stable, no goroutine leak |
| Cluster sync — dense zone | 200 near origin | Multiple clusters formed, no single 200-player cluster |
| Cluster sync — movement storm | 200 all moving | Recompute triggers frequently, interval cadence respected |
| Mixed transport | 100 KCP + 100 WSS | KCP and WSS clients in same clusters, same delta content |

Metrics to capture during load test:

```
cluster_compute_duration_ms  (p50, p95, p99)
cluster_count                (number of active clusters)
cluster_size_avg             (average players per cluster)
bytes_per_client_per_second  (delta bandwidth)
room_tick_duration_ms        (must not grow with cluster computation)
```

---

## Acceptance Criteria

- [ ] Nearby players are grouped into the same cluster.
- [ ] Far players (beyond `max_cluster_radius`) are not grouped together.
- [ ] Cluster membership changes when a player moves far enough (movement threshold + hysteresis).
- [ ] Small movements do not cause cluster flicker (hysteresis enforced).
- [ ] KCP and WSS clients can be in the same cluster.
- [ ] Cluster output is deterministic for the same input (centroid seeding is stable for tests).
- [ ] Cluster computation does not block the room tick beyond budget (< 5ms at 200 players, 20 iterations).
- [ ] Broadcast interest set is cluster-scoped (not flat radius) when `cluster_enabled=true`.
- [ ] `cluster_enabled=false` falls back to radius interest without error.
- [ ] No full-room full-state broadcast during normal cluster-based tick.
- [ ] Race detector passes with cluster computation in room loop.

---

## Deferred Future Scope

The following are explicitly out of scope for Phase 1 K-Means cluster sync:

```txt
Voice grouping:
- VoiceGroupAllocator may reuse position clusters in a future phase.
- K-Means cluster IDs will NOT be directly reused as voice group IDs without a separate design.
- Deferred / Future Scope

Object sync:
- Object interest management will use a separate spatial query (object radius), not cluster membership.
- Deferred / Future Scope

Object locking:
- Unrelated to position cluster sync.
- Deferred / Future Scope

LOD / blue-avatar thresholds:
- InterestManager LOD distance tiers are deferred.
- Cluster membership does not currently encode LOD information.
- Deferred / Future Scope

Per-client bandwidth accounting:
- Not implemented in Phase 1.
- Cluster size bounding gives indirect bandwidth control.
- Deferred / Future Scope

Cross-cluster visibility (transitional zone):
- Players at cluster boundaries may miss near-boundary players in adjacent clusters.
- Mitigation via hysteresis and `max_cluster_radius` tuning.
- A "border zone" overlap query (spatial hash + cluster membership combined) is a future optimization.
- Deferred / Future Scope
```

---

## Implementation Status (Stage 2 Task 7)

The cluster allocator skeleton is implemented:

- `ClusterAllocator` interface — `internal/game/cluster/allocator.go`
- `KMeansClusterAllocator` — `internal/game/cluster/kmeans.go`
- Domain types and config — `internal/game/cluster/types.go`
- Unit tests — `internal/game/cluster/kmeans_test.go`

Not yet implemented (intentionally deferred to next tasks):

- `ClusterConfig` is not in `RoomConfig` — room integration is the next task.
- The room loop does not call `ClusterAllocator.Compute` — deferred to room integration.
- The broadcast path still uses `InterestManager.QueryVisiblePlayers` (radius query) — deferred.
- No cluster metrics are emitted — deferred.

Hysteresis skeleton note: centroids in the output reflect K-Means convergence, not
post-hysteresis membership. The prevCentroid used in the next recompute is the K-Means
centroid, which may drift slightly from the actual hysteresis-adjusted cluster position.
A future optimisation may recompute centroids after the hysteresis pass.

---

## Reference

- `docs/architecture.md` — Position Cluster Sync Architecture section
- `docs/interest-management.md` — existing spatial hash and interest manager
- `docs/delta-broadcast.md` — existing delta broadcast pipeline
- `docs/specs/spec_spatial_hash.md` — spatial hash implementation detail
- `docs/full_production_architecture_workflow_blueprint.md` — Section 30 Phase 1 Gameplay Focus
