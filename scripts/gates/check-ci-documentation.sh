#!/usr/bin/env bash
set -euo pipefail

root=${2:-${1:-$(git rev-parse --show-toplevel)}}
mode=${1:-check}
doc="$root/docs/ci-doc.md"
declaration="$root/.ci-doc-no-impact"

require_text() {
  local file=$1 text=$2 message=$3
  grep -Fq "$text" "$root/$file" || { echo "$message" >&2; exit 1; }
}

facts() {
  cat <<'EOF'
<!-- ci-facts:start -->
| Machine-checked fact | Configuration |
| --- | --- |
| Required verification entry point | `make verify` |
| Repository policy entry point | `make gates` (test policy, agent adapters, CI-documentation drift, artifact sizes) |
| Custom gate rejection tests | `make gate-self-test` |
| Compiled application smoke test | `make smoke` |
| Required CI workflows | `CI` (Verify + Security), `Dependency audit` (weekly + manual) |
| Required vulnerability checks | `bun audit`, `govulncheck ./...` |
<!-- ci-facts:end -->
EOF
}

validate_configuration() {
  require_text Makefile '$(MAKE) gates' 'Makefile verify must invoke gates.'
  require_text Makefile 'gates: binary test-policy agent-adapters ci-documentation size-check' 'Makefile gates target is incomplete.'
  require_text .github/workflows/ci.yml 'run: make verify' 'CI must run make verify.'
  require_text .github/workflows/ci.yml 'run: make gate-self-test' 'CI must run custom gate self-tests.'
  require_text .github/workflows/ci.yml 'run: make smoke' 'CI must run the compiled-application smoke test.'
  require_text .github/workflows/dependency-audit.yml 'cron:' 'Dependency audit must have a schedule.'
  require_text .github/workflows/dependency-audit.yml 'workflow_dispatch:' 'Dependency audit must support manual runs.'
  require_text .github/workflows/dependency-audit.yml 'run: bun audit' 'Dependency audit must run bun audit.'
  require_text .github/workflows/dependency-audit.yml 'run: govulncheck ./...' 'Dependency audit must run govulncheck.'
}

write_facts() {
  local generated
  generated=$(mktemp "${TMPDIR:-/tmp}/mega-agents-ci-facts.XXXXXX")
  trap 'rm -f -- "$generated"' RETURN
  facts > "$generated"
  if grep -Fq '<!-- ci-facts:start -->' "$doc"; then
    awk -v replacement="$generated" '
      /<!-- ci-facts:start -->/ {
        while ((getline line < replacement) > 0) print line
        skip=1
        next
      }
      /<!-- ci-facts:end -->/ { skip=0; next }
      !skip { print }
    ' "$doc" > "$doc.tmp"
    mv "$doc.tmp" "$doc"
  else
    printf '\n## Machine-checked CI facts\n\n' >> "$doc"
    cat "$generated" >> "$doc"
  fi
}

validate_facts() {
  local expected actual
  [[ $(grep -Fc '<!-- ci-facts:start -->' "$doc") -eq 1 && \
    $(grep -Fc '<!-- ci-facts:end -->' "$doc") -eq 1 ]] || {
    echo 'docs/ci-doc.md must contain exactly one machine-checked facts block.' >&2
    exit 1
  }
  expected=$(mktemp "${TMPDIR:-/tmp}/mega-agents-ci-expected.XXXXXX")
  actual=$(mktemp "${TMPDIR:-/tmp}/mega-agents-ci-actual.XXXXXX")
  trap 'rm -f -- "$expected" "$actual"' RETURN
  facts > "$expected"
  awk '/<!-- ci-facts:start -->/{copy=1} copy{print} /<!-- ci-facts:end -->/{exit}' "$doc" > "$actual"
  cmp -s "$expected" "$actual" || {
    echo 'docs/ci-doc.md machine-checked facts are stale; run scripts/gates/check-ci-documentation.sh --write-facts.' >&2
    exit 1
  }
}

if [[ $mode == --write-facts ]]; then
  write_facts
  exit 0
fi

validate_configuration
validate_facts

if [[ -n ${CI_CHANGED_FILES+x} ]]; then
  changed=$CI_CHANGED_FILES
elif [[ -n ${CI_BASE_SHA:-} ]]; then
  changed=$(git -C "$root" diff --name-only "$CI_BASE_SHA" "${CI_HEAD_SHA:-HEAD}")
else
  changed=$(git -C "$root" diff --cached --name-only)
fi

relevant=$(printf '%s\n' "$changed" | awk '
  /^Makefile$/ || /^\.github\/workflows\/[^/]+\.ya?ml$/ || /^scripts\/gates\/[^/]+$/ { print }
' | LC_ALL=C sort -u)
[[ -n $relevant ]] || exit 0

if printf '%s\n' "$changed" | grep -Fxq 'docs/ci-doc.md'; then
  exit 0
fi

if ! printf '%s\n' "$changed" | grep -Fxq '.ci-doc-no-impact'; then
  echo 'CI-defining files changed without docs/ci-doc.md or a changed .ci-doc-no-impact declaration.' >&2
  exit 1
fi

mapfile -t lines < "$declaration"
[[ ${lines[0]:-} == CI_DOCUMENTATION_NO_IMPACT_V1 ]] || {
  echo '.ci-doc-no-impact has an invalid version header.' >&2; exit 1;
}
[[ ${lines[1]:-} == reason=* && ${#lines[1]} -ge 28 ]] || {
  echo '.ci-doc-no-impact requires a specific reason of at least 21 characters.' >&2; exit 1;
}

expected=$(mktemp "${TMPDIR:-/tmp}/mega-agents-ci-declaration.XXXXXX")
actual=$(mktemp "${TMPDIR:-/tmp}/mega-agents-ci-declaration.XXXXXX")
trap 'rm -f -- "$expected" "$actual"' EXIT
while IFS= read -r path; do
  if [[ -f $root/$path ]]; then
    printf '%s  %s\n' "$(sha256sum "$root/$path" | awk '{print $1}')" "$path"
  else
    printf 'DELETED  %s\n' "$path"
  fi
done <<< "$relevant" > "$expected"
printf '%s\n' "${lines[@]:2}" > "$actual"
LC_ALL=C sort -o "$actual" "$actual"
cmp -s "$expected" "$actual" || {
  echo '.ci-doc-no-impact must list exactly the changed CI-defining files with their current SHA-256 hashes.' >&2
  exit 1
}
