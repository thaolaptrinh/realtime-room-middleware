---
description: Change gateway HTTP handlers, join flow, or room resolution
argument-hint: "[change description]"
---

Gateway change: $ARGUMENTS

Rules:
1. Inspect docs/architecture.md and docs/room-lifecycle.md.
2. Do not add high-frequency realtime state to the gateway.
3. The /join response must include transport-specific endpoint assignment:
   - `client_platform: native` → return KCP/UDP game-server endpoint (:9000)
   - `client_platform: webgl` → return WSS/WebSocket game-server endpoint (:9001)
4. Both transport types receive the same: room instance id, session token, protocol version.
5. Run:
   - make test
   - make smoke-gateway
6. Report impact on join flow, session token handling, and transport endpoint routing.
