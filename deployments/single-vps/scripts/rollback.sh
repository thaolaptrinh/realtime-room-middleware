#!/usr/bin/env bash
#
# rollback.sh — Rollback single VPS to previous release
#
# This script is a scaffold. Do NOT run against production without
# explicit approval.
#
set -euo pipefail

PREVIOUS_RELEASE="${1:?Usage: rollback.sh <previous_release_id>}"
DEPLOY_ROOT="/opt/realtime-server"
CURRENT_LINK="${DEPLOY_ROOT}/current"
TARGET_DIR="${DEPLOY_ROOT}/releases/${PREVIOUS_RELEASE}"

echo "=== Rollback to release ${PREVIOUS_RELEASE} ==="

if [ ! -d "${TARGET_DIR}" ]; then
    echo "ERROR: Previous release not found at ${TARGET_DIR}"
    exit 1
fi

# TODO: All steps require explicit approval before running on production
echo "[1/5] Point symlink to previous release — DRY RUN"
echo "[2/5] Restart gateway — DRY RUN"
echo "[3/5] Restart game-server — DRY RUN"
echo "[4/5] HTTP healthcheck — DRY RUN"
echo "[5/5] KCP smoke test — DRY RUN"

echo "=== Rollback scaffold complete. Production steps require manual approval. ==="
