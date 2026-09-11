#!/usr/bin/env bash
# Export the Fabric production volumes used by this course project.
# The command is read-only with respect to the existing ledger volumes.
set -euo pipefail

PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
OUTPUT_DIR=${1:-"$PROJECT_ROOT/ledger-state"}
HELPER_IMAGE=${LEDGER_STATE_HELPER_IMAGE:-busybox:latest}

if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then
  echo "Docker 不可用。请启动 Docker 并确认当前用户有 docker 权限。" >&2
  exit 1
fi

mkdir -p "$OUTPUT_DIR"

backup_volume() {
  local volume=$1 archive=$2
  if ! docker volume inspect "$volume" >/dev/null 2>&1; then
    echo "未找到 Fabric 账本卷：$volume" >&2
    exit 1
  fi
  echo "正在备份 $volume"
  docker run --rm \
    -v "$volume:/source:ro" \
    -v "$OUTPUT_DIR:/backup" \
    "$HELPER_IMAGE" \
    tar -C /source -czf "/backup/$archive" .
}

backup_volume "compose_peer0.org1.example.com" "peer0.org1.production.tar.gz"
backup_volume "compose_peer0.org2.example.com" "peer0.org2.production.tar.gz"
backup_volume "compose_orderer.example.com" "orderer.production.tar.gz"

(
  cd "$OUTPUT_DIR"
  sha256sum peer0.org1.production.tar.gz peer0.org2.production.tar.gz orderer.production.tar.gz > SHA256SUMS
)

echo "账本状态备份完成：$OUTPUT_DIR"
