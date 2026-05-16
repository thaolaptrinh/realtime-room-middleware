---
description: Create or run load test plan
argument-hint: "[scenario]"
---

Load test: $ARGUMENTS

Rules:
1. Identify target mode: dev, single-vps, or distributed-k3s.
2. Define CCU, movement pattern, object interaction, duration.
3. Specify transport coverage:
   - Native KCP tests: use Go KCP load client (loadtest/shared/kcp_client.go)
   - WebSocket/WSS tests: use Go WebSocket load client (loadtest/shared/ws_client.go)
   - Mixed room tests: KCP and WebSocket clients in the same room
4. Capture CPU, memory, bandwidth, latency, packet stats per transport type.
5. Do not claim capacity without measured results.
6. Docker Compose is dev/integration only — not production load test evidence.
7. Web visualizers and debug endpoints are not load test evidence.
8. Save findings in docs/load-testing.md.
