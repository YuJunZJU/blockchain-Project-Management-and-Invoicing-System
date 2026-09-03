#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
DEFAULT_NETWORK="$PROJECT_ROOT/fabric-runtime/test-network"
NETWORK_DIR="${FABRIC_TEST_NETWORK:-$DEFAULT_NETWORK}"

if [[ ! -f "$NETWORK_DIR/network.sh" ]]; then
  echo "未找到 Fabric Test Network：$NETWORK_DIR" >&2
  echo "请设置 FABRIC_TEST_NETWORK=/绝对路径/fabric-samples/test-network" >&2
  exit 1
fi

FABRIC_BIN_DIR=$(cd "$NETWORK_DIR/../bin" 2>/dev/null && pwd || true)
if [[ -z "$FABRIC_BIN_DIR" || ! -x "$FABRIC_BIN_DIR/configtxlator" ]]; then
  echo "缺少 Fabric 二进制：$NETWORK_DIR/../bin/configtxlator" >&2
  echo "请依据 fabric-runtime/README.md 补齐本地 Fabric 运行环境。" >&2
  exit 1
fi

# Test Network 在生成连接配置与锚节点更新时会直接执行此脚本。
chmod +x "$NETWORK_DIR/organizations/ccp-generate.sh"

cd "$NETWORK_DIR"
# 课程项目使用 test-network 自带的 cryptogen 生成测试证书。
# 不使用 -ca：这样无需依赖 fabric-ca-client，也可避免本地 CA 客户端与
# Docker 中 CA 镜像版本不同导致的首次启动失败。
bash ./network.sh up createChannel -c mychannel
