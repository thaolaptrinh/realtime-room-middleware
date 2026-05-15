# Spec: Voice Grouping

> Implementation spec placeholder.

## Scope

Milestone 5 deliverable. VoiceGroupAllocator interface, ProximityVoiceAllocator,
optional KMeansVoiceAllocator, VoiceGroupDelta.

## Key Decisions

- Pluggable allocator interface.
- Proximity-based default.
- K-Means optional, not foundational.
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
