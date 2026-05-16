# Interest Management

## Phase 1 Focus

Interest management in Phase 1 drives cluster-scoped player delta broadcast.

| Mechanism | Purpose |
|---|---|
| `GridSpatialHash` | Per-tick proximity index. O(1) insert/update/remove. Input data source for K-Means. |
| `InterestManager` | Radius-based visibility query (fallback when cluster disabled). |
| `ClusterAllocator` | Periodic K-Means cluster assignment. Output replaces radius query as primary interest source. |

Phase 1 interest is exclusively player-to-player. Object interest and voice candidate interest are deferred.

## Implementation

Interest management uses spatial hashing as the per-tick proximity index, and K-Means clustering as the periodic grouping policy that drives delta broadcast.

### Components

| Component | Package | Purpose |
|-----------|---------|---------|
| GridSpatialHash | `internal/game/spatial` | Grid-based spatial index for proximity queries |
| InterestManager | `internal/game/interest` | Computes per-client visibility sets using spatial index (fallback path) |
| ClusterAllocator | `internal/game/cluster` | K-Means cluster assignment (primary interest path in Phase 1) |
| Room integration | `internal/game/room` | Maintains spatial index and cluster output synchronized with player state |

### Spatial Hash

The `GridSpatialHash` operates on the XZ ground plane (Y/vertical is ignored). It maps entity positions to grid cells and supports O(1) insert/update/remove and efficient radius queries by scanning only neighboring cells.

Key behaviors:
- **Insert/Update**: `Update(entityID, position)` — inserts or moves an entity. Cell membership is updated automatically.
- **Remove**: `Remove(entityID)` — removes an entity and cleans up empty cells.
- **Query**: `QueryRadius(position, radius)` — returns all entities within the radius. Uses squared-distance comparison to avoid sqrt.
- **Cell size**: Configurable via `SpatialConfig.CellSizeM` (default 10m).

The spatial hash is **not goroutine-safe**. In the room architecture, the room loop is the sole mutator under `sessionMu`. External reads (`NearbyPlayers`, `NearbyPlayersAt`) hold `sessionMu.RLock`.

### Room Integration

The `Room` struct owns a `*spatial.GridSpatialHash` that is kept synchronized with player state:

| Event | Spatial operation |
|-------|-------------------|
| CmdJoin | `Update(playerID, origin)` |
| CmdPlayerInput | `Update(playerID, inputPosition)` |
| CmdUpdatePlayerTransform | `Update(playerID, transformPosition)` |
| CmdLeave | `Remove(playerID)` |
| CmdDisconnect | `Remove(playerID)` |

Public query methods:
- `NearbyPlayers(playerID, radius)` — returns nearby player IDs, excluding self.
- `NearbyPlayersAt(position, radius)` — returns all player IDs near a world position.

### Interest Manager

`InterestManager` wraps spatial queries with configured radii. It is the **fallback path** when `cluster_enabled = false`.

```go
mgr := NewInterestManager(InterestConfig{VisualRadiusM: 30})
set := mgr.QueryVisiblePlayers(spatialIndex, viewerPos, viewerID)
```

Returns an `InterestSet` with `VisiblePlayers` (excluding the viewer).

### K-Means Cluster Interest (Phase 1 Primary Path)

When `cluster_enabled = true`, the broadcast path uses cluster membership instead of a radius query:

```
viewer cluster = ClusterOutput.Assignments[viewerPlayerID]
visible players = ClusterOutput.Clusters[viewer cluster] \ {viewerPlayerID}
```

The `DeltaBuilder` receives this player list and computes enter/update/leave identically to the radius path. The interest source switches; the delta pipeline does not change.

### Interest Selection at Broadcast

```
Broadcast tick:
  if cluster_enabled:
    visible = cluster membership lookup            ← Phase 1 primary path
  else:
    visible = InterestManager.QueryVisiblePlayers  ← fallback path
  DeltaBuilder.BuildPlayerDelta(tick, visible, snapshot, playerStates)
```

### K-Means Cadence and Update Rules

K-Means does not run on every tick. Recompute fires when any of the following conditions are true:

| Trigger | Description |
|---|---|
| Interval | Every `recluster_interval_ticks` ticks (default 10 ticks = 500ms at 20 Hz) |
| Movement | Any player moved > `movement_threshold` meters since last recompute (default 2.0m) |
| Membership change | Player count changed (join or disconnect) since last recompute |

Spatial hash is updated every tick. K-Means reads from spatial hash positions at recompute time.

### Hysteresis / Anti-Flicker Rules

To prevent cluster membership from flickering near centroid boundaries:

- A player is only reassigned if their distance to the new centroid is more than `membership_hysteresis` meters less than their distance to the current centroid (default 5.0m).
- Small positional changes near a boundary do not trigger reassignment.

```
reassign if: dist(player, new_centroid) < dist(player, current_centroid) - membership_hysteresis
```

### Mixed Transport Clients in the Same Cluster

Cluster assignment is transport-agnostic. KCP and WSS clients at similar positions are placed in the same cluster and receive each other's `PlayerDelta` updates.

The `ClusterInput` contains player positions only. Transport type is not an input to K-Means.

### Configuration

```yaml
spatial:
  cell_size_m: 10

interest:
  visual_radius_m: 30      # used by fallback radius path only
  object_radius_m: 30      # deferred / future scope
  voice_radius_m: 30       # deferred / future scope
  full_avatar_radius_m: 30 # deferred / future scope
  low_lod_radius_m: 30     # deferred / future scope

cluster:
  enabled: true
  target_cluster_size: 8
  max_cluster_radius: 30.0
  recluster_interval_ticks: 10
  movement_threshold: 2.0
  membership_hysteresis: 5.0
  max_iterations: 20
```

## Tests

- Spatial hash: insert, update, remove, radius query, cell boundaries, negative coordinates, cross-cell queries, duplicate updates, clear
- Interest manager: visible players, self-exclusion, empty results, multiple nearby
- Room integration: spatial sync on join/leave/disconnect/input, NearbyPlayers, NearbyPlayersAt
- Cluster interest: nearby players in same cluster, far players in different clusters, fallback when disabled, hysteresis, mixed transport players in same cluster

## Hard Rules

- Interest management must be deterministic and testable.
- K-Means output is not the sole source of truth for proximity. The spatial hash remains the ground truth proximity index.
- Cluster computation must not happen in transport goroutines. Only the room loop calls `ClusterAllocator.Compute`.
- Transport type must not affect cluster membership.

## Delta Integration

The interest manager (radius path) and cluster allocator (cluster path) are both called inside `buildDeltaBatches` in the room broadcast path:

```
broadcast(tick) [holds sessionMu.Lock]
  → for each active session:
      resolve visible players (cluster path or radius fallback)
      → DeltaBuilder.BuildPlayerDelta(tick, visiblePlayers, snapshot, playerStates)
```

See `docs/delta-broadcast.md` for the full delta broadcast design.
See `docs/specs/spec_kmeans_cluster_sync.md` for the full cluster allocator spec.

## Deferred / Future Scope

The following interest management features are not Phase 1 implementation targets:

```txt
Object entity tracking in spatial index (EntityObject)    — Deferred / Future Scope
LOD/blue-avatar distance thresholds in InterestManager    — Deferred / Future Scope
Voice candidate radius queries                            — Deferred / Future Scope
Object culling outside object radius                      — Deferred / Future Scope
Per-client bandwidth accounting                           — Deferred / Future Scope
Cross-cluster border zone overlap queries                 — Deferred / Future Scope
```
