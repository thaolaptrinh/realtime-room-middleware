---
description: Review changes before commit or merge
argument-hint: "[scope or file pattern]"
---

Review: $ARGUMENTS

Rules:
1. Identify changed files.
2. Check against hard rules in CLAUDE.md.
3. Run relevant tests.
4. Check for:
   - Protocol compatibility (KCP and WebSocket both affected by protocol changes)
   - Concurrency safety (room loop single-writer rule)
   - Full broadcast regression (no full-room state broadcast in normal ticks)
   - Config drift (mode-specific config not leaking into shared core)
   - Missing docs (protocol.md, architecture.md up to date)
   - No separate native/WebGL gameplay protocol introduced
   - Transport adapters do not mutate room state
   - No JSON used for realtime gameplay packets on either transport
   - No Protobuf introduced for Protocol v1
5. Report: issues found, tests passed, tests skipped, risks.
