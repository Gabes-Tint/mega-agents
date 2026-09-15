#!/usr/bin/env bash
set -euo pipefail

root=$(git rev-parse --show-toplevel)
runner="$root/scripts/run-with-status.sh"
fixture=$(mktemp -d "${TMPDIR:-/tmp}/mega-agents-status.XXXXXX")
trap 'rm -rf -- "$fixture"' EXIT

"$runner" 'Example check' sh -c 'printf command-output' \
  >"$fixture/success.out" 2>"$fixture/success.err"
grep -Fq 'command-output' "$fixture/success.out"
grep -Fq '✅ Example check' "$fixture/success.out"
test ! -s "$fixture/success.err"

set +e
"$runner" 'Broken check' sh -c 'printf failure-output >&2; exit 7' \
  >"$fixture/failure.out" 2>"$fixture/failure.err"
status=$?
set -e

test "$status" -eq 7
grep -Fq 'failure-output' "$fixture/failure.err"
grep -Fq '❌ Broken check' "$fixture/failure.err"
test ! -s "$fixture/failure.out"
