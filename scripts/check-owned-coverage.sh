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

# mktemp answers a POSIX path. Under git-bash/MSYS the Go toolchain is a native
# Win32 binary, and /tmp/… is not a location it can open — so the profile go
# writes and reads back has to carry the Win32 spelling. cygpath exists on
# MSYS/Cygwin only; on Linux and macOS the POSIX path is already the right one.
# Cleanup keeps the POSIX spelling: it is the path mktemp vouched for, and msys
# rm reads both.
PROFILE_OWNED=""
if [[ -n "${COVERPROFILE:-}" ]]; then
  PROFILE="$COVERPROFILE"
else
  PROFILE_OWNED="$(mktemp -t ia-owned-cover.XXXXXX.out)"
  PROFILE="$PROFILE_OWNED"
  if command -v cygpath >/dev/null 2>&1; then
    PROFILE="$(cygpath -w "$PROFILE_OWNED")"
  fi
fi
cleanup() {
  if [[ -n "$PROFILE_OWNED" && -f "$PROFILE_OWNED" ]]; then
    rm -f "$PROFILE_OWNED"
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
