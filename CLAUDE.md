# Realtime Room Middleware

## Product

Custom realtime middleware server for Unity, replacing part of Normcore synchronization for 200 CCU room instances.

## Deployment Modes

- **dev**: Docker Compose
- **single-vps**: Go binaries + systemd, no Docker, no Redis required
- **distributed-k3s**: K3s + Redis + KEDA + container registry

## Transport

- Control plane: HTTP/TCP JSON Gateway `:8080`
- Realtime data plane: KCP over UDP `:9000`
- Realtime payload: MessagePack

## Core Architecture

- Spatial hashing for interest management
- Delta broadcast for bandwidth reduction
- Room loop is the only writer of room state
- Network goroutines push inputs/commands into queues
- Object locking uses server command queue + lease TTL
- Voice grouping is pluggable; K-Means is optional, not foundational

## Implementation Phase

- **Current target:** Phase 1 — single-vps production.
- **Distributed K3s:** docs, skeleton, and placeholder files only. Do not implement distributed runtime until Phase 2 is explicitly started.

## Hard Rules

- Do not full-broadcast room state in normal ticks.
- Do not mutate room state from network goroutines.
- Do not change protocol format without updating `docs/protocol.md` and tests.
- Do not duplicate core logic between single-vps and distributed modes.
- Do not introduce Redis dependency into single-vps runtime unless explicitly requested.
- Do not introduce Docker dependency into single-vps production.
- Do not run destructive infra commands.
- Do not edit secrets or .env files.
- Do not deploy or restart production services unless explicitly approved.
- Do not claim 200 CCU capacity without measured load test results.

## Transport Rules

- Unity native clients use KCP/UDP `:9000`.
- Unity WebGL clients use WSS/WebSocket `:9001`.
- Both transports carry the same MessagePack Protocol v1 payloads — one shared application/gameplay protocol, not two.
- Do not create separate gameplay message types or schemas for native vs WebGL.
- Do not use JSON for realtime gameplay packets on either transport.
- Do not implement Protobuf for Protocol v1. Protobuf is deferred to a future Protocol v2.
- Transport adapters must not mutate room state. Only the room loop writes room state.
- Native and WebGL clients may coexist in the same room instance.
- Mixed transport tests (KCP senders + WebSocket receivers, and vice versa) are required before declaring production readiness.

## Verification

Gateway changes:
- `make test`
- `make smoke-gateway`

Game server changes:
- `make test`
- `make test-race`
- `make smoke-kcp`

Protocol changes:
- update `docs/protocol.md`
- run protocol compatibility tests
- run `make smoke-kcp`

Spatial/delta changes:
- run unit tests
- run benchmark if performance-sensitive
- run loadtest if behavior affects bandwidth

Infra changes:
- plan/diff only unless explicitly approved
- update runbook

## Project Structure

```
cmd/gateway/           Gateway binary
cmd/game-server/       Game server binary
internal/config/       Config loader
internal/protocol/     MessagePack envelope and messages
internal/transport/kcp/ KCP server and sessions
internal/gateway/      HTTP handlers
internal/game/room/    Room struct, room loop, RoomManager
internal/game/player/  PlayerState, movement input
internal/game/object/  ObjectState, lock manager (lease TTL)
internal/game/session/ Session management, KCP↔player mapping
internal/game/spatial/ Spatial hash
internal/game/interest/ Interest manager
internal/game/delta/   Delta broadcaster, snapshot cache
internal/game/voice/   Voice group allocator
internal/adapters/     Resolver and registry implementations
internal/observability/ Metrics and logging
deployments/           Mode-specific deployment configs
config/                Mode-explicit example configs
docs/                  Architecture, protocol, runbooks
```
