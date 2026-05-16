# Spec: Spatial Hash and Interest Management

## Status: Implemented (Milestone 3)

## Scope

GridSpatialHash, InterestManager, room integration for player proximity queries.

## Implementation Summary

### Spatial Hash (`internal/game/spatial`)

- **GridSpatialHash**: Grid-based spatial index operating on the XZ ground plane.
- **EntityID**: String type, compatible with `player.PlayerID` via conversion.
- **EntityPosition**: 2D position (X, Z) — Y (vertical) is ignored for proximity.
- **CellCoord**: Grid cell identifier computed via `Floor(position / cellSize)`.
- **SpatialConfig**: Cell size in meters (default 10m).

Operations:
- `Update(id, pos)` — insert or move entity, auto-updates cell membership.
- `Remove(id)` — remove entity, cleans up empty cells.
- `Get(id)` — read entity position.
- `QueryRadius(pos, radius)` — returns all entities within radius using squared-distance comparison.
- `Clear()` — remove all entities.
- `Len()` — entity count.

Not goroutine-safe. Room loop is the sole mutator under `sessionMu`.

### Interest Manager (`internal/game/interest`)

- **InterestConfig**: Five configurable radii (visual, object, voice, full-avatar, low-LOD), all default 30m.
- **InterestManager**: Wraps spatial queries with configured visual radius, excludes viewer from results.
- **InterestSet**: Contains `VisiblePlayers` slice.

### Room Integration (`internal/game/room`)

- Room struct has `spatial *spatial.GridSpatialHash` field.
- `RoomConfig.SpatialCellSizeM` (default 10m) flows through RoomManager → RoomSpec → newRoom.
- Spatial index is synchronized with player state on every command:
  - CmdJoin → `Update(playerID, origin)`
  - CmdPlayerInput → `Update(playerID, inputPosition)`
  - CmdUpdatePlayerTransform → `Update(playerID, transformPosition)`
  - CmdLeave → `Remove(playerID)`
  - CmdDisconnect → `Remove(playerID)`
- `NearbyPlayers(playerID, radius)` — thread-safe read, excludes self.
- `NearbyPlayersAt(position, radius)` — thread-safe read, returns all players near position.

## Files

- `internal/game/spatial/types.go` — EntityID, EntityKind, EntityPosition, CellCoord, SpatialConfig
- `internal/game/spatial/hash.go` — GridSpatialHash implementation
- `internal/game/spatial/hash_test.go` — unit tests + benchmarks
- `internal/game/interest/manager.go` — InterestConfig, InterestSet, InterestManager
- `internal/game/interest/manager_test.go` — interest manager tests
- `internal/game/room/types.go` — RoomConfig.SpatialCellSizeM
- `internal/game/room/room.go` — spatial field, NearbyPlayers, NearbyPlayersAt
- `internal/game/room/tick.go` — spatial sync in handleCommand
- `internal/game/room/room_test.go` — spatial integration tests

## Tests Implemented

- Spatial hash: insert, update same cell, update cross-cell, remove, remove nonexistent, query radius (includes/excludes/boundary), zero radius, cross-cell query, negative coordinates, cell boundary math, duplicate update determinism, clear, get, many entities (200), empty cell cleanup, distance precision
- Interest manager: visible players, self-exclusion, no one nearby, multiple nearby, default config, empty index
- Room integration: spatial insert on join, remove on leave, remove on disconnect, update on player input, update on direct transform, NearbyPlayers, NearbyPlayersExcludesSelf, NearbyPlayersNonexistentPlayer, NearbyPlayersAt, custom cell size config

## Key Decisions

- Grid-based spatial hash with configurable cell size.
- Query neighboring cells, filter by exact squared distance (no sqrt).
- XZ plane only — Y (vertical) is ignored for proximity.
- Not goroutine-safe by design — room loop synchronizes via sessionMu.
- InterestManager excludes viewer from results.
- InterestSet only has VisiblePlayers for now; objects and voice candidates deferred.

## Future (not yet implemented)

- Object entity tracking in spatial index (EntityObject)
- LOD/blue-avatar distance thresholds in InterestManager
- Voice candidate radius queries
- Per-client snapshot caches and delta broadcast integration
- Object culling outside object radius
