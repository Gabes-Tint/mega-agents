#!/usr/bin/env bash
set -euo pipefail
root=$(git rev-parse --show-toplevel)
cd "$root"
if git diff --cached --quiet; then exit 0; fi
test -d frontend/node_modules || { echo 'Run make install before committing.' >&2; exit 1; }
# Verify precisely the index, including on the first commit.
snapshot=$(mktemp -d "${TMPDIR:-/tmp}/mega-agents-check.XXXXXX")
trap 'rm -r -- "$snapshot"' EXIT
git checkout-index --all --prefix="$snapshot/"
for manifest in package.json bun.lock; do
  cmp -s "frontend/$manifest" "$snapshot/frontend/$manifest" || {
    echo "Stage frontend/$manifest consistently and run make install." >&2; exit 1;
  }
done
ln -s "$root/frontend/node_modules" "$snapshot/frontend/node_modules"
# The snapshot has no .git directory. Drop hook-injected Git variables so Go's
# VCS stamping does not read a stale relative GIT_DIR inside the snapshot.
env -u GIT_DIR -u GIT_WORK_TREE -u GIT_INDEX_FILE -u GIT_COMMON_DIR \
  -u GIT_OBJECT_DIRECTORY -u GIT_ALTERNATE_OBJECT_DIRECTORIES \
  CI_CHANGED_FILES="$(git diff --cached --name-only)" make -C "$snapshot" verify
