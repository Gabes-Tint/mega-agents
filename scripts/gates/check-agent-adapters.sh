#!/usr/bin/env bash
set -euo pipefail

root=${1:-$(git rev-parse --show-toplevel)}
skills=(
  feature-development
  diagnosing-bugs
  writing-agent-instructions
  code-review
  agent-handoff
)

test -d "$root/.agents/skills" || {
  echo 'Missing canonical skill directory: .agents/skills' >&2
  exit 1
}

test -d "$root/.claude/skills" || {
  echo 'Missing Claude-compatible skill directory: .claude/skills' >&2
  exit 1
}

grep -Fxq '@AGENTS.md' "$root/CLAUDE.md" || {
  echo 'CLAUDE.md must import AGENTS.md.' >&2
  exit 1
}

expected=$(printf '%s\n' "${skills[@]}" | LC_ALL=C sort)
canonical=$(find "$root/.agents/skills" -mindepth 2 -maxdepth 2 -type f \
  -name SKILL.md -printf '%h\n' | sed 's#.*/##' | LC_ALL=C sort)
claude=$(find "$root/.claude/skills" -mindepth 2 -maxdepth 2 -type f \
  -name SKILL.md -printf '%h\n' | sed 's#.*/##' | LC_ALL=C sort)

[[ $canonical == "$expected" ]] || {
  echo 'Canonical skill inventory differs from the supported skill inventory.' >&2
  exit 1
}

[[ $claude == "$expected" ]] || {
  echo 'Claude adapter inventory differs from the supported skill inventory.' >&2
  exit 1
}

for skill in "${skills[@]}"; do
  canonical_skill=".agents/skills/$skill/SKILL.md"
  claude_skill=".claude/skills/$skill/SKILL.md"

  test -f "$root/$canonical_skill" || {
    echo "Missing canonical skill: $canonical_skill" >&2
    exit 1
  }

  grep -Fxq "name: $skill" "$root/$canonical_skill" || {
    echo "Canonical skill name does not match its directory: $canonical_skill" >&2
    exit 1
  }

  grep -Fq "($canonical_skill)" "$root/AGENTS.md" || {
    echo "AGENTS.md must reference the canonical $skill skill." >&2
    exit 1
  }

  test -f "$root/$claude_skill" || {
    echo "Missing Claude-compatible adapter: $claude_skill" >&2
    exit 1
  }

  grep -Fxq "name: $skill" "$root/$claude_skill" || {
    echo "Claude adapter name does not match its directory: $claude_skill" >&2
    exit 1
  }

  grep -Fq "../../../$canonical_skill" "$root/$claude_skill" || {
    echo "Claude adapter must reference its canonical skill: $claude_skill" >&2
    exit 1
  }
done
