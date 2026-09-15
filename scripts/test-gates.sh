#!/usr/bin/env bash
set -euo pipefail

fixture=$(mktemp -d "${TMPDIR:-/tmp}/mega-agents-gates.XXXXXX")
trap 'rm -r -- "$fixture"' EXIT
mkdir -p "$fixture/frontend/src" "$fixture/assets"
printf 'test.only("bad", () => {});\n' > "$fixture/frontend/src/bad.test.ts"
if bash scripts/check-test-policy.sh "$fixture" >/dev/null 2>&1; then
  echo 'Test-policy gate accepted a focused test.' >&2
  exit 1
fi

truncate -s 2 "$fixture/app"
truncate -s 2 "$fixture/assets/app.js"
truncate -s 2 "$fixture/assets/app.css"
if MAX_BINARY_BYTES=1 bash scripts/check-artifact-sizes.sh "$fixture/app" "$fixture/assets" \
  >/dev/null 2>&1; then
  echo 'Artifact-size gate accepted an oversized binary.' >&2
  exit 1
fi

mkdir -p "$fixture/.agents/skills/feature-development"
touch "$fixture/.agents/skills/feature-development/SKILL.md"
printf '%s\n' \
  '[feature workflow](.agents/skills/feature-development/SKILL.md)' \
  > "$fixture/AGENTS.md"
printf '@AGENTS.md\n' > "$fixture/CLAUDE.md"
mkdir -p "$fixture/.claude/skills/feature-development"
printf '%s\n' 'Adapter missing its canonical reference.' \
  > "$fixture/.claude/skills/feature-development/SKILL.md"
if bash scripts/check-agent-adapters.sh "$fixture" >/dev/null 2>&1; then
  echo 'Agent-adapter gate accepted a broken skill adapter.' >&2
  exit 1
fi

echo 'Custom gate rejection fixtures passed.'
