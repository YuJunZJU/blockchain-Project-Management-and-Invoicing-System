#!/usr/bin/env bash
set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib-runtime.sh"

ACTION=${1:-}
if [[ "$ACTION" != "init" && "$ACTION" != "resume" && "$ACTION" != "check" ]]; then
  echo "用法：$0 {check|init|resume}" >&2
  echo "  check  检查 Docker、Fabric 二进制和配置" >&2
  echo "  init   仅首次：创建测试网络和 mychannel" >&2
  echo "  resume 正常恢复：只启动已有网络，不创建/重复加入通道" >&2
  exit 2
fi
require_runtime
if [[ "$ACTION" == "check" ]]; then echo "环境检查通过：$NETWORK_DIR"; exit 0; fi

# Test Network 在生成连接配置与锚节点更新时会直接执行此脚本。
chmod +x "$NETWORK_DIR/organizations/ccp-generate.sh"

cd "$NETWORK_DIR"
if [[ "$ACTION" == "init" ]]; then
  if channel_exists; then echo "检测到已有 mychannel；请使用 ./scripts/start-network.sh resume。" >&2; exit 1; fi
  bash ./network.sh up createChannel -c mychannel
else
  if ! channel_exists; then echo "未检测到 mychannel；请先执行 ./scripts/start-network.sh init。" >&2; exit 1; fi
  bash ./network.sh up
  echo "Fabric 网络已恢复；已有通道 mychannel 未被重复创建。"
fi
