# Object Synchronization and Locking

## Status: Skeleton implemented (Milestone 4 / Stage 2 Task 6)

Object state, ObjectManager, and LockManager are implemented.
Full object sync (interest management, ObjectDelta broadcast, transport send) is deferred to Milestone 4.

## Components

| Component | Package | Purpose |
|-----------|---------|---------|
| ObjectState | `internal/game/object` | Authoritative runtime record for a room object |
| ObjectManager | `internal/game/object` | Create, get, list, update, remove objects |
| LockManager | `internal/game/object` | Acquire, refresh, release, expire, disconnect-release locks |
| Room | `internal/game/room` | Owns ObjectManager and LockManager; dispatches object commands |

## Object State Model

```go
type ObjectState struct {
    ID          ObjectID
    Kind        ObjectKind
    Transform   ObjectTransform  // Position (Vec3) + Rotation (Quat)
    CustomState []byte           // Arbitrary serialized state; opaque to the lock manager
    Status      ObjectStatus     // Active or Inactive
    Lock        LockState        // Current lock info (LockedBy, SessionID, LockUntil)
    Version     uint32           // Incremented on every state or lock change
}
```

`ObjectManager` and `LockManager` are not goroutine-safe.
Both are accessed exclusively from the room loop goroutine, protected by `sessionMu` for external readers.

## Command Queue + Lease TTL Consistency Model

Chosen model: **server-authoritative command queue + lease lock**.

- Client sends intent (CmdObjectLockAcquire / CmdObjectLockRefresh / CmdObjectLockRelease).
- Room loop is the sole decision maker.
- Transport goroutines enqueue commands; they never call ObjectManager or LockManager directly.

## Lock Flows

### Acquire

```
Client sends CmdObjectLockAcquire
Room loop (handleCommand):
  - Object must exist and be active
  - Object must be unlocked or lock expired (cleaned by ReleaseExpired)
  - Owner must be below MaxLocksPerUser (default 3)
  → Lock granted: LockState set, Version++, userLockCount++
  → Lock rejected: reason logged; no state change
Special case: if the owner already holds the active lock, TTL is extended (re-acquire = refresh)
```

### Refresh

```
Client sends CmdObjectLockRefresh
Room loop:
  - Object must exist
  - Lock must be active and owned by the requester
  → LockUntil extended by TTL
  → Version NOT incremented (TTL extension is not a state change)
```

### Release

```
Client sends CmdObjectLockRelease
Room loop:
  - Object must exist
  - Lock must be active and owned by the requester
  → LockState cleared (LockedBy = "", LockUntil = zero)
  → Version++
  → userLockCount--
```

### Expiration

```
Room tick (every tick, before drainCommands):
  releaseExpiredLocks()
    → LockManager.ReleaseExpired(now)
    → For each object: if LockedBy != "" && LockUntil <= now → clearLock
    → userLockCount decremented
```

Expired locks are cleared before command processing on the same tick. This ensures
`CmdObjectLockAcquire` handlers see clean lock state without needing to handle partial expiry.

### Disconnect / Leave

```
CmdDisconnect or CmdLeave:
  → LockManager.ReleaseBySession(sessionID, now)
  → All locks held by that session are cleared
  → Version++ for each affected object
  → userLockCount decremented
```

## Lock Configuration

Default values (from `object.DefaultLockLease()`):

```yaml
object_lock:
  lease_ttl:           10s   # Time before a lock expires without refresh
  max_locks_per_user:  3     # Maximum concurrent locks per user
```

`RoomConfig.ObjectLockLease` holds these values.

## Room Commands

| Command Kind | Payload | Handler action |
|---|---|---|
| `CmdObjectLockAcquire` | `ObjectCommandPayload{ObjectID}` | `LockManager.AcquireLock` |
| `CmdObjectLockRefresh` | `ObjectCommandPayload{ObjectID}` | `LockManager.RefreshLock` |
| `CmdObjectLockRelease` | `ObjectCommandPayload{ObjectID}` | `LockManager.ReleaseLock` |
| `CmdObjectUpdate` | `ObjectCommandPayload{ObjectID, Transform, CustomState}` | `ObjectManager.UpdateTransform / UpdateCustomState` |

## Object Creation

Objects are created server-side via `Room.CreateObject(id, kind, transform)`.
This is for server initialization only — clients cannot create room objects directly.

```go
if err := room.CreateObject("chair-1", "chair", object.ObjectTransform{}); err != nil {
    // handle
}
```

`CreateObject` holds `sessionMu.Lock()` for goroutine safety.

## Hard Rules

- Server-authoritative command queue + lease lock.
- No optimistic locking.
- Locks expire on TTL (enforced each tick).
- Disconnect releases all user locks.
- ObjectManager and LockManager are accessed only from the room loop (protected by sessionMu).
- Transport packages must not import `internal/game/object`.

## Not Yet Implemented

- `ObjectDelta` broadcasting (enter/update/leave/lock per viewer interest set)
- `LockAccepted` / `LockRejected` MessagePack responses to client
- Object interest management integration (spatial hash query for objects)
- Object radius / proximity check before granting lock
- FullSnapshot includes object state
- Per-client object snapshot cache
- ObjectStatus inactive propagation to delta

## Files

```
internal/game/object/doc.go         — package documentation
internal/game/object/types.go       — ObjectID, ObjectKind, ObjectStatus, ObjectTransform,
                                      Vec3, Quat, LockState, LockLease, ObjectState
internal/game/object/manager.go     — ObjectManager (CRUD)
internal/game/object/lock.go        — LockResult, LockManager (acquire/refresh/release/expire/disconnect)
internal/game/object/object_test.go — unit tests for ObjectManager and LockManager
```

## Tests

### ObjectManager unit tests
- Create object
- Create duplicate rejected
- Get existing / not found
- List all
- UpdateTransform increments version
- UpdateTransform not found returns error
- MarkInactive (removed from ActiveObjects, still in Count)
- Remove deletes entirely
- ActiveObjects filters correctly

### LockManager unit tests
- AcquireLock succeeds
- AcquireLock rejected when locked by another user
- AcquireLock same owner extends TTL (re-acquire = refresh, no double-count)
- AcquireLock rejected when MaxLocksPerUser reached
- AcquireLock rejected for nonexistent object
- AcquireLock rejected for inactive object
- RefreshLock succeeds for owner
- RefreshLock rejected for non-owner
- RefreshLock rejected after expiry
- ReleaseLock succeeds for owner
- ReleaseLock rejected for non-owner
- ReleaseExpired clears all expired locks
- ReleaseExpired clears only expired locks (partial expiry test)
- ReleaseBySession clears session locks, leaves other sessions intact
- Lock expiry allows new owner to acquire

### Room integration tests
- CreateObject visible via ObjectCount
- CmdObjectLockAcquire granted through room loop
- CmdObjectLockRelease released through room loop
- CmdObjectLockAcquire rejected for second user while first holds lock
- CmdDisconnect releases owned locks
- CmdObjectLockRefresh processed without panic
- CmdObjectUpdate processed without panic
