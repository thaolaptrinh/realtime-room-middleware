---
description: Create or run load test plan
argument-hint: "[scenario]"
---

Load test: $ARGUMENTS

Rules:
1. Identify target mode: dev, single-vps, or distributed-k3s.
2. Define CCU, movement pattern, object interaction, duration.
3. Capture CPU, memory, bandwidth, latency, packet stats.
4. Do not claim capacity without measured results.
5. Save findings in docs/load-testing.md.
