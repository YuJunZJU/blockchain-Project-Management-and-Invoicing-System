#!/usr/bin/env bash
# Restore a packaged Fabric ledger only into empty Docker volumes.
# This safety restriction prevents accidental replacement of an existing ledger.
set -euo pipefail

PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
STATE_DIR=${1:-"$PROJECT_ROOT/ledger-state"}
HELPER_IMAGE=${LEDGER_STATE_HELPER_IMAGE:-busybox:latest}

if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then
  echo "Docker 不可用。请启动 Docker 并确认当前用户有 docker 权限。" >&2
  exit 1
fi
if [[ ! -d "$STATE_DIR" ]]; then
  echo "未找到账本状态目录：$STATE_DIR" >&2
  exit 1
fi
for archive in peer0.org1.production.tar.gz peer0.org2.production.tar.gz orderer.production.tar.gz; do
  if [[ ! -f "$STATE_DIR/$archive" ]]; then
    echo "账本备份不完整，缺少：$STATE_DIR/$archive" >&2
    exit 1
  fi
done

if docker ps --format '{{.Names}}' | grep -Eq '^(peer0\.org1\.example\.com|peer0\.org2\.example\.com|orderer\.example\.com)$'; then
  echo "检测到 Fabric 节点正在运行。请先停止节点；为保护已有账本，本脚本不会覆盖正在使用的数据卷。" >&2
  exit 1
fi

is_empty_volume() {
  local volume=$1
  docker run --rm -v "$volume:/target" "$HELPER_IMAGE" sh -c 'test -z "$(ls -A /target)"'
}

restore_volume() {
  local volume=$1 archive=$2
  if ! docker volume inspect "$volume" >/dev/null 2>&1; then
    docker volume create "$volume" >/dev/null
  fi
  if ! is_empty_volume "$volume"; then
    echo "目标 Docker 卷 $volume 已包含数据。为保护已有账本，恢复已取消。" >&2
    echo "请在全新的 Docker 环境中运行，或由维护者确认后手动处理旧测试网络。" >&2
    exit 1
  fi
  echo "正在恢复 $volume"
  docker run --rm \
    -v "$volume:/target" \
    -v "$STATE_DIR:/backup:ro" \
    "$HELPER_IMAGE" \
    tar -C /target -xzf "/backup/$archive"
}

if [[ -f "$STATE_DIR/SHA256SUMS" ]]; then
  echo "正在校验账本备份完整性"
  (cd "$STATE_DIR" && sha256sum -c SHA256SUMS)
fi

restore_volume "compose_peer0.org1.example.com" "peer0.org1.production.tar.gz"
restore_volume "compose_peer0.org2.example.com" "peer0.org2.production.tar.gz"
restore_volume "compose_orderer.example.com" "orderer.production.tar.gz"

echo "账本状态恢复完成。下一步执行：./scripts/start-network.sh resume"
