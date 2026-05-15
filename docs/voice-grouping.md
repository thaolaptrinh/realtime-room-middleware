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
- K-Means is optional, not foundational.
- Max participants per group must be enforced.
