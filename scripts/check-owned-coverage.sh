#!/usr/bin/env bash
# Fail unless statement coverage across owned-packages is >= MIN_COVER.
# See CONSTRAINTS.md. Do not weaken the threshold here — edit CONSTRAINTS.md
# in its own reviewed commit if the bar must change.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

# shellcheck source=scripts/owned-packages.sh
. "$ROOT/scripts/owned-packages.sh"

MIN_COVER="${MIN_COVER:-90}"
PROFILE="${COVERPROFILE:-$(mktemp -t ia-owned-cover.XXXXXX.out)}"
cleanup() {
  if [[ "${COVERPROFILE:-}" == "" && -f "$PROFILE" ]]; then
    rm -f "$PROFILE"
  fi
}
trap cleanup EXIT

mapfile -t PKGS < <(owned_packages "$ROOT")

echo "owned packages: ${PKGS[*]}"
go test "${PKGS[@]}" "-coverprofile=$PROFILE" "-covermode=set"
total="$(go tool cover -func="$PROFILE" | awk '/^total:/ { print $3 }' | tr -d '%')"
if [[ -z "$total" ]]; then
  echo "error: could not parse coverage total from $PROFILE" >&2
  exit 2
fi

awk -v got="$total" -v need="$MIN_COVER" 'BEGIN {
  if (got+0 < need+0) {
    printf "FAIL: owned coverage %.1f%% < %s%% (CONSTRAINTS.md)\n", got, need
    exit 1
  }
  printf "OK: owned coverage %.1f%% >= %s%%\n", got, need
  exit 0
}'
