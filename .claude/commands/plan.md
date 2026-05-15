---
description: Create an implementation plan without editing files
argument-hint: "[task]"
---

Task: $ARGUMENTS

Rules:
1. Do not edit files.
2. Identify affected mode: dev, single-vps, distributed-k3s, or shared core.
3. Identify affected area: protocol, gateway, room, spatial, delta, object, voice, infra, CI/CD, loadtest.
4. Read relevant docs and code.
5. Produce:
   - goal
   - affected files
   - implementation steps
   - risks
   - tests to run
   - rollback notes
6. For protocol or infra changes, require explicit approval before implementation.
