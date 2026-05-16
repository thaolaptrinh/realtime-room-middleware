# Spec: Voice Grouping

> Implementation spec placeholder.

## Scope

Milestone 5 deliverable. VoiceGroupAllocator interface, ProximityVoiceAllocator,
optional KMeansVoiceAllocator, VoiceGroupDelta.

## Key Decisions

- Pluggable allocator interface.
- Proximity-based default.
- K-Means is already used in Phase 1 as the position-based ClusterAllocator. For future voice grouping, K-Means is only an optional VoiceGroupAllocator policy and must not reuse Phase 1 cluster IDs without a separate design.
- Max participants per group enforced.

## Files

- `internal/game/voice/allocator.go`
- `internal/game/voice/proximity.go`
- `internal/game/voice/allocator_test.go`

## Tests Required

- Max group size
- Proximity allocation correctness
- K-Means allocation if enabled
- Stable grouping under small movement
- Config switch works
