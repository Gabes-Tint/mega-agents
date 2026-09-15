#!/usr/bin/env bash
set -euo pipefail

port=$((20000 + $$ % 20000))
log=$(mktemp "${TMPDIR:-/tmp}/mega-agents-smoke.XXXXXX")
PORT=$port ./bin/mega-agents >"$log" 2>&1 &
server_pid=$!
trap 'kill "$server_pid" 2>/dev/null || true; rm -f -- "$log"' EXIT

for _ in {1..50}; do
  if response=$(curl --fail --silent "http://127.0.0.1:$port/api/status"); then
    break
  fi
  sleep 0.1
done

test "${response:-}" = '{"message":"Mega Agents backend is running"}' || {
  echo 'Status endpoint smoke test failed.' >&2
  cat "$log" >&2
  exit 1
}
curl --fail --silent "http://127.0.0.1:$port/" | rg -q '<title>Mega Agents</title>'
echo 'Embedded application smoke test passed.'
