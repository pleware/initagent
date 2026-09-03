#!/usr/bin/env bash
# Read the owned-packages list. Sourced by every gate scoped to owned code,
# so the list has one parser rather than one per check.

owned_packages() {
  local root="$1"
  local list="$root/owned-packages"

  if [[ ! -f "$list" ]]; then
    echo "error: missing $list" >&2
    return 2
  fi

  local pkgs
  mapfile -t pkgs < <(grep -v '^[[:space:]]*#' "$list" | grep -v '^[[:space:]]*$' || true)
  if [[ ${#pkgs[@]} -eq 0 ]]; then
    echo "error: owned-packages is empty" >&2
    return 2
  fi

  printf '%s\n' "${pkgs[@]}"
}
