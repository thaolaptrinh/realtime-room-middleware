---
description: Change room lifecycle, membership, overflow, or cleanup
---

Room change: $ARGUMENTS

Rules:
1. Inspect docs/room-lifecycle.md.
2. Do not migrate live rooms.
3. Preserve logical room vs room instance distinction.
4. Add tests for join, leave, reconnect, cleanup.
5. Run:
   - make test
   - make test-race
   - make smoke-kcp
