#!/usr/bin/env bash
set -euo pipefail

fixture=$(mktemp -d "${TMPDIR:-/tmp}/mega-agents-gates.XXXXXX")
trap 'rm -r -- "$fixture"' EXIT
mkdir -p "$fixture/frontend/src" "$fixture/assets"
printf 'test.only("bad", () => {});\n' > "$fixture/frontend/src/bad.test.ts"
if bash scripts/gates/check-test-policy.sh "$fixture" >/dev/null 2>&1; then
  echo 'Test-policy gate accepted a focused test.' >&2
  exit 1
fi

truncate -s 2 "$fixture/app"
truncate -s 2 "$fixture/assets/app.js"
truncate -s 2 "$fixture/assets/app.css"
if MAX_BINARY_BYTES=1 bash scripts/gates/check-artifact-sizes.sh "$fixture/app" "$fixture/assets" \
  >/dev/null 2>&1; then
  echo 'Artifact-size gate accepted an oversized binary.' >&2
  exit 1
fi

agent_fixture="$fixture/agent-adapters"
agent_skills=(
  feature-development
  diagnosing-bugs
  writing-agent-instructions
  code-review
  agent-handoff
)

create_agent_fixture() {
  if [[ -d $agent_fixture ]]; then
    rm -r -- "$agent_fixture"
  fi
  mkdir -p "$agent_fixture/.agents/skills" "$agent_fixture/.claude/skills"
  printf '@AGENTS.md\n' > "$agent_fixture/CLAUDE.md"
  : > "$agent_fixture/AGENTS.md"
  for skill in "${agent_skills[@]}"; do
    mkdir -p \
      "$agent_fixture/.agents/skills/$skill" \
      "$agent_fixture/.claude/skills/$skill"
    printf '%s\n' '---' "name: $skill" '---' \
      > "$agent_fixture/.agents/skills/$skill/SKILL.md"
    printf '[%s](.agents/skills/%s/SKILL.md)\n' "$skill" "$skill" \
      >> "$agent_fixture/AGENTS.md"
    printf '%s\n' '---' "name: $skill" '---' \
      "Read ../../../.agents/skills/$skill/SKILL.md" \
      > "$agent_fixture/.claude/skills/$skill/SKILL.md"
  done
}

expect_agent_adapter_rejection() {
  local message=$1
  if bash scripts/gates/check-agent-adapters.sh "$agent_fixture" >/dev/null 2>&1; then
    echo "$message" >&2
    exit 1
  fi
}

create_agent_fixture
bash scripts/gates/check-agent-adapters.sh "$agent_fixture"

rm "$agent_fixture/.agents/skills/diagnosing-bugs/SKILL.md"
expect_agent_adapter_rejection 'Agent-adapter gate accepted a missing canonical skill.'

create_agent_fixture
sed -i 's/name: diagnosing-bugs/name: wrong-name/' \
  "$agent_fixture/.agents/skills/diagnosing-bugs/SKILL.md"
expect_agent_adapter_rejection 'Agent-adapter gate accepted mismatched canonical metadata.'

create_agent_fixture
sed -i '/diagnosing-bugs/d' "$agent_fixture/AGENTS.md"
expect_agent_adapter_rejection 'Agent-adapter gate accepted a missing AGENTS.md skill pointer.'

create_agent_fixture
printf 'Claude instructions without the shared import.\n' > "$agent_fixture/CLAUDE.md"
expect_agent_adapter_rejection 'Agent-adapter gate accepted a missing CLAUDE.md import.'

create_agent_fixture
rm "$agent_fixture/.claude/skills/code-review/SKILL.md"
expect_agent_adapter_rejection 'Agent-adapter gate accepted a missing Claude adapter.'

create_agent_fixture
sed -i 's/name: code-review/name: wrong-name/' \
  "$agent_fixture/.claude/skills/code-review/SKILL.md"
expect_agent_adapter_rejection 'Agent-adapter gate accepted mismatched adapter metadata.'

create_agent_fixture
sed -i 's#../../../.agents/skills/code-review/SKILL.md#../../../.agents/skills/feature-development/SKILL.md#' \
  "$agent_fixture/.claude/skills/code-review/SKILL.md"
expect_agent_adapter_rejection 'Agent-adapter gate accepted a broken canonical pointer.'

create_agent_fixture
mkdir -p "$agent_fixture/.agents/skills/unregistered-skill"
printf '%s\n' '---' 'name: unregistered-skill' '---' \
  > "$agent_fixture/.agents/skills/unregistered-skill/SKILL.md"
expect_agent_adapter_rejection 'Agent-adapter gate accepted an unregistered canonical skill.'

create_agent_fixture
mkdir -p "$agent_fixture/.claude/skills/unregistered-skill"
printf '%s\n' 'Unexpected adapter.' \
  > "$agent_fixture/.claude/skills/unregistered-skill/SKILL.md"
expect_agent_adapter_rejection 'Agent-adapter gate accepted an unregistered Claude adapter.'

ci_fixture="$fixture/ci-documentation"
mkdir -p "$ci_fixture/.github/workflows" "$ci_fixture/docs" "$ci_fixture/scripts/gates"
touch "$ci_fixture/docs/ci-doc.md"
printf 'verify:\n\t$(MAKE) gates\ngates: binary test-policy agent-adapters ci-documentation size-check\n' > "$ci_fixture/Makefile"
printf 'name: CI\nrun: make verify\nrun: make gate-self-test\nrun: make smoke\n' \
  > "$ci_fixture/.github/workflows/ci.yml"
printf 'name: Dependency audit\ncron: weekly\nrun: bun audit\nrun: govulncheck ./...\n' \
  > "$ci_fixture/.github/workflows/dependency-audit.yml"
printf 'workflow_dispatch:\n' >> "$ci_fixture/.github/workflows/dependency-audit.yml"
cp scripts/gates/check-ci-documentation.sh "$ci_fixture/scripts/gates/"
bash "$ci_fixture/scripts/gates/check-ci-documentation.sh" --write-facts "$ci_fixture"

CI_CHANGED_FILES=$'.github/workflows/ci.yml\ndocs/ci-doc.md' \
  bash scripts/gates/check-ci-documentation.sh "$ci_fixture"

if CI_CHANGED_FILES='.github/workflows/ci.yml' \
  bash scripts/gates/check-ci-documentation.sh "$ci_fixture" >/dev/null 2>&1; then
  echo 'CI-documentation gate accepted a CI change without documentation.' >&2
  exit 1
fi

printf '%s\n' 'CI_DOCUMENTATION_NO_IMPACT_V1' \
  'reason=This workflow-only refactor preserves every documented CI behavior.' \
  'deadbeef  .github/workflows/ci.yml' > "$ci_fixture/.ci-doc-no-impact"
if CI_CHANGED_FILES=$'.github/workflows/ci.yml\n.ci-doc-no-impact' \
  bash scripts/gates/check-ci-documentation.sh "$ci_fixture" >/dev/null 2>&1; then
  echo 'CI-documentation gate accepted an invalid no-impact declaration.' >&2
  exit 1
fi

workflow_hash=$(sha256sum "$ci_fixture/.github/workflows/ci.yml" | awk '{print $1}')
printf '%s\n' 'CI_DOCUMENTATION_NO_IMPACT_V1' \
  'reason=This workflow-only refactor preserves every documented CI behavior.' \
  "$workflow_hash  .github/workflows/ci.yml" > "$ci_fixture/.ci-doc-no-impact"
CI_CHANGED_FILES=$'.github/workflows/ci.yml\n.ci-doc-no-impact' \
  bash scripts/gates/check-ci-documentation.sh "$ci_fixture"

sed -i 's/Required verification entry point/Incorrect verification entry point/' \
  "$ci_fixture/docs/ci-doc.md"
if CI_CHANGED_FILES=$'.github/workflows/ci.yml\ndocs/ci-doc.md' \
  bash scripts/gates/check-ci-documentation.sh "$ci_fixture" >/dev/null 2>&1; then
  echo 'CI-documentation gate accepted factual mismatch after a documentation update.' >&2
  exit 1
fi

echo 'Custom gate rejection fixtures passed.'
