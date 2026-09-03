#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
DEFAULT_NETWORK="$PROJECT_ROOT/../../lab7/fabric-resources/fabric-resources/fabric-samples/test-network"
NETWORK_DIR="${FABRIC_TEST_NETWORK:-$DEFAULT_NETWORK}"

if [[ ! -f "$NETWORK_DIR/network.sh" ]]; then
  echo "未找到 Fabric Test Network：$NETWORK_DIR" >&2
  echo "请设置 FABRIC_TEST_NETWORK=/绝对路径/fabric-samples/test-network" >&2
  exit 1
fi

cd "$NETWORK_DIR"
bash ./network.sh up createChannel -c mychannel -ca
