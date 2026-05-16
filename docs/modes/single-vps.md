# Single VPS Production Mode

## Runtime

- 2 Go binaries: `gateway` and `game-server`
- Process manager: systemd
- No Docker, Redis, K3s, KEDA, or ECR required
- See `deployments/single-vps/`

## Gateway

- HTTP on `:8080` (JSON control plane)
- Routes: `GET /healthz`, `GET /readyz`, `POST /join`
- Uses `SingleNodeResolver` — returns configured local KCP address and WSS URL
- `POST /join` accepts `client_platform` (`native` or `webgl`) and optional `requested_transport`
- `client_platform=native` → `transport=kcp`, returns `kcp_addr`
- `client_platform=webgl` → `transport=websocket`, returns `websocket_url`
- Session tokens: opaque random (hardening needed for production auth)
- Graceful shutdown on SIGINT/SIGTERM with 5-second timeout

## Game Server

- KCP/UDP on `:9000` (MessagePack realtime data plane — Unity native)
- WSS/WebSocket on `:9001` (MessagePack realtime data plane — Unity WebGL, not yet implemented)
- See `docs/protocol.md` for wire format

## Phase 1 Status

- Gateway HTTP skeleton: implemented
- Dual transport join contract (KCP + WebSocket endpoint assignment): implemented
- SingleNodeResolver: implemented
- Session token placeholder: implemented
- Room runtime: not yet implemented
- Object locking: not yet implemented
- WebSocket server: not yet implemented
- Production auth: not yet implemented
- Load testing: not yet done

## What Is Not Included

- No Redis dependency at runtime
- No RedisNodeResolver (distributed-k3s only)
- No Docker at runtime
- No K3s, KEDA, or container registry
- No production authentication
- No TLS termination (handled upstream or to be added later)
