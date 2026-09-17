#!/usr/bin/env bash
set -euo pipefail

binary=${1:-bin/mega-agents}
assets=${2:-internal/web/dist/assets}
max_binary=${MAX_BINARY_BYTES:-16777216}
# 140 KiB: the graph workspace added an SVG edge layer and node interaction
# logic, then executable blocks (Git actions, agents, validation, routing) with
# their property editors and run views; re-baselined deliberately in review
# rather than trimming features.
max_javascript=${MAX_JAVASCRIPT_BYTES:-143360}
max_css=${MAX_CSS_BYTES:-51200}

test -f "$binary" || { echo "Missing binary: $binary" >&2; exit 1; }
test -d "$assets" || { echo "Missing frontend assets: $assets" >&2; exit 1; }

binary_bytes=$(wc -c < "$binary")
javascript_bytes=$(find "$assets" -type f -name '*.js' -exec wc -c {} + | awk '{total += $1} END {print total + 0}')
css_bytes=$(find "$assets" -type f -name '*.css' -exec wc -c {} + | awk '{total += $1} END {print total + 0}')

check_budget() {
  local label=$1 actual=$2 limit=$3
  if (( actual > limit )); then
    echo "$label is $actual bytes; budget is $limit bytes." >&2
    return 1
  fi
  printf '%s: %d / %d bytes\n' "$label" "$actual" "$limit"
}

check_budget 'Go binary' "$binary_bytes" "$max_binary"
check_budget 'Frontend JavaScript' "$javascript_bytes" "$max_javascript"
check_budget 'Frontend CSS' "$css_bytes" "$max_css"
