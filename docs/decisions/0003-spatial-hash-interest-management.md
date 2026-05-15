# ADR 0003: Spatial Hashing for Interest Management

## Status

Accepted

## Context

With 200 users/room, full broadcast is too expensive. Need a deterministic,
testable way to find nearby players and objects.

## Decision

- Grid-based spatial hash as interest management foundation.
- Configurable cell size and query radius.
- K-Means is not the foundation. It can exist as one voice grouping implementation
  behind the VoiceGroupAllocator interface.

## Consequences

- Deterministic and testable interest sets.
- O(1) cell lookup, O(neighbors) query.
- K-Means flicker and instability isolated behind pluggable interface.
