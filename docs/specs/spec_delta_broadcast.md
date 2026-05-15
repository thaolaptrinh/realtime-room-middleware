# Spec: Delta Broadcast

> Implementation spec placeholder.

## Scope

Milestone 3 deliverable. Per-client snapshot cache, delta semantics, FullSnapshot.

## Key Decisions

- ClientSnapshotCache tracks last-sent version per entity.
- Enter: entity newly visible → compact snapshot.
- Update: entity still visible, version changed → changed fields.
- Leave: entity no longer visible → hide/remove.
- No full-room broadcast during normal ticks.

## Files

- `internal/game/delta/broadcaster.go`
- `internal/game/delta/cache.go`
- `internal/game/delta/broadcaster_test.go`

## Tests Required

- Enter delta
- Update delta
- Leave delta
- No-change produces no packet
- Full snapshot fallback
