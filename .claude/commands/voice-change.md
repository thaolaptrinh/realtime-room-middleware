---
description: Change voice grouping or proximity allocation
argument-hint: "[change description]"
---

Voice change: $ARGUMENTS

Rules:
1. Inspect docs/voice-grouping.md.
2. Preserve VoiceGroupAllocator interface.
3. K-Means must stay behind the interface, not foundational.
4. Add tests for group size, allocation correctness, stability.
5. Run:
   - make test
   - make test-race
