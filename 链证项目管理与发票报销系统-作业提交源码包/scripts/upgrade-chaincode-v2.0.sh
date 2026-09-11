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
echo "升级 invoice 链码：Version 1.9 / Sequence 1 → Version 2.0 / Sequence 2"
bash ./network.sh deployCC -c mychannel -ccn invoice -ccp "$PROJECT_ROOT/chaincode" -ccl go -ccv 2.0 -ccs 2
