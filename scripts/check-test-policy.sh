#!/usr/bin/env bash
set -euo pipefail
root=${1:-$(git rev-parse --show-toplevel)}

if rg -n \
  --glob '*.test.js' --glob '*.test.ts' \
  --glob '*.spec.js' --glob '*.spec.ts' \
  '\b(describe|it|test)\.(only|skip|todo)\s*\(' "$root/frontend/src"; then
  echo 'Focused, skipped, and placeholder frontend tests are not allowed.' >&2
  exit 1
fi

if rg -n --glob '*_test.go' '\.(Skip|Skipf|SkipNow)\s*\(' "$root" \
  --glob '!frontend/node_modules/**'; then
  echo 'Skipped Go tests are not allowed.' >&2
  exit 1
fi
