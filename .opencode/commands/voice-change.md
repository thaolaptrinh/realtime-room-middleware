---
description: Change voice grouping or proximity allocation
---

Voice change: $ARGUMENTS

Rules:
1. Inspect docs/voice-grouping.md.
2. Preserve VoiceGroupAllocator interface.
3. For voice changes, do not modify the Phase 1 position ClusterAllocator unless explicitly requested. Any future K-Means voice policy must stay behind VoiceGroupAllocator and remain separate from Phase 1 position cluster sync.
4. Add tests for group size, allocation correctness, stability.
5. Run:
   - make test
   - make test-race
