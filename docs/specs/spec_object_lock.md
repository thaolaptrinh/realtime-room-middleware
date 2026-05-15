# Spec: Object Locking

> Implementation spec placeholder.

## Scope

Milestone 4 deliverable. ObjectState, command queue, lease TTL, ObjectDelta.

## Key Decisions

- Server-authoritative command queue + lease lock.
- Lock has TTL; expires if not refreshed.
- Disconnect releases all user locks.
- Max locks per user enforced.

## Files

- `internal/game/lock/manager.go`
- `internal/game/lock/manager_test.go`

## Tests Required

- Lock success
- Lock reject when owned
- Expired lock acquired
- Refresh by owner / reject by non-owner
- Release on disconnect
- Max locks per user
