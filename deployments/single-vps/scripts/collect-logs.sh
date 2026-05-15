#!/usr/bin/env bash
#
# collect-logs.sh — Collect logs from single VPS services
#
# Shows journald output for gateway and game-server units.
# Safe to run at any time — read-only, no side effects.
#
# Environment variables:
#   LINES        — Number of recent log lines to show (default: 100)
#   SINCE        — journald --since expression (default: "1 hour ago")
#                  Overrides LINES if set to a non-empty value.
#
set -euo pipefail

LINES="${LINES:-100}"
SINCE="${SINCE:-}"

GATEWAY_UNIT="gateway"
GAME_SERVER_UNIT="game-server"

echo "=== Single VPS Log Collection ==="
echo ""

for unit in "${GATEWAY_UNIT}" "${GAME_SERVER_UNIT}"; do
    echo "--- ${unit} ---"

    if [ -n "${SINCE}" ]; then
        journalctl -u "${unit}" --since "${SINCE}" --no-pager -q 2>/dev/null || echo "  (journald not available or unit not found)"
    else
        journalctl -u "${unit}" -n "${LINES}" --no-pager -q 2>/dev/null || echo "  (journald not available or unit not found)"
    fi

    echo ""
done

echo "=== Log collection complete ==="
echo ""
echo "Usage:"
echo "  LINES=200 ./collect-logs.sh"
echo "  SINCE='30 min ago' ./collect-logs.sh"
