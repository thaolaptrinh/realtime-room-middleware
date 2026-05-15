---
description: Review changes before commit or merge
argument-hint: "[scope or file pattern]"
---

Review: $ARGUMENTS

Rules:
1. Identify changed files.
2. Check against hard rules in CLAUDE.md.
3. Run relevant tests.
4. Check for: protocol compatibility, concurrency safety, full broadcast regression, config drift, missing docs.
5. Report: issues found, tests passed, tests skipped, risks.
