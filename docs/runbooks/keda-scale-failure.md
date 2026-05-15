# Runbook: KEDA Scale Failure (Distributed Mode)

> Placeholder.

## Symptoms

- Pending room queue growing
- No new game nodes spawning

## Actions

1. Check pending-room queue depth.
2. Check KEDA operator logs.
3. Check ScaledObject configuration.
4. Check image pull success.
5. Check game-node readiness.
6. Manually scale game-node deployment if needed.
