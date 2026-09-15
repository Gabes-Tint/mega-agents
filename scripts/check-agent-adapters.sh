#!/usr/bin/env bash
set -euo pipefail

root=${1:-$(git rev-parse --show-toplevel)}
skill='.agents/skills/feature-development/SKILL.md'
claude_skill='.claude/skills/feature-development/SKILL.md'

test -f "$root/$skill" || {
  echo "Missing canonical feature-development skill: $skill" >&2
  exit 1
}

grep -Fq '(.agents/skills/feature-development/SKILL.md)' "$root/AGENTS.md" || {
  echo 'AGENTS.md must reference the canonical feature-development skill.' >&2
  exit 1
}

grep -Fxq '@AGENTS.md' "$root/CLAUDE.md" || {
  echo 'CLAUDE.md must import AGENTS.md.' >&2
  exit 1
}

test -f "$root/$claude_skill" || {
  echo "Missing Claude-compatible feature-development adapter: $claude_skill" >&2
  exit 1
}

grep -Fq '../../../.agents/skills/feature-development/SKILL.md' "$root/$claude_skill" || {
  echo 'The Claude-compatible skill must reference the canonical skill.' >&2
  exit 1
}
