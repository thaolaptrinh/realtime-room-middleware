# Runbook: Deploy to Single VPS

> Placeholder — to be written during Milestone 6.

## Steps

1. Build release binaries via CI.
2. SCP release to `/opt/realtime-server/releases/{release_id}/`.
3. Update `/opt/realtime-server/current` symlink.
4. Restart gateway and game-server via systemctl.
5. Run HTTP healthcheck.
6. Run KCP smoke test.
7. Confirm logs and metrics.

## Rollback

1. Point symlink to previous release.
2. Restart services.
3. Verify healthcheck and KCP smoke test.

## Safety

- Do not run these commands automatically from CI without explicit approval.
- Always verify healthcheck after restart.
