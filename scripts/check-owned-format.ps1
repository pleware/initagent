# Fail unless gofmt is clean across owned-packages. See CONSTRAINTS.md.
# Windows companion to check-owned-format.sh.

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
Set-Location $Root

. (Join-Path $PSScriptRoot "OwnedPackages.ps1")

$pkgs = Get-OwnedPackages -Root $Root

Write-Host "owned packages: $($pkgs -join ' ')"
$unformatted = & gofmt -l @pkgs
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

if ($unformatted) {
    Write-Host "FAIL: gofmt not clean (CONSTRAINTS.md):"
    $unformatted | ForEach-Object { Write-Host $_ }
    Write-Host "run: gofmt -w $($pkgs -join ' ')"
    exit 1
}

Write-Host "OK: gofmt clean across owned packages"
exit 0
