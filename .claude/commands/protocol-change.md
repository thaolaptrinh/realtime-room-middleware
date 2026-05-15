---
description: Safely change MessagePack/KCP protocol
argument-hint: "[change description]"
---

Protocol change: $ARGUMENTS

Rules:
1. Inspect docs/protocol.md first.
2. Preserve backward compatibility unless explicitly approved.
3. Update docs/protocol.md.
4. Update protocol tests and fixtures.
5. Run:
   - make test
   - make smoke-kcp
6. Report Unity client impact.
