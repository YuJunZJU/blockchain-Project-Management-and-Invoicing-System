#!/usr/bin/env bash
# Lightweight, offline file-layout verification for the course submission package.
set -euo pipefail

PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
required=(
  "backend/main.go"
  "chaincode/invoice_contract.go"
  "web/index.html"
  "fabric-runtime/test-network/network.sh"
  "fabric-runtime/test-network/channel-artifacts/mychannel.block"
  "backend/data/accounts.json"
  ".env"
  "ledger-state/peer0.org1.production.tar.gz"
  "ledger-state/peer0.org2.production.tar.gz"
  "ledger-state/orderer.production.tar.gz"
  "作业提交运行说明.md"
)

missing=0
for path in "${required[@]}"; do
  if [[ ! -e "$PROJECT_ROOT/$path" ]]; then
    echo "缺少：$path" >&2
    missing=1
  fi
done
if [[ $missing -ne 0 ]]; then
  exit 1
fi

if [[ -f "$PROJECT_ROOT/ledger-state/SHA256SUMS" ]]; then
  (cd "$PROJECT_ROOT/ledger-state" && sha256sum -c SHA256SUMS)
fi

echo "提交包文件检查通过。"
