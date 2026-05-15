---
description: Change single VPS deployment, systemd, scripts, or CI/CD
argument-hint: "[change description]"
---

Single VPS infra change: $ARGUMENTS

Rules:
1. Do not run production commands automatically.
2. Do not restart services without explicit approval.
3. Do not edit secrets.
4. Update deployments/single-vps and docs/runbooks.
5. Include rollback steps.
6. Prefer dry-run or script validation.
