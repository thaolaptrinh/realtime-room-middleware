#!/usr/bin/env bash
#
# preflight.sh — Verify prerequisites before single VPS deployment
#
# Safe checks only. Does not modify anything, does not connect to
# production, does not require secrets.
#
# Environment variables:
#   DEPLOY_ROOT   — Install root (default: /opt/realtime-server)
#   CONFIG_PATH   — Expected config file path (default: /opt/realtime-server/config/production.yaml)
#
set -euo pipefail

DEPLOY_ROOT="${DEPLOY_ROOT:-/opt/realtime-server}"
CONFIG_PATH="${CONFIG_PATH:-${DEPLOY_ROOT}/config/production.yaml}"
ERRORS=0

echo "=== Single VPS Preflight Checks ==="
echo ""

# --- Required binaries ---
echo "--- Required binaries ---"

for cmd in go systemctl; do
    if command -v "${cmd}" >/dev/null 2>&1; then
        echo "  [OK] ${cmd} — $(command -v "${cmd}")"
    else
        echo "  [MISSING] ${cmd} — not found in PATH"
        ERRORS=$((ERRORS + 1))
    fi
done

echo ""

# --- Config file ---
echo "--- Config file ---"

if [ -f "${CONFIG_PATH}" ]; then
    echo "  [OK] ${CONFIG_PATH} exists"
else
    echo "  [MISSING] ${CONFIG_PATH} — will be needed at deploy time"
    ERRORS=$((ERRORS + 1))
fi

echo ""

# --- Systemd units ---
echo "--- Systemd unit files ---"

for unit in gateway.service game-server.service; do
    UNIT_PATH="/etc/systemd/system/${unit}"
    if [ -f "${UNIT_PATH}" ]; then
        echo "  [OK] ${UNIT_PATH}"
    else
        echo "  [MISSING] ${UNIT_PATH} — install before deploying"
        ERRORS=$((ERRORS + 1))
    fi
done

echo ""

# --- Firewall notes ---
echo "--- Firewall reminders ---"
echo "  Ensure the following rules are configured:"
echo "    ALLOW TCP :8080  (Gateway HTTP)"
echo "    ALLOW UDP :9000  (Game Server KCP)"
echo "    ALLOW SSH from admin IP only"
echo "    DENY  all other inbound"
echo ""

# --- File descriptor limits ---
echo "--- File descriptor limit ---"

SOFT_LIMIT=$(ulimit -Sn 2>/dev/null || echo "unknown")
echo "  Current soft limit: ${SOFT_LIMIT}"
echo "  Recommended: 1048576 (set in systemd unit LimitNOFILE=)"
echo ""

# --- Summary ---
if [ "${ERRORS}" -eq 0 ]; then
    echo "=== Preflight PASSED ==="
else
    echo "=== Preflight: ${ERRORS} issue(s) found ==="
fi

exit "${ERRORS}"
