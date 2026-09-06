#!/usr/bin/env bash
# Shared, source-only helpers for all runtime scripts.
set -euo pipefail

PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
if [[ -f "$PROJECT_ROOT/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$PROJECT_ROOT/.env"
  set +a
fi
DEFAULT_NETWORK="$PROJECT_ROOT/fabric-runtime/test-network"
NETWORK_DIR="${FABRIC_TEST_NETWORK:-$DEFAULT_NETWORK}"

require_runtime() {
  if [[ ! -f "$NETWORK_DIR/network.sh" ]]; then
    echo "未找到 Fabric Test Network：$NETWORK_DIR" >&2
    echo "请在 .env 中设置 FABRIC_TEST_NETWORK=/绝对路径/test-network，或检查 fabric-runtime。" >&2
    exit 1
  fi
  if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then
    echo "Docker 不可用。请启动 Docker 服务，并确认当前用户有 docker 权限。" >&2
    exit 1
  fi
  if [[ ! -x "$NETWORK_DIR/../bin/peer" || ! -x "$NETWORK_DIR/../bin/configtxlator" ]]; then
    echo "缺少 Fabric 二进制文件（peer/configtxlator）：$NETWORK_DIR/../bin" >&2
    exit 1
  fi
}

channel_exists() { [[ -f "$NETWORK_DIR/channel-artifacts/mychannel.block" ]]; }
