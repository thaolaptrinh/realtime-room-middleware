---
description: Prepare a release for single VPS deployment
---

Release: $ARGUMENTS

Rules:
1. Run full test suite: make test test-race lint.
2. Build both binaries: make build.
3. Tag release in git.
4. Generate release notes.
5. Do not deploy automatically.
6. Provide deploy command for manual approval.
