---
description: Change delta broadcast or snapshot cache
argument-hint: "[change description]"
---

Delta broadcast change: $ARGUMENTS

Rules:
1. Inspect docs/delta-broadcast.md.
2. Preserve enter/update/leave semantics.
3. Do not introduce full-room broadcast in normal ticks.
4. Add tests for enter, update, leave, no-op, full snapshot fallback.
5. Run:
   - make test
   - make bench-delta
   - make smoke-kcp
6. Report packet size impact if measurable.
