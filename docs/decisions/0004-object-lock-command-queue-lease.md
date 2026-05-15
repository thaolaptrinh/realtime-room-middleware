# ADR 0004: Object Lock via Command Queue + Lease TTL

## Status

Accepted

## Context

Multiple users can interact with the same room object. Need to prevent conflicting
interactions without poor UX from optimistic locking or stuck permanent locks.

## Decision

- Server-authoritative command queue.
- Lease-based locking with configurable TTL.
- Automatic expiration when lease not refreshed.
- Disconnect releases all user locks.
- Max locks per user enforced.

## Consequences

- Predictable lock behavior.
- No stuck locks from disconnect/crash.
- Lock results are server-authoritative (no client-side assumption of success).
- Requires lock TTL tuning in load tests.
