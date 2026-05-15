---
description: Change gateway HTTP handlers, join flow, or room resolution
argument-hint: "[change description]"
---

Gateway change: $ARGUMENTS

Rules:
1. Inspect docs/architecture.md and docs/room-lifecycle.md.
2. Do not add high-frequency realtime state to the gateway.
3. Run:
   - make test
   - make smoke-gateway
4. Report impact on join flow and session token handling.
