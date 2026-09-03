#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
DEFAULT_NETWORK="$PROJECT_ROOT/fabric-runtime/test-network"
export FABRIC_TEST_NETWORK="${FABRIC_TEST_NETWORK:-$DEFAULT_NETWORK}"

# Optional local configuration for OCR and AI correction. The real .env file
# must stay local; .env.example is safe to commit as a template.
if [[ -f "$PROJECT_ROOT/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$PROJECT_ROOT/.env"
  set +a
fi

if ! command -v go >/dev/null 2>&1; then
  echo "未找到 Go。请先安装 Go 1.23 或更高版本。" >&2
  exit 1
fi

cd "$PROJECT_ROOT/backend"
go run .
