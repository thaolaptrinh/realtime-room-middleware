# OpenCode Configuration — Realtime Room Middleware

## Dual Agent Support

This repo supports both **Claude Code** and **OpenCode** as coding agent workflows.

- **Claude Code** uses `CLAUDE.md` and `.claude/*` for commands, agents, and settings.
- **OpenCode** uses this file (`OPENCODE.md`) and `.opencode/*` for commands and agents.
- Both workflows follow the same architecture rules, scope constraints, and hard rules.
- The authoritative project rules file is `CLAUDE.md`. Read it first for the full hard rules and scope definitions.

## Hard Rules

These rules are mandatory for all OpenCode sessions. They mirror the hard rules in `CLAUDE.md`.

### Always

- Do not full-broadcast room state in normal ticks.
- Do not mutate room state from network goroutines.
- Do not change protocol format without updating `docs/protocol.md` and tests.
- Do not duplicate core logic between single-vps and distributed modes.
- Do not introduce Redis dependency into single-vps runtime unless explicitly requested.
- Do not introduce Docker dependency into single-vps production.
- Do not run destructive infra commands.
- Do not edit secrets or `.env` files.
- Do not deploy or restart production services unless explicitly approved.
- Do not claim 200 CCU capacity without measured load test results.

### Phase 1 Gameplay Scope

- Do not implement voice grouping (VoiceGroupAllocator, VoiceGroupDelta) unless explicitly requested.
- Do not implement object sync (ObjectState, ObjectDelta, object command queue) unless explicitly requested.
- Do not expand object locking (ObjectLockManager runtime) unless explicitly requested.
- Keep voice/object features documented as future scope — do not delete their documentation.
- Current Phase 1 gameplay focus is player position transform sync + K-Means cluster-based delta broadcast.

### Transport Rules

- KCP and WSS clients must receive the same MessagePack gameplay protocol — no separate schemas.
- Do not use JSON for realtime gameplay packets on either transport.
- Do not add Protobuf dependencies.
- Do not add `.proto` files.
- Do not implement Protobuf for Protocol v1. Protobuf is deferred to a future Protocol v2.
- Transport type must not affect cluster membership, delta content, or room logic.
- Transport packages must not import `internal/game`.
- Mixed transport tests (KCP senders + WSS receivers in the same room) are required before declaring production readiness.

### Cluster Allocator Rules

- Use spatial hash (`GridSpatialHash`) for per-tick proximity lookups.
- Use `ClusterAllocator` interface for position-based sync grouping.
- K-Means is the Phase 1 `ClusterAllocator` for position-based sync grouping. It must remain behind the interface.
- Do not call `ClusterAllocator.Compute` on every room tick without benchmark approval. Use the interval + movement + membership change triggers defined in `docs/specs/spec_kmeans_cluster_sync.md`.
- Cluster computation must happen only in the room loop goroutine. Never in transport goroutines.
- Room loop is the sole owner of room/player/cluster mutations.

### Implementation Guardrails

- Do not run tests or build commands unless the user explicitly asks.
- Do not change protocol format without updating `docs/protocol.md` and tests.
- Do not duplicate core logic between single-vps and distributed modes.
- Do not add Redis/KEDA/K3s runtime behavior to Phase 1.
- Do not run destructive infra commands.
- Do not edit secrets or `.env` files.

## Architecture

- Room loop is the sole writer of room state, player state, and cluster assignments.
- Network goroutines push inputs/commands into queues — never mutate room state.
- Spatial hash (`GridSpatialHash`) is the per-tick proximity index (always required).
- K-Means is the Phase 1 `ClusterAllocator` for position-based sync grouping — behind the interface.
- Delta broadcast is cluster-scoped in Phase 1. No full-room broadcast in normal ticks.
- Transport adapters must not import `internal/game`.
- Delta/cluster code must not depend on transport type.
- Cluster allocation is transport-agnostic.

## Transport

- **Control plane**: HTTP/TCP JSON Gateway `:8080`.
- **Realtime data plane (Unity native)**: KCP over UDP `:9000` + MessagePack.
- **Realtime data plane (Unity WebGL)**: WSS/WebSocket `:9001` + MessagePack.
- **Shared realtime payload**: MessagePack Protocol v1 — identical on both transports.

## Current Target

- **Phase 1**: position cluster sync on single-vps production.
- **Immediate gameplay focus**: player transform sync, spatial hash, K-Means cluster allocator, cluster-based PlayerDelta, mixed KCP/WSS support.

## Deferred Future Scope

Do not implement until explicitly requested:
- Voice grouping (VoiceGroupAllocator, VoiceGroupDelta).
- Object sync (ObjectState, ObjectDelta, object command queue).
- Object locking expansion (ObjectLockManager runtime).
- Distributed K3s runtime (RedisNodeResolver, RedisRoomRegistry, KEDA).
- Reconnect flow, LeaveRoom.

## Key Docs

- `docs/full_production_architecture_workflow_blueprint.md` — full architecture and workflow blueprint
- `docs/architecture.md` — system architecture overview
- `docs/protocol.md` — MessagePack protocol specification
- `docs/specs/spec_kmeans_cluster_sync.md` — K-Means cluster allocator spec
- `docs/interest-management.md` — spatial hash, cluster, and interest management
- `docs/delta-broadcast.md` — delta broadcast pipeline
- `docs/room-lifecycle.md` — room lifecycle, join/leave, tick loop

## Commands

OpenCode commands are in `.opencode/commands/`. Each mirrors a `.claude/commands/` equivalent.

## Agents

OpenCode agents are in `.opencode/agents/`. Each mirrors a `.claude/agents/` equivalent.

## Keeping Parity

When updating Claude Code workflows (`.claude/commands/`, `.claude/agents/`), apply the same changes to the corresponding OpenCode files (`.opencode/commands/`, `.opencode/agents/`). See `.opencode/README.md` for the full mapping.
