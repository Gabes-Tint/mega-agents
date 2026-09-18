#!/usr/bin/env bash
set -euo pipefail

binary=${1:-bin/mega-agents}
assets=${2:-internal/web/dist/assets}
# The page the browser loads names the scripts the first paint waits for; a
# chunk fetched later is not among them.
page=${3:-$(dirname "$assets")/index.html}
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
#
# 196 KiB, and it now measures the scripts index.html loads rather than every
# chunk on disk. CodeMirror moved into a chunk of its own, fetched when a
# field first shows an editor, which took the entry from 549933 to 197930
# bytes; the limit is the snug step above that, back near the 192 KiB the
# budget stood at before the editors landed. The whole bundle is unchanged
# and is held to its own budget below, so the drop is what the first paint
# stops paying for, not weight that went away.
max_javascript=${MAX_JAVASCRIPT_BYTES:-200704}
# 544 KiB: every chunk together, the figure this gate used to check. Splitting
# the editor out added 1901 bytes of chunk boilerplate (549933 to 551834), so
# the budget review last set stands, and a lazily fetched chunk still cannot
# grow without a deliberate re-baseline.
max_total_javascript=${MAX_TOTAL_JAVASCRIPT_BYTES:-557056}
max_css=${MAX_CSS_BYTES:-51200}

test -f "$binary" || { echo "Missing binary: $binary" >&2; exit 1; }
test -d "$assets" || { echo "Missing frontend assets: $assets" >&2; exit 1; }
test -f "$page" || { echo "Missing frontend page: $page" >&2; exit 1; }

binary_bytes=$(wc -c < "$binary")
# Concatenated rather than summed per file: `wc -c` on several files adds a
# total line of its own, which counted every byte twice once the build
# emitted a second chunk.
total_javascript_bytes=$(find "$assets" -type f -name '*.js' -exec cat {} + | wc -c)
css_bytes=$(find "$assets" -type f -name '*.css' -exec cat {} + | wc -c)

# Scripts the page names itself: the entry module and anything it preloads.
mapfile -t initial < <(grep -o '[^"'"'"']*\.js\b' "$page" | LC_ALL=C sort -u)
(( ${#initial[@]} > 0 )) || { echo "No scripts named by $page" >&2; exit 1; }
javascript_bytes=0
for script in "${initial[@]}"; do
  file="$(dirname "$page")/${script#/}"
  test -f "$file" || { echo "Missing script $script named by $page" >&2; exit 1; }
  javascript_bytes=$(( javascript_bytes + $(wc -c < "$file") ))
done

check_budget() {
  local label=$1 actual=$2 limit=$3
  if (( actual > limit )); then
    echo "$label is $actual bytes; budget is $limit bytes." >&2
    return 1
  fi
  printf '%s: %d / %d bytes\n' "$label" "$actual" "$limit"
}

check_budget 'Go binary' "$binary_bytes" "$max_binary"
check_budget 'Frontend JavaScript (first paint)' "$javascript_bytes" "$max_javascript"
check_budget 'Frontend JavaScript (all chunks)' "$total_javascript_bytes" "$max_total_javascript"
check_budget 'Frontend CSS' "$css_bytes" "$max_css"
