# Runbook: Redis Failure (Distributed Mode)

> Placeholder.

## Symptoms

- Gateway fails readiness
- New room assignment fails

## Actions

1. Existing game rooms continue if already running.
2. New room assignment may fail or degrade.
3. Do not kill active game nodes automatically.
4. Follow Redis recovery runbook.
