#!/usr/bin/env bash
set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib-runtime.sh"
export FABRIC_TEST_NETWORK="$NETWORK_DIR"
require_runtime

if ! command -v go >/dev/null 2>&1; then
  echo "未找到 Go。请先安装 Go 1.23 或更高版本。" >&2
  exit 1
fi

cd "$PROJECT_ROOT/backend"
go run .
