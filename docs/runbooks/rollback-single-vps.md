# Runbook: Rollback Single VPS

> Placeholder — to be written during Milestone 6.

## Steps

1. Find previous release directory.
2. Point symlink to previous release.
3. systemctl restart gateway
4. systemctl restart game-server
5. HTTP healthcheck.
6. KCP smoke test.
7. Confirm logs.

## Safety

- Previous release must not be deleted until rollback is confirmed.
