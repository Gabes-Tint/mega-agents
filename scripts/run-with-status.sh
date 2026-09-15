#!/usr/bin/env bash
set -uo pipefail

label=$1
shift

if "$@"; then
  printf '✅ %s\n' "$label"
else
  status=$?
  printf '❌ %s\n' "$label" >&2
  exit "$status"
fi
