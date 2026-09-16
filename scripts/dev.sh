#!/usr/bin/env bash
# Runs the full application in hot-reload mode: the Go backend rebuilds and
# restarts on Go source changes (air), and the frontend dev server hot-reloads
# Svelte changes and proxies /api to the backend. Either process exiting stops
# both. The backend dev port defaults to the first free port in 8180-8189 and
# can be forced with MEGA_AGENTS_DEV_PORT.
set -uo pipefail

root=$(git rev-parse --show-toplevel)
cd "$root"

AIR=$(go env GOPATH)/bin/air
if ! test -x "$AIR"; then
  echo 'Run make install first (air is missing).' >&2
  exit 1
fi
if ! test -d frontend/node_modules; then
  echo 'Run make install first (frontend dependencies are missing).' >&2
  exit 1
fi
if ! test -d internal/web/dist; then
  echo 'Embedded assets are missing; building them once before starting...' >&2
  make frontend >/dev/null || exit 1
fi

if test -n "${MEGA_AGENTS_DEV_PORT:-}"; then
  port=$MEGA_AGENTS_DEV_PORT
else
  port=""
  for candidate in 8180 8181 8182 8183 8184 8185 8186 8187 8188 8189; do
    if ! ss -ltn "sport = :$candidate" | grep -q LISTEN; then
      port=$candidate
      break
    fi
  done
  if test -z "$port"; then
    echo 'No free backend port in 8180-8189; set MEGA_AGENTS_DEV_PORT.' >&2
    exit 1
  fi
fi

export PORT=$port
export MEGA_AGENTS_API="http://localhost:$port"

pids=()
cleanup() {
  trap - EXIT INT TERM
  for pid in "${pids[@]:-}"; do
    # Children run with setsid, so killing the process group reaches vite's
    # node process (bun's child) and air's built binary as well.
    kill -- "-$pid" 2>/dev/null || kill "$pid" 2>/dev/null || true
  done
  wait 2>/dev/null || true
}
trap cleanup EXIT INT TERM

echo "Backend dev port: $port"
setsid "$AIR" -c .air.toml &
pids+=($!)
setsid bash -c 'cd frontend && exec bun run dev -- --host' &
pids+=($!)

wait -n
status=$?
echo "A dev process exited (status ${status}); stopping the other." >&2
exit $status
