param(
  [ValidateSet("install", "uninstall", "remove")]
  [string]$Action = "install"
)
$ErrorActionPreference = "Stop"
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$Repo = if ($env:INITAGENT_REPO) { $env:INITAGENT_REPO } else { "pleware/initagent" }
$Version = if ($env:INITAGENT_VERSION) { $env:INITAGENT_VERSION } else { "latest" }
$Ref = if ($env:INITAGENT_REF) { $env:INITAGENT_REF } else { "main" }
$Addr = if ($env:INITAGENT_ADDR) { $env:INITAGENT_ADDR } else { ":4200" }
$InstallSource = if ($env:INITAGENT_INSTALL_SOURCE) { $env:INITAGENT_INSTALL_SOURCE } else { "auto" }
$GoVersion = if ($env:INITAGENT_GO_VERSION) { $env:INITAGENT_GO_VERSION } else { "1.25.0" }
$NodeMajor = if ($env:INITAGENT_NODE_MAJOR) { [int]$env:INITAGENT_NODE_MAJOR } else { 20 }
$Base = if ($env:LOCALAPPDATA) { Join-Path $env:LOCALAPPDATA "Initagent" } else { Join-Path $HOME "AppData\Local\Initagent" }
$BinDir = if ($env:INITAGENT_BIN_DIR) { $env:INITAGENT_BIN_DIR } else { Join-Path $Base "bin" }
$DataDir = if ($env:INITAGENT_DATA_DIR) { $env:INITAGENT_DATA_DIR } else { Join-Path $Base "data" }
$Bin = Join-Path $BinDir "initagent.exe"
$PreviousBin = Join-Path $BinDir "initagent.previous.exe"
$Task = if ($env:INITAGENT_SERVICE_NAME) { $env:INITAGENT_SERVICE_NAME } else { "InitagentHub" }
$Runner = Join-Path $BinDir "initagent-hub.ps1"

if ($InstallSource -notin @("auto", "binary", "source")) {
  throw "INITAGENT_INSTALL_SOURCE must be auto, binary, or source"
}

function Write-InitagentLog([string]$Message) {
  Write-Host "initagent: $Message"
}

function Invoke-Download([string]$Uri, [string]$OutFile) {
  Invoke-WebRequest -UseBasicParsing -Uri $Uri -OutFile $OutFile
}

function Get-ExpectedChecksum([string]$Content, [string]$Asset) {
  $escaped = [regex]::Escape($Asset)
  $line = ($Content -split "`n" | Where-Object { $_ -match "^[0-9a-fA-F]{64}\s+\*?$escaped\s*$" } | Select-Object -First 1)
  if (-not $line) { throw "checksums.txt does not contain $Asset" }
  return ($line -split "\s+")[0].ToLowerInvariant()
}

function Assert-Checksum([string]$Path, [string]$Expected, [string]$Label) {
  $actual = (Get-FileHash -Algorithm SHA256 $Path).Hash.ToLowerInvariant()
  if ($actual -ne $Expected.ToLowerInvariant()) { throw "$Label checksum verification failed" }
}

function Stop-HubTask {
  & schtasks.exe /End /TN $Task 2>$null | Out-Null
}

if ($Action -in @("uninstall", "remove")) {
  Stop-HubTask
  & schtasks.exe /Delete /TN $Task /F 2>$null | Out-Null
  Remove-Item -Force -ErrorAction SilentlyContinue $Bin, $PreviousBin, $Runner
  if ($env:INITAGENT_PURGE -eq "1") {
    $fullData = [IO.Path]::GetFullPath($DataDir)
    $unsafe = @([IO.Path]::GetPathRoot($fullData), [IO.Path]::GetFullPath($HOME), [IO.Path]::GetFullPath($Base))
    if ($unsafe -contains $fullData) { throw "refusing to purge unsafe data directory: $fullData" }
    Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $fullData
    Write-InitagentLog "purged data at $fullData"
  } else {
    Write-InitagentLog "kept data at $DataDir"
  }
  Write-InitagentLog "uninstalled"
  exit 0
}

$RawArch = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
$Arch = switch -Regex ($RawArch.ToLowerInvariant()) {
  "amd64|x64" { "amd64"; break }
  "arm64" { "arm64"; break }
  default { throw "Unsupported architecture: $RawArch" }
}
$Asset = "initagent_windows_$Arch"
$TempRoot = Join-Path ([IO.Path]::GetTempPath()) ("initagent-install-" + [Guid]::NewGuid().ToString("N"))
$StagedBin = Join-Path $TempRoot "initagent.exe"
New-Item -ItemType Directory -Force -Path $TempRoot, $BinDir, $DataDir | Out-Null

function Get-ReleaseBinary {
  $url = if ($Version -eq "latest") {
    "https://github.com/$Repo/releases/latest/download/$Asset"
  } else {
    "https://github.com/$Repo/releases/download/$Version/$Asset"
  }
  Write-InitagentLog "trying release binary: $url"
  try {
    Invoke-Download $url $StagedBin
    $releaseBase = $url.Substring(0, $url.LastIndexOf('/'))
    $checksums = (Invoke-WebRequest -UseBasicParsing -Uri "$releaseBase/checksums.txt").Content
    $expected = Get-ExpectedChecksum $checksums $Asset
    Assert-Checksum $StagedBin $expected "release"
    return $true
  } catch {
    Remove-Item -Force -ErrorAction SilentlyContinue $StagedBin
    Write-InitagentLog "release binary unavailable: $($_.Exception.Message)"
    return $false
  }
}

function Ensure-GoToolchain {
  $current = $null
  if (Get-Command go -ErrorAction SilentlyContinue) {
    if ((& go env GOVERSION 2>$null) -match '^go([0-9.]+)') { $current = [version]$Matches[1] }
  }
  if ($current -and $current -ge [version]$GoVersion) { return }

  Write-InitagentLog "using temporary Go $GoVersion toolchain"
  $manifest = Invoke-RestMethod -UseBasicParsing -Uri "https://go.dev/dl/?mode=json&include=all"
  $release = $manifest | Where-Object { $_.version -eq "go$GoVersion" } | Select-Object -First 1
  $file = $release.files | Where-Object { $_.os -eq "windows" -and $_.arch -eq $Arch -and $_.kind -eq "archive" } | Select-Object -First 1
  if (-not $file) { throw "could not resolve Go $GoVersion for windows/$Arch" }
  $archive = Join-Path $TempRoot $file.filename
  Invoke-Download "https://go.dev/dl/$($file.filename)" $archive
  Assert-Checksum $archive $file.sha256 "Go"
  $destination = Join-Path $TempRoot "toolchains"
  Expand-Archive -Force -Path $archive -DestinationPath $destination
  $env:PATH = (Join-Path $destination "go\bin") + ";" + $env:PATH
}

function Ensure-NodeToolchain {
  $current = 0
  if ((Get-Command node -ErrorAction SilentlyContinue) -and (Get-Command npm -ErrorAction SilentlyContinue)) {
    $current = [int]((& node --version).TrimStart('v').Split('.')[0])
  }
  if ($current -ge $NodeMajor) { return }

  Write-InitagentLog "using temporary Node $NodeMajor toolchain"
  $baseUrl = "https://nodejs.org/dist/latest-v$NodeMajor.x"
  $sums = (Invoke-WebRequest -UseBasicParsing -Uri "$baseUrl/SHASUMS256.txt").Content
  $pattern = "^[0-9a-fA-F]{64}\s+(node-v[0-9.]+-win-$Arch\.zip)\s*$"
  $archiveName = $null
  foreach ($line in ($sums -split "`n")) {
    if ($line -match $pattern) { $archiveName = $Matches[1]; break }
  }
  if (-not $archiveName) { throw "could not resolve Node $NodeMajor for windows/$Arch" }
  $archive = Join-Path $TempRoot $archiveName
  Invoke-Download "$baseUrl/$archiveName" $archive
  Assert-Checksum $archive (Get-ExpectedChecksum $sums $archiveName) "Node"
  $destination = Join-Path $TempRoot "toolchains\node"
  Expand-Archive -Force -Path $archive -DestinationPath $destination
  $nodeRoot = Get-ChildItem -Path $destination -Directory | Select-Object -First 1
  $env:PATH = $nodeRoot.FullName + ";" + $env:PATH
}

function Build-FromSource {
  Ensure-GoToolchain
  Ensure-NodeToolchain
  $sourceRef = if ($Version -ne "latest") { $Version } else { $Ref }
  Write-InitagentLog "building from source: $Repo@$sourceRef"
  $archive = Join-Path $TempRoot "source.zip"
  $extractRoot = Join-Path $TempRoot "source-archive"
  Invoke-Download "https://github.com/$Repo/archive/$sourceRef.zip" $archive
  Expand-Archive -Force -Path $archive -DestinationPath $extractRoot
  $source = Get-ChildItem -Path $extractRoot -Directory | Select-Object -First 1
  if (-not $source) { throw "source archive was empty" }

  Push-Location (Join-Path $source.FullName "ui")
  try {
    & npm install --no-audit --no-fund
    if ($LASTEXITCODE -ne 0) { throw "npm install failed" }
    & npm run build
    if ($LASTEXITCODE -ne 0) { throw "UI build failed" }
  } finally { Pop-Location }

  $uiDist = Join-Path $source.FullName "ui\dist"
  $embedded = Join-Path $source.FullName "cmd\overseer\uidist"
  Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $embedded
  New-Item -ItemType Directory -Force -Path $embedded | Out-Null
  Copy-Item -Recurse -Force (Join-Path $uiDist "*") $embedded

  Push-Location $source.FullName
  try {
    & go build -ldflags "-s -w -X main.version=$sourceRef" -o $StagedBin ./cmd/overseer
    if ($LASTEXITCODE -ne 0) { throw "Go build failed" }
  } finally { Pop-Location }
}

try {
  if ($env:INITAGENT_LOCAL_BINARY) {
    if (-not (Test-Path -LiteralPath $env:INITAGENT_LOCAL_BINARY -PathType Leaf)) { throw "local binary not found: $env:INITAGENT_LOCAL_BINARY" }
    Write-InitagentLog "using local binary: $env:INITAGENT_LOCAL_BINARY"
    Copy-Item -Force $env:INITAGENT_LOCAL_BINARY $StagedBin
  } else {
    $downloaded = $false
    if ($InstallSource -ne "source") { $downloaded = Get-ReleaseBinary }
    if (-not $downloaded) {
      if ($InstallSource -eq "binary") { throw "release binary was not available" }
      Write-InitagentLog "release binary unavailable; falling back to source build"
      Build-FromSource
    }
  }

  Stop-HubTask
  if (Test-Path -LiteralPath $Bin) { Copy-Item -Force $Bin $PreviousBin }
  Move-Item -Force $StagedBin $Bin

  function Quote-PowerShellLiteral([string]$Value) { return $Value.Replace("'", "''") }
  $runnerLines = @(
    '$ErrorActionPreference = ''Stop''',
    '$env:INITAGENT_MANAGED = ''hub''',
    ("`$env:INITAGENT_WINDOWS_TASK = '" + (Quote-PowerShellLiteral $Task) + "'"),
    ("& '" + (Quote-PowerShellLiteral $Bin) + "' serve --addr '" + (Quote-PowerShellLiteral $Addr) + "' --data-dir '" + (Quote-PowerShellLiteral $DataDir) + "'")
  )
  Set-Content -Path $Runner -Value $runnerLines -Encoding UTF8
  $taskCommand = "powershell.exe -NoProfile -ExecutionPolicy Bypass -File `"$Runner`""
  & schtasks.exe /Create /TN $Task /SC ONLOGON /TR $taskCommand /F | Out-Null
  if ($LASTEXITCODE -ne 0) { throw "could not create Scheduled Task $Task" }
  & schtasks.exe /Run /TN $Task | Out-Null
  if ($LASTEXITCODE -ne 0) { throw "could not start Scheduled Task $Task" }

  $installedVersion = (& $Bin version 2>$null) -replace '^initagent\s+', ''
  if ($installedVersion) { Write-InitagentLog "installed $installedVersion" } else { Write-InitagentLog "installed initagent" }
  $openAddr = if ($Addr.StartsWith(':')) { "http://localhost$Addr" } else { "http://$Addr" }
  Write-InitagentLog "service: $Task"
  Write-InitagentLog "open: $openAddr"
} finally {
  Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $TempRoot
}
