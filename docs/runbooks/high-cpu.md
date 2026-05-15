# Runbook: High CPU on Single VPS

> Placeholder.

## Symptoms

- CPU > 75%
- Tick duration grows
- Latency increases

## Actions

1. Check active rooms/sessions.
2. Check tick duration metrics.
3. Check delta packet size.
4. Reduce broadcast rate if needed.
5. Enable overflow room for new joins.
6. Do not migrate live users.
