# Room Lifecycle

## Logical Room ID vs Room Instance ID

**Logical room ID** (`LogicalRoomID`) is the product-facing room identifier, e.g., `expo-room-a`. Multiple physical instances may share one logical ID when a room overflows or scales.

**Room instance ID** (`RoomInstanceID`) is the physical runtime instance identifier, e.g., `expo-room-a-0001`. Each running room loop has a unique instance ID. The `RoomManager` generates instance IDs using a monotonically increasing counter.

## Room Creation and Assignment

```
Client → Gateway POST /join (logical_room_id)
Gateway → NodeResolver.ResolveRoom(logical_room_id, user_id)
  single-vps: SingleNodeResolver → configured game server address
  distributed: RedisNodeResolver → Redis-backed node lookup (Phase 2)
Gateway → returns: game_node_addr, room_instance_id, session_token, transport endpoint
Client → opens KCP/UDP or WSS connection to game server
```

On the game server side:

```
RoomManager.CreateRoom(ctx, logicalID)
  → generate unique RoomInstanceID
  → RoomRegistry.CreateRoom(spec)   -- registers metadata
  → newRoom(spec, logger)           -- allocates Room struct
  → room.Start(ctx)                 -- launches tick goroutine
  → store in live rooms map
  → return *Room
```

## Room States

```
Created  →  Running  →  Draining  →  Closed
           (Start)       (Stop)      (Stop complete)
```

| State    | Description                                   |
|----------|-----------------------------------------------|
| Created  | `newRoom()` called; tick goroutine not started |
| Running  | Tick goroutine active; accepts Enqueue calls   |
| Draining | Stop initiated; no new Enqueue accepted        |
| Closed   | Tick goroutine exited                          |

## Join Flow (game server side)

```
1. Transport goroutine (KCP or WSS) receives Hello + JoinRoom.
2. Transport goroutine validates envelope and session token.
3. Transport goroutine calls room.Enqueue(RoomCommand{Kind: CmdJoin, ...}).
4. Room tick loop processes CmdJoin on next tick.
5. Room loop sends JoinAccepted + FullSnapshot to session. (future milestone)
```

Transport goroutines must not mutate room state directly. All state changes go through `Enqueue`.

## Leave Flow

```
1. Client sends LeaveRoom (future) or connection closes.
2. Transport goroutine enqueues RoomCommand{Kind: CmdLeave or CmdDisconnect}.
3. Room loop processes command on next tick:
   - removes player from state (future milestone: PlayerState)
   - releases all locks owned by player (future milestone: ObjectLockManager)
   - removes player from spatial index (future milestone: SpatialIndex)
   - emits player leave in next delta broadcast (future milestone: DeltaBroadcaster)
```

## Reconnect Flow

Reserved. Not yet implemented. Will use `CmdDisconnect` + a reconnect token flow.

## Idle Room Cleanup

Not yet implemented. Placeholder: the `RoomManager.CloseRoom` method can be called explicitly to shut down an idle room. An idle detection loop (watching `PlayerCount() == 0` for a configurable duration) will be added in a later milestone.

## Session Tracking

The `Room` maintains two internal indexes updated exclusively by the room loop:

- `activeSessions map[SessionID]sessionAttachment` — maps each attached session to its player/user IDs.
- `userSessionIndex map[UserID]SessionID` — reverse index for duplicate-user detection.

Both indexes are protected by `sessionMu sync.RWMutex`. The room loop holds the write lock when mutating (inside `handleCommand`). External callers hold the read lock via `HasSession`, `HasUser`, and `ActiveSessions`.

### Duplicate User Rule

A `CmdJoin` command whose `UserID` is already present in `userSessionIndex` is **silently rejected** by the room loop. The `playerCount` is not incremented and the session is not added to `activeSessions`. A future milestone will send an error response to the client.

### Session Cleanup on Disconnect

`CmdDisconnect` removes the session from both indexes, exactly as `CmdLeave` does. The room does not call `Close()` on any transport object — that is the `SessionManager`'s responsibility.

## Room Loop Rule

**Only the room loop goroutine (`runTick`) may mutate room state.**

Network goroutines (KCP read loop, WebSocket read loop) must:
- Read packets from transport
- Decode the MessagePack envelope
- Validate protocol version and message type
- Call `room.Enqueue(RoomCommand{...})`

Network goroutines must not:
- Write to `Room.Players`, `Room.Objects`, or any state map
- Call spatial index mutation methods
- Call lock manager methods

## Room Tick and Broadcast

```yaml
game:
  tick_rate_hz: 20        # simulation and command drain frequency
  broadcast_rate_hz: 10   # delta packet send frequency
```

Phase 1 skeleton: tick drains command queue only. Full simulation added in Milestone 3–5.

## Overflow Room Behavior

Not yet implemented. Design intent:
- When a room instance reaches `MaxPlayers`, the resolver creates a new instance.
- New joins are routed to the overflow instance with the same `LogicalRoomID`.
- Live rooms are not migrated. Players complete their session in their assigned instance.

## Hard Rules

- Do not migrate live rooms.
- Room loop is the only writer of room state.
- `RoomInstance` in the registry tracks metadata only; live state lives in `Room`.
- Phase 1 registry (`InMemoryRoomRegistry`) has no external dependencies.
