# Voice / Proximity Grouping

> Placeholder — to be written during Milestone 5.

## Contents

- VoiceGroupAllocator interface
- ProximityVoiceAllocator design
- KMeansVoiceAllocator design (optional)
- VoiceGroupDelta format
- Group stability strategy
- Configuration (allocator type, radius, max participants, recompute interval)

## Hard Rules

- Voice grouping is pluggable.
- K-Means as a voice grouping policy is optional and deferred. Use `KMeansVoiceAllocator` behind `VoiceGroupAllocator` only if proximity-based allocation is insufficient. Do not conflate with Phase 1 `ClusterAllocator` (position-based sync grouping).
- Max participants per group must be enforced.
