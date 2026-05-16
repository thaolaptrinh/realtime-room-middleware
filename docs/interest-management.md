# Interest Management

## Implementation

Interest management uses spatial hashing to determine which entities are visible to each client.

### Components

| Component | Package | Purpose |
|-----------|---------|---------|
| GridSpatialHash | `internal/game/spatial` | Grid-based spatial index for proximity queries |
| InterestManager | `internal/game/interest` | Computes per-client visibility sets using spatial index |
| Room integration | `internal/game/room` | Maintains spatial index synchronized with player state |

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

`InterestManager` wraps spatial queries with configured radii:

```go
mgr := NewInterestManager(InterestConfig{VisualRadiusM: 30})
set := mgr.QueryVisiblePlayers(spatialIndex, viewerPos, viewerID)
```

Returns an `InterestSet` with `VisiblePlayers` (excluding the viewer).

### Configuration

```yaml
spatial:
  cell_size_m: 10

interest:
  visual_radius_m: 30
  object_radius_m: 30
  voice_radius_m: 30
  full_avatar_radius_m: 30
  low_lod_radius_m: 30
```

All radius values are independently configurable even though they default to 30m.

## Tests

- Spatial hash: insert, update, remove, radius query, cell boundaries, negative coordinates, cross-cell queries, duplicate updates, clear
- Interest manager: visible players, self-exclusion, empty results, multiple nearby
- Room integration: spatial sync on join/leave/disconnect/input, NearbyPlayers, NearbyPlayersAt

## Hard Rules

- Interest management must be deterministic and testable.
- K-Means must not be the only source of truth for visibility.

## Future (not yet implemented)

- Object entity tracking in spatial index
- LOD/blue-avatar distance thresholds
- Voice candidate queries
- Per-client snapshot caches and delta computation
- Object culling outside object radius
