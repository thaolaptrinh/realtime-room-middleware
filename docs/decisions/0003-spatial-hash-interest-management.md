# ADR 0003: Spatial Hashing for Interest Management

## Status

Accepted

## Context

With 200 users/room, full broadcast is too expensive. Need a deterministic,
testable way to find nearby players and objects.

## Decision

- Grid-based spatial hash as per-tick proximity index and interest management foundation.
- Configurable cell size and query radius.
- K-Means (`KMeansClusterAllocator`) is the Phase 1 implementation of `ClusterAllocator` for position-based sync grouping. It runs periodically behind the `ClusterAllocator` interface. It is not the proximity index foundation, not room authority, and not used for physics or transport routing.
- Radius-based `InterestManager` is the fallback path when `cluster_enabled=false`.
- Voice grouping (`VoiceGroupAllocator`) is a separate, deferred interface. Phase 1 K-Means position clustering must not be conflated with future voice grouping.

## Consequences

- Deterministic and testable interest sets.
- O(1) cell lookup, O(neighbors) query via spatial hash.
- K-Means cluster membership drives Phase 1 per-client delta interest sets. Future allocator policies can be substituted behind the `ClusterAllocator` interface without changing room loop or transport code.
- Voice grouping K-Means (future) is isolated behind a separate `VoiceGroupAllocator` interface.
