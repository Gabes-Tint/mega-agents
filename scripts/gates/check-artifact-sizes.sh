#!/usr/bin/env bash
set -euo pipefail

binary=${1:-bin/mega-agents}
assets=${2:-internal/web/dist/assets}
# 32 MiB: the Router block embeds CEL (cel.dev/cel-go), whose protobuf and
# ANTLR runtime take the binary from 12 MiB to about 24 MiB. The development
# notes choose CEL for type-checked, sandboxed routing expressions over a
# hand-rolled language; re-baselined deliberately in review.
max_binary=${MAX_BINARY_BYTES:-33554432}
# 528 KiB: the graph workspace added an SVG edge layer and node interaction
# logic, then executable blocks (Git actions, agents, validation, routing) with
# their property editors and run views, then the editor redesign (themes,
# block deletion, palette explanations, a problems panel), then draw.io style
# arrows (drag handles, output menu, arrow selection, deletion and
# reconnection) and agent health logos with their inline brand marks in the
# status bar, and finally CodeMirror 6 editors for the command, prompt and
# schema fields. That last step costs about 334 KiB on its own: it was chosen
# with the product owner over a hand-rolled highlighter so the fields get a
# real editor (shell and JSON syntax, undo, bracket matching, soft wrap) and
# completion of the variables that reach the block, which a bespoke textarea
# overlay would have to reimplement and keep correct. Canvas zoom then took
# the bundle to 540437 bytes and raised the limit to 544 KiB, which it did
# not need: the build still fit under 528 KiB. Colouring each arrow with the
# hue of the block it leaves replaced three arrowheads with one and left the
# bundle at 540414 bytes, so the limit went back to the snug step above it.
# 544 KiB again: the breakpoint editor took the bundle to 545874 bytes, for
# the marks a block carries, the paused panel listing every block a run waits
# before with the values that reached it, and a code editor for the prompt or
# command that block would run, which the resume endpoint takes back.
# Re-baselined deliberately in review rather than trimming features.
max_javascript=${MAX_JAVASCRIPT_BYTES:-557056}
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
