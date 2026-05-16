# Spec: Room Manager and Multi-Room

## Scope

Milestone 2 (Stage 2 Task 1 and Task 3) deliverable. Logical/instance room IDs, RoomManager, InMemoryRoomRegistry, room lifecycle (create, start, stop, close), tick loop skeleton, and player transform state skeleton.

## Implementation Status

**Stage 2 Task 1 (Room Manager)**: Complete for Phase 1 single-vps skeleton.

**Stage 2 Task 3 (Player Transform Skeleton)**: Complete. Player position and rotation state tracking with validation is implemented. Spatial hashing, delta broadcast, object locking, and voice grouping are deferred to later milestones.

## Files

```
internal/game/room/doc.go         — package documentation
internal/game/room/types.go       — LogicalRoomID, RoomInstanceID, RoomStatus, RoomConfig,
                                    RoomCommandKind, RoomCommand, RoomSpec, RoomInstance
internal/game/room/registry.go    — RoomRegistry interface, InMemoryRoomRegistry
internal/game/room/room.go        — Room struct, newRoom(), getters, Enqueue(),
                                    GetPlayerState(), CurrentTick()
internal/game/room/lifecycle.go   — Start(), Stop()
internal/game/room/tick.go        — runTick(), tick(), drainCommands(), handleCommand()
                                    (now handles player transform updates)
internal/game/room/manager.go     — RoomManager, NewRoomManager(), CreateRoom(), GetRoom(),
                                    CloseRoom(), Shutdown(), ActiveRoomCount()
internal/game/room/room_test.go   — unit and integration tests

internal/game/player/doc.go       — package documentation
internal/game/player/player.go     — PlayerID, UserID, PlayerStatus, PlayerState,
                                    NewPlayerState(), Snapshot(), UpdateTransform()
internal/game/player/types.go      — Vector3, Quaternion, PlayerTransform, PlayerInput
internal/game/player/validate.go   — ValidatePlayerID, ValidateUserID, ValidateVector3,
                                    ValidateQuaternion, ValidatePlayerTransform, ValidatePlayerInput
internal/game/player/player_test.go — unit tests for player types and validation
```

## Domain Types

| Type              | Description                                            |
|-------------------|--------------------------------------------------------|
| `LogicalRoomID`   | Product-facing room identifier (e.g., `expo-room-a`)  |
| `RoomInstanceID`  | Physical instance identifier (e.g., `expo-room-a-0001`) |
| `PlayerID`        | Player identifier (string)                             |
| `SessionID`       | Transport session identifier (string)                  |
| `RoomStatus`      | Created / Running / Draining / Closed                  |
| `RoomConfig`      | MaxPlayers, TickRateHz, BroadcastRateHz, CommandQueueSize |
| `RoomCommandKind` | CmdJoin, CmdLeave, CmdDisconnect, CmdPlayerInput, CmdUpdatePlayerTransform |
| `RoomCommand`     | Envelope sent to room loop by transport goroutines     |
| `RoomSpec`        | Input for creating a room (logical ID, instance ID, config) |
| `RoomInstance`    | Registry metadata record (not live room state)         |
| `Vector3`         | 3D position with X, Y, Z float32 components            |
| `Quaternion`      | Rotation as X, Y, Z, W unit quaternion                 |
| `PlayerTransform` | Position + rotation + tick                            |
| `PlayerInput`     | Client movement update with sequence number            |
| `PlayerState`     | Player runtime record with transform, version, status  |

## RoomRegistry Interface

```go
type RoomRegistry interface {
    CreateRoom(ctx context.Context, spec RoomSpec) (*RoomInstance, error)
    GetRoom(ctx context.Context, instanceID RoomInstanceID) (*RoomInstance, error)
    ListInstances(ctx context.Context, logicalRoomID LogicalRoomID) ([]*RoomInstance, error)
    MarkClosed(ctx context.Context, instanceID RoomInstanceID) error
}
```

Phase 1 implementation: `InMemoryRoomRegistry` (no external dependencies, mutex-protected).
Phase 2 implementation: `RedisRoomRegistry` in `internal/adapters/registry/` (deferred).

## Room Lifecycle

```
newRoom()   → status Created
Start(ctx)  → status Running   (tick goroutine launched)
Stop()      → status Draining → status Closed (tick goroutine waits and exits)
```

`Stop()` is idempotent. Calling it on a Closed room is a no-op.

## Tick Loop Skeleton

`runTick` runs at `RoomConfig.TickRateHz` (default 20 Hz). Each tick:

1. Drain the command queue (`drainCommands`)
2. Dispatch each command (`handleCommand`) — currently: update `playerCount` placeholder

Future ticks will: update player/object state, release expired locks, update spatial hash, compute interest sets, allocate voice groups, build delta packets.

## Mutation Rule

Only `runTick` (the room loop goroutine) calls `handleCommand` and mutates room state. Transport goroutines call `Room.Enqueue(RoomCommand{...})` — never mutation methods directly.

## Command Queue

Buffered channel (`RoomConfig.CommandQueueSize`, default 256). Enqueue returns an error immediately if the queue is full or the room is not Running.

## Instance ID Generation

`RoomManager` generates instance IDs as `"<logicalID>-<zero-padded-counter>"` using an atomic counter (e.g., `expo-room-a-0001`, `expo-room-a-0002`). No single-room hardcoding.

## Tests

| Test                                         | Status |
|----------------------------------------------|--------|
| Create room with logical ID                  | ✓      |
| Get room by instance ID                      | ✓      |
| Get nonexistent room returns error           | ✓      |
| Duplicate instance ID rejected               | ✓      |
| List instances by logical ID                 | ✓      |
| List empty returns nil slice (not error)     | ✓      |
| MarkClosed sets status and ClosedAt          | ✓      |
| MarkClosed nonexistent returns error         | ✓      |
| Registry concurrent access is safe           | ✓      |
| Manager create and get                       | ✓      |
| Multiple rooms — no single-room hardcode     | ✓      |
| Manager close room                           | ✓      |
| Manager close nonexistent returns error      | ✓      |
| Manager shutdown stops all rooms             | ✓      |
| Room start → Running status                  | ✓      |
| Room stop → Closed status                    | ✓      |
| Stop is idempotent                           | ✓      |
| Context cancellation exits tick goroutine    | ✓      |
| Tick loop processes CmdJoin command          | ✓      |
| Tick loop processes CmdLeave command         | ✓      |
| Enqueue rejected when room not Running       | ✓      |

## Dependencies

- No Redis client dependency.
- No transport package dependency from `internal/game/room`.
- No `internal/game` imports from transport packages.

## Session Tracking (Stage 2 Task 2)

### New Room fields

| Field               | Type                              | Access                                                          |
|---------------------|-----------------------------------|-----------------------------------------------------------------|
| `sessionMu`         | `sync.RWMutex`                    | Internal                                                        |
| `activeSessions`    | `map[SessionID]sessionAttachment` | Room loop (write); `HasSession`, `ActiveSessions` (read)        |
| `userSessionIndex`  | `map[UserID]SessionID`            | Room loop (write); `HasUser` (read)                             |

### New Room methods

| Method                          | Safe from      | Returns                        |
|---------------------------------|----------------|--------------------------------|
| `HasSession(SessionID) bool`    | Any goroutine  | True if session is attached    |
| `HasUser(UserID) bool`          | Any goroutine  | True if user is in room        |
| `ActiveSessions() []SessionID`  | Any goroutine  | Snapshot copy of session IDs   |

### New types (room package)

- `UserID string` — in `types.go`, for room-internal use in commands and indexes.
- `sessionAttachment struct{playerID PlayerID; userID UserID}` — private to the room package.
- `RoomCommand.UserID UserID` — new field; set by caller on `CmdJoin` for duplicate detection.

### New packages

| Package                   | Provides                                                              |
|---------------------------|-----------------------------------------------------------------------|
| `internal/game/session`   | `Session`, `SessionManager`, `SessionID`, `UserID`, `SessionState`   |
| `internal/game/player`    | `PlayerState`, `PlayerStatus`, `PlayerID`, `UserID`                  |

### Additional tests (Stage 2 Task 2)

| Test                                              | Status |
|---------------------------------------------------|--------|
| HasSession returns true after CmdJoin             | ✓      |
| HasSession returns false after CmdLeave           | ✓      |
| HasUser after join; absent after leave            | ✓      |
| Duplicate user join rejected by room loop         | ✓      |
| CmdDisconnect removes session                     | ✓      |
| ActiveSessions returns correct count              | ✓      |
| session.Register creates Pending session          | ✓      |
| session.Attach → Attached state + user index      | ✓      |
| session.Attach rejects duplicate user             | ✓      |
| session.Detach → Detached; user index cleared     | ✓      |
| session.Close removes from indexes; closes conn   | ✓      |
| KCP and WebSocket sessions both register          | ✓      |
| session package does not import game/room         | ✓      |

### Player Transform (Stage 2 Task 3)

| Test                                              | Status |
|---------------------------------------------------|--------|
| PlayerState created on CmdJoin                    | ✓      |
| PlayerState removed on CmdLeave                   | ✓      |
| PlayerState removed on CmdDisconnect              | ✓      |
| UpdateTransform increments version                | ✓      |
| GetPlayerState returns consistent snapshot        | ✓      |
| CmdPlayerInput updates player transform           | ✓      |
| CmdUpdatePlayerTransform updates transform        | ✓      |
| Invalid player input rejected (NaN/Inf)           | ✓      |
| CurrentTick increments each tick loop             | ✓      |
| Player transform validation works                 | ✓      |
| Transport packages still do not import game/room  | ✓      |

## What Remains Intentionally Unimplemented

- **Player animation state** (`AnimState`) — transform skeleton is in place, but animation state tracking is deferred
- **Spatial hashing** (`SpatialIndex`)
- **Interest management** (`InterestManager`)
- **Delta broadcast** (`DeltaBroadcaster`, `ClientSnapshotCache`)
- **Object state and locking** (`ObjectState`, `ObjectLockManager`)
- **Voice grouping** (`VoiceGroupAllocator`)
- **Session token validation** (deferred with transport milestone)
- **Reconnect flow**
- **Idle room cleanup** (no auto-detection yet)
- **FullSnapshot and delta packet encoding**
- **Redis registry** (`RedisRoomRegistry`) — Phase 2 only
