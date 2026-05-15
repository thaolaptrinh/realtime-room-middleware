#!/usr/bin/env bash
#
# healthcheck.sh — Verify Gateway and Game Server health on single VPS
#
# Checks Gateway HTTP health endpoint. KCP smoke test is left as a TODO
# until the transport layer is implemented.
#
# Environment variables:
#   GATEWAY_ADDR  — Gateway HTTP address (default: http://127.0.0.1:8080)
#   TIMEOUT_SECS  — curl timeout in seconds (default: 5)
#
set -euo pipefail

GATEWAY_ADDR="${GATEWAY_ADDR:-http://127.0.0.1:8080}"
TIMEOUT_SECS="${TIMEOUT_SECS:-5}"
EXIT_CODE=0

echo "=== Single VPS Healthcheck ==="

# --- Gateway HTTP health ---
GATEWAY_URL="${GATEWAY_ADDR}/health"
echo -n "[Gateway HTTP] ${GATEWAY_URL} ... "

HTTP_STATUS=$(curl -s -o /dev/null -w "%{http_code}" --max-time "${TIMEOUT_SECS}" "${GATEWAY_URL}" 2>/dev/null) || true

if [ "${HTTP_STATUS}" = "200" ]; then
    echo "OK (${HTTP_STATUS})"
else
    echo "FAIL (HTTP ${HTTP_STATUS:-unreachable})"
    EXIT_CODE=1
fi

# --- Game server KCP smoke test ---
# TODO: Implement KCP healthcheck once transport layer is available.
# The KCP smoke test should:
#   1. Open a KCP session to GAME_SERVER_ADDR (default: 127.0.0.1:9000)
#   2. Send a Ping message
#   3. Wait for Pong within a timeout
#   4. Report success or failure
# Until then, this check is informational only.
echo "[Game Server KCP] :9000 — SKIP (transport not implemented yet)"

# --- Summary ---
if [ "${EXIT_CODE}" -eq 0 ]; then
    echo "=== Healthcheck PASSED ==="
else
    echo "=== Healthcheck FAILED ==="
fi

exit "${EXIT_CODE}"
