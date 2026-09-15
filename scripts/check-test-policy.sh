#!/usr/bin/env bash
set -euo pipefail
root=${1:-$(git rev-parse --show-toplevel)}

if grep -ERn \
  --include='*.test.js' --include='*.test.ts' \
  --include='*.spec.js' --include='*.spec.ts' \
  '(describe|it|test)\.(only|skip|todo)[[:space:]]*\(' "$root/frontend/src"; then
  echo 'Focused, skipped, and placeholder frontend tests are not allowed.' >&2
  exit 1
fi

if grep -ERn --include='*_test.go' --exclude-dir=node_modules \
  '\.(Skip|Skipf|SkipNow)[[:space:]]*\(' "$root"; then
  echo 'Skipped Go tests are not allowed.' >&2
  exit 1
fi
