#!/usr/bin/env bash
#
# deploy.sh — Deploy a release to single VPS
#
# Usage: ./deploy.sh <release_id> [binary_path]
#
# This script is a scaffold. Do NOT run against production without
# explicit approval. Update paths and SCP target for your environment.
#
set -euo pipefail

RELEASE_ID="${1:?Usage: deploy.sh <release_id> [binary_path]}"
BINARY_PATH="${2:-.}"
DEPLOY_ROOT="/opt/realtime-server"
RELEASE_DIR="${DEPLOY_ROOT}/releases/${RELEASE_ID}"
CURRENT_LINK="${DEPLOY_ROOT}/current"

echo "=== Deploy release ${RELEASE_ID} ==="

# Build
echo "[1/6] Building binaries..."
go build -o "${BINARY_PATH}/gateway" ./cmd/gateway
go build -o "${BINARY_PATH}/game-server" ./cmd/game-server

# TODO: SCP to VPS (requires explicit approval before running)
# scp "${BINARY_PATH}/gateway" "${BINARY_PATH}/game-server" vps:"${RELEASE_DIR}/"
echo "[2/6] SCP step — DRY RUN (not executed without explicit approval)"

# TODO: Update symlink on VPS
echo "[3/6] Update symlink — DRY RUN"

# TODO: Restart services on VPS
echo "[4/6] Restart services — DRY RUN"

# TODO: Healthcheck
echo "[5/6] Healthcheck — DRY RUN"

# TODO: KCP smoke test
echo "[6/6] KCP smoke test — DRY RUN"

echo "=== Deploy scaffold complete. Production steps require manual approval. ==="
