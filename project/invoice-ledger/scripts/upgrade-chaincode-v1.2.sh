#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
DEFAULT_NETWORK="$PROJECT_ROOT/fabric-runtime/test-network"
NETWORK_DIR="${FABRIC_TEST_NETWORK:-$DEFAULT_NETWORK}"

if [[ ! -f "$NETWORK_DIR/network.sh" ]]; then
  echo "未找到 Fabric Test Network：$NETWORK_DIR" >&2
  exit 1
fi

cd "$NETWORK_DIR"
echo "修复 invoice 链码响应字段：Version 1.1 / Sequence 2 → Version 1.2 / Sequence 3"
bash ./network.sh deployCC -c mychannel -ccn invoice -ccp "$PROJECT_ROOT/chaincode" -ccl go -ccv 1.2 -ccs 3
