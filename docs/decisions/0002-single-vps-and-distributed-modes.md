# ADR 0002: Single VPS and Distributed Deployment Modes

## Status

Accepted

## Context

Current budget provides one Sakura Cloud Tokyo VPS. Future scale needs K3s/KEDA.
Core logic must not be duplicated between modes.

## Decision

- One codebase, one shared realtime core.
- Three runtime modes: dev (Docker Compose), single-vps (Go binaries + systemd), distributed-k3s (K3s + Redis + KEDA).
- Mode-specific logic lives only in adapters and deployment folders.
- Single VPS must not require Docker, Redis, K3s, KEDA, or ECR.

## Consequences

- Clean separation of core logic from deployment infrastructure.
- Single VPS deploys as two native Go binaries.
- Distributed mode is built as separate adapters and manifests.
- No live room migration in initial design.
