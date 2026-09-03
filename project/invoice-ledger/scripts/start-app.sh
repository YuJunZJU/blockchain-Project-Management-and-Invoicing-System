#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
DEFAULT_NETWORK="$PROJECT_ROOT/../../lab7/fabric-resources/fabric-resources/fabric-samples/test-network"
export FABRIC_TEST_NETWORK="${FABRIC_TEST_NETWORK:-$DEFAULT_NETWORK}"

if ! command -v go >/dev/null 2>&1; then
  echo "未找到 Go。请先安装 Go 1.23 或更高版本。" >&2
  exit 1
fi

cd "$PROJECT_ROOT/backend"
go run .
