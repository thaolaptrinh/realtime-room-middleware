# Spec: Room Manager and Multi-Room

> Implementation spec placeholder.

## Scope

Milestone 2 deliverable. Logical/instance room IDs, RoomManager, InMemoryRoomRegistry,
room lifecycle (create, get, destroy, cleanup).

## Files

- `internal/game/room.go`
- `internal/game/roommanager.go`
- `internal/adapters/registry/memory.go`
- `internal/game/room_test.go`
- `internal/adapters/registry/memory_test.go`

## Tests Required

- Create room with logical ID
- Get room by instance ID
- List instances by logical ID
- Destroy room
- Cleanup idle room
- No single-room hardcode
