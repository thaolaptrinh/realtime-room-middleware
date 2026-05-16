---
description: Safely change MessagePack/KCP protocol
argument-hint: "[change description]"
---

Protocol change: $ARGUMENTS

Rules:
1. Inspect docs/protocol.md first.
2. Preserve backward compatibility unless explicitly approved.
3. Protocol changes affect BOTH KCP and WebSocket transports — validate both.
4. Update docs/protocol.md.
5. Update MessagePack fixture tests for the changed message types.
6. Verify mixed transport compatibility: the change must not break interoperability between KCP and WebSocket clients in the same room.
7. Run:
   - make test
   - make smoke-kcp
8. Report Unity client impact (native and WebGL).
