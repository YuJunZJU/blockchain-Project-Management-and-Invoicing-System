#!/usr/bin/env bash
# Usage: ./scripts/upgrade-chaincode-v2.6.sh [current-sequence]
# A freshly deployed v2.4 network uses Sequence 1; a network previously
# upgraded to v2.5 uses Sequence 2. Fabric requires the next exact sequence.
set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib-runtime.sh"
require_runtime
if ! channel_exists; then
  echo "未检测到 mychannel；无法升级链码。" >&2
  exit 1
fi

CURRENT_SEQUENCE=${1:-1}
if ! [[ "$CURRENT_SEQUENCE" =~ ^[1-9][0-9]*$ ]]; then
  echo "当前 Sequence 必须是正整数，例如：$0 1" >&2
  exit 2
fi
NEXT_SEQUENCE=$((CURRENT_SEQUENCE + 1))
cd "$NETWORK_DIR"
echo "升级 invoice 链码：当前 Sequence ${CURRENT_SEQUENCE} → Version 2.6 / Sequence ${NEXT_SEQUENCE}"
bash ./network.sh deployCC -c mychannel -ccn invoice -ccp "$PROJECT_ROOT/chaincode" -ccl go -ccv 2.6 -ccs "$NEXT_SEQUENCE"
