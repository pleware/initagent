# Read the owned-packages list. Dot-sourced by every gate scoped to owned
# code, so the list has one parser rather than one per check.
# Windows companion to owned-packages.sh.

function Get-OwnedPackages {
    param(
        [Parameter(Mandatory = $true)][string]$Root
    )

    $list = Join-Path $Root "owned-packages"
    if (-not (Test-Path $list)) {
        throw "missing $list"
    }

    $pkgs = Get-Content $list |
        Where-Object { $_ -notmatch '^\s*#' -and $_ -notmatch '^\s*$' }
    if (-not $pkgs) {
        throw "owned-packages is empty"
    }

    return $pkgs
}
