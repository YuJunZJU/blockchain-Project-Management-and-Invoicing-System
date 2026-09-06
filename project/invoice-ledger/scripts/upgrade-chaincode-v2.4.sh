#!/usr/bin/env bash
set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib-runtime.sh"
require_runtime
if ! channel_exists; then echo "未检测到 mychannel；无法升级链码。" >&2; exit 1; fi

cd "$NETWORK_DIR"
echo "升级 invoice 链码：Version 2.3 / Sequence 5 → Version 2.4 / Sequence 6"
bash ./network.sh deployCC -c mychannel -ccn invoice -ccp "$PROJECT_ROOT/chaincode" -ccl go -ccv 2.4 -ccs 6
