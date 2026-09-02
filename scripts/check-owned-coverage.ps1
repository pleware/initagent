# Fail unless statement coverage across owned-packages is >= MIN_COVER.
# Windows companion to check-owned-coverage.sh. Same threshold — CONSTRAINTS.md.
param(
    [double]$MinCover = 90
)

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
Set-Location $Root

$List = Join-Path $Root "owned-packages"
if (-not (Test-Path $List)) {
    Write-Error "missing $List"
}

$pkgs = Get-Content $List |
    Where-Object { $_ -notmatch '^\s*#' -and $_ -notmatch '^\s*$' }
if (-not $pkgs) {
    Write-Error "owned-packages is empty"
}

$profile = Join-Path $env:TEMP ("ia-owned-cover-{0}.out" -f [guid]::NewGuid().ToString("n"))
try {
    Write-Host "owned packages: $($pkgs -join ' ')"
    & go test @pkgs "-coverprofile=$profile" "-covermode=set"
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

    $totalLine = & go tool cover "-func=$profile" | Select-String '^total:'
    if (-not $totalLine) {
        Write-Error "could not parse coverage total from $profile"
    }
    if ($totalLine.Line -notmatch '([\d.]+)%') {
        Write-Error "unexpected total line: $($totalLine.Line)"
    }
    $got = [double]$Matches[1]
    if ($got -lt $MinCover) {
        Write-Host ("FAIL: owned coverage {0:N1}% < {1}% (CONSTRAINTS.md)" -f $got, $MinCover)
        exit 1
    }
    Write-Host ("OK: owned coverage {0:N1}% >= {1}%" -f $got, $MinCover)
    exit 0
}
finally {
    if (Test-Path $profile) { Remove-Item $profile -Force }
}
