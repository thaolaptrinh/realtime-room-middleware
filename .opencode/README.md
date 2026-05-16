# OpenCode Workflow — Parity with Claude Code

This repo supports two equivalent coding agent workflows:

- **Claude Code** uses `CLAUDE.md` (project rules) and `.claude/` (commands, agents, settings).
- **OpenCode** uses `OPENCODE.md` (project rules) and `.opencode/` (commands, agents).

Both workflows are intended to be **functionally equivalent** — same rules, same guardrails, same commands, same agents. `CLAUDE.md` is the authoritative source for hard rules; `OPENCODE.md` mirrors them for OpenCode compatibility.

This directory (`.opencode/`) mirrors the `.claude/` workflow for OpenCode compatibility.

## Commands Mapping

| Claude Code | OpenCode | Purpose |
|---|---|---|
| `.claude/commands/plan.md` | `.opencode/commands/plan.md` | Create an implementation plan without editing files |
| `.claude/commands/implement.md` | `.opencode/commands/implement.md` | Implement a planned change after approval |
| `.claude/commands/review.md` | `.opencode/commands/review.md` | Review changes before commit or merge |
| `.claude/commands/protocol-change.md` | `.opencode/commands/protocol-change.md` | Safely change MessagePack/KCP protocol |
| `.claude/commands/gateway-change.md` | `.opencode/commands/gateway-change.md` | Change gateway HTTP handlers, join flow, or room resolution |
| `.claude/commands/room-change.md` | `.opencode/commands/room-change.md` | Change room lifecycle, membership, overflow, or cleanup |
| `.claude/commands/spatial-change.md` | `.opencode/commands/spatial-change.md` | Change spatial hashing or grid configuration |
| `.claude/commands/delta-change.md` | `.opencode/commands/delta-change.md` | Change delta broadcast or snapshot cache |
| `.claude/commands/object-lock-change.md` | `.opencode/commands/object-lock-change.md` | Change object synchronization or locking logic |
| `.claude/commands/voice-change.md` | `.opencode/commands/voice-change.md` | Change voice grouping or proximity allocation |
| `.claude/commands/infra-single-vps-change.md` | `.opencode/commands/infra-single-vps-change.md` | Change single VPS deployment, systemd, scripts, or CI/CD |
| `.claude/commands/infra-distributed-change.md` | `.opencode/commands/infra-distributed-change.md` | Change distributed K3s, Redis, KEDA, ECR, or Kubernetes manifests |
| `.claude/commands/loadtest.md` | `.opencode/commands/loadtest.md` | Create or run load test plan |
| `.claude/commands/release-single-vps.md` | `.opencode/commands/release-single-vps.md` | Prepare a release for single VPS deployment |

## Agents Mapping

| Claude Code | OpenCode | Purpose |
|---|---|---|
| `.claude/agents/go-network-reviewer.md` | `.opencode/agents/go-network-reviewer.md` | Reviews Go KCP/UDP networking code |
| `.claude/agents/protocol-compat-reviewer.md` | `.opencode/agents/protocol-compat-reviewer.md` | Reviews MessagePack protocol changes and Unity compatibility |
| `.claude/agents/concurrency-reviewer.md` | `.opencode/agents/concurrency-reviewer.md` | Reviews Go concurrency, room loop, queues, locks, and race risks |
| `.claude/agents/realtime-sync-reviewer.md` | `.opencode/agents/realtime-sync-reviewer.md` | Reviews spatial hashing, interest management, delta broadcast correctness |
| `.claude/agents/infra-reviewer.md` | `.opencode/agents/infra-reviewer.md` | Reviews systemd, CI/CD, K3s, Redis, KEDA, Terraform, deployment safety |
| `.claude/agents/loadtest-reviewer.md` | `.opencode/agents/loadtest-reviewer.md` | Reviews load test scenarios, metrics, acceptance targets |

## Skills

No `.claude/skills` directory exists in this repo. No `.opencode/skills` directory is needed.

## How to Use OpenCode Commands

In OpenCode, invoke a command by name:

```
/plan [task description]
/implement [task description]
/review [scope or file pattern]
/protocol-change [change description]
/gateway-change [change description]
/room-change [change description]
/spatial-change [change description]
/delta-change [change description]
/object-lock-change [change description]
/voice-change [change description]
/infra-single-vps-change [change description]
/infra-distributed-change [change description]
/loadtest [scenario]
/release-single-vps [version or notes]
```

## How to Use OpenCode Agents

Agents are dispatched by OpenCode as subagents for specialized review tasks. Each agent is read-only (no file edits) unless explicitly noted otherwise.

## Keeping Parity

When updating a file in `.claude/commands/` or `.claude/agents/`, update the matching file in `.opencode/commands/` or `.opencode/agents/` with the same content. The prompt body should be identical. Only the frontmatter format differs between Claude Code and OpenCode conventions.

## Claude Code After This Change

Claude Code is unchanged. All `.claude/` files remain intact and authoritative. `CLAUDE.md` is the primary project rules file for both workflows.

## OpenCode After This Change

OpenCode reads `OPENCODE.md` as its entry point. `OPENCODE.md` contains the same hard rules, transport rules, cluster allocator rules, and implementation guardrails as `CLAUDE.md`. Commands and agents mirror the Claude equivalents 1-to-1.
