---
description: Change object synchronization or locking logic
argument-hint: "[change description]"
---

Object lock change: $ARGUMENTS

Rules:
1. Inspect docs/object-locking.md.
2. Preserve command queue + lease TTL model.
3. Add tests for lock, reject, refresh, release, expiration, disconnect.
4. Run:
   - make test
   - make test-race
   - make smoke-kcp
5. Report impact on object state versioning and ObjectDelta.
