# Runbook: Room Overflow

> Placeholder.

## Symptoms

- Room approaching 200 CCU
- New join requests for full room

## Actions

1. Check room instance capacity.
2. Redirect new joins to overflow instance if configured.
3. Do not migrate live users.
4. Document whether overflow is automatic or manual.
