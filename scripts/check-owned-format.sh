#!/usr/bin/env bash
# Fail unless gofmt is clean across owned-packages. See CONSTRAINTS.md.
# Scoped to owned code because upstream Overseer files do not pass gofmt on
# this checkout, so `gofmt -l .` cannot be the gate.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

# shellcheck source=scripts/owned-packages.sh
. "$ROOT/scripts/owned-packages.sh"

mapfile -t PKGS < <(owned_packages "$ROOT")

echo "owned packages: ${PKGS[*]}"
unformatted="$(gofmt -l "${PKGS[@]}")"
if [[ -n "$unformatted" ]]; then
  echo "FAIL: gofmt not clean (CONSTRAINTS.md):" >&2
  echo "$unformatted" >&2
  echo "run: gofmt -w ${PKGS[*]}" >&2
  exit 1
fi

echo "OK: gofmt clean across owned packages"
