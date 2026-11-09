#!/usr/bin/env bash
set -euo pipefail
export VERIFY_EXPECTED_ADVIK="${VERIFY_EXPECTED_ADVIK:-a7d4f0c626369a3241aa168d241cdb2499e0f08d}"
export VERIFY_EXPECTED_DIYA="${VERIFY_EXPECTED_DIYA:-f704c79db0313f15b81c86986653fb284accaf94}"
cd "$(dirname "${BASH_SOURCE[0]}")/.."
GO_BIN="${GO_BIN:-go}"
if ! command -v "$GO_BIN" >/dev/null 2>&1; then
  if [[ -x ../tools/go/bin/go.exe ]]; then GO_BIN=../tools/go/bin/go.exe; else echo 'FAIL Go: install Go 1.26+ or set GO_BIN'; exit 1; fi
fi
export PATH="$(dirname "$(command -v "$GO_BIN")"):$PATH"
export CGO_ENABLED=0
export GOMAXPROCS="${GOMAXPROCS:-1}"
export GOFLAGS="${GOFLAGS:--p=1}"
export GOMEMLIMIT="${GOMEMLIMIT:-256MiB}"
export GOGC="${GOGC:-20}"
exec "$GO_BIN" run ./cmd/verifyintegration
