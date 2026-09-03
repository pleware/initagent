package gateway

// unixInstallScript is the one-paste joiner. HUB is this gateway's URL.
const unixInstallScript = `#!/bin/sh
# initagent device installer — enrolls into the project gateway
set -eu

HUB="%s"
TOKEN="%s"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "initagent: unsupported architecture: $ARCH" >&2; exit 1 ;;
esac
case "$OS" in
  linux|darwin) ;;
  *) echo "initagent: unsupported OS: $OS (Linux and macOS only for now)" >&2; exit 1 ;;
esac

BIN_DIR="$HOME/.initagent/bin"
mkdir -p "$BIN_DIR"
BIN="$BIN_DIR/initagent"

echo "→ downloading agent for $OS/$ARCH from the gateway..."
if ! curl -fSL "$HUB/api/agent-binary?os=$OS&arch=$ARCH" -o "$BIN.tmp"; then
  echo "initagent: gateway has no binary for $OS/$ARCH." >&2
  exit 1
fi
mv "$BIN.tmp" "$BIN"
chmod +x "$BIN"

echo "→ enrolling this device with the gateway..."
"$BIN" agent enroll --hub "$HUB" --token "$TOKEN"

echo "→ installing background service..."
"$BIN" agent install-service

echo ""
echo "✓ Done. This device is connected to the project gateway."
`

const windowsInstallScript = `$ErrorActionPreference = "Stop"

$Hub = "%s"
$Token = "%s"

$RawArch = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
switch -Regex ($RawArch.ToLowerInvariant()) {
  "amd64|x64" { $Arch = "amd64"; break }
  "arm64" { $Arch = "arm64"; break }
  default { throw "initagent: unsupported architecture: $RawArch" }
}

$BinDir = Join-Path $env:LOCALAPPDATA "Initagent\bin"
New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
$Bin = Join-Path $BinDir "initagent.exe"
$Tmp = "$Bin.tmp"

Write-Host "-> downloading agent for windows/$Arch from the gateway..."
Invoke-WebRequest -UseBasicParsing -Uri "$Hub/api/agent-binary?os=windows&arch=$Arch" -OutFile $Tmp
Move-Item -Force $Tmp $Bin

Write-Host "-> enrolling this device with the gateway..."
& $Bin agent enroll --hub $Hub --token $Token

Write-Host "-> installing background task..."
& $Bin agent install-service

Write-Host ""
Write-Host "Done. This device is connected to the project gateway."
`
