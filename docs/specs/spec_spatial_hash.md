# Spec: Spatial Hash and Interest Management

> Implementation spec placeholder.

## Scope

Milestone 3 deliverable. GridSpatialHash, InterestManager, per-client interest sets.

## Key Decisions

- Grid-based spatial hash with configurable cell size.
- Query neighboring cells, filter by exact distance.
- InterestSet contains visible players, objects, voice candidates.
- Must be deterministic and testable.

## Files

- `internal/game/spatial/hash.go`
- `internal/game/spatial/hash_test.go`
- `internal/game/interest/manager.go`
- `internal/game/interest/manager_test.go`

## Tests Required

- Player enter/leave cell
- Boundary positions
- Query radius correctness
- Visible within radius / hidden outside
- Object culling outside radius
