#!/bin/sh
# Install initagent hub on a Linux systemd host.
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/pleware/initagent/main/scripts/install.sh | sh
#   curl -fsSL https://raw.githubusercontent.com/pleware/initagent/main/scripts/install.sh | sh -s -- uninstall
set -eu

ACTION="${1:-install}"
REPO="${INITAGENT_REPO:-pleware/initagent}"
VERSION="${INITAGENT_VERSION:-latest}"
REF="${INITAGENT_REF:-main}"
ADDR="${INITAGENT_ADDR:-:4200}"
DATA_DIR="${INITAGENT_DATA_DIR:-/var/lib/initagent}"
USER_NAME="${INITAGENT_USER:-initagent}"
BIN_DIR="${INITAGENT_BIN_DIR:-/usr/local/bin}"
MANAGED_BIN_DIR="$DATA_DIR/bin"
SERVICE_NAME="${INITAGENT_SERVICE_NAME:-initagent-hub}"
UNIT_DIR="${INITAGENT_SYSTEMD_DIR:-/etc/systemd/system}"
TLS_DOMAIN="${INITAGENT_TLS_DOMAIN:-}"
TLS_EMAIL="${INITAGENT_TLS_EMAIL:-}"
INSTALL_SOURCE="${INITAGENT_INSTALL_SOURCE:-auto}"
GO_VERSION="${INITAGENT_GO_VERSION:-1.25.0}"
GO_SHA256="${INITAGENT_GO_SHA256:-}"
NODE_MAJOR="${INITAGENT_NODE_MAJOR:-20}"
PURGE="${INITAGENT_PURGE:-0}"
LOCAL_BINARY="${INITAGENT_LOCAL_BINARY:-}"
SERVICE_SHELL="${INITAGENT_SERVICE_SHELL:-/bin/sh}"

log() { printf '%s\n' "initagent: $*"; }
die() { printf '%s\n' "initagent: $*" >&2; exit 1; }
need_cmd() { command -v "$1" >/dev/null 2>&1 || die "missing required command: $1"; }

[ "$(uname -s)" = Linux ] || die "this installer supports Linux hosts only"
command -v systemctl >/dev/null 2>&1 || die "this installer requires systemd"
case "$INSTALL_SOURCE" in auto|binary|source) ;; *) die "INITAGENT_INSTALL_SOURCE must be auto, binary, or source" ;; esac
case "$USER_NAME:$SERVICE_NAME" in *[!a-zA-Z0-9_.:-]*) die "service user/name contains unsupported characters" ;; esac

SUDO=""
if [ "$(id -u)" -ne 0 ]; then need_cmd sudo; SUDO=sudo; fi

safe_purge() {
  case "$DATA_DIR" in ""|/|/var|/usr|/etc|"$HOME") die "refusing to purge unsafe data directory: $DATA_DIR" ;; esac
  $SUDO rm -rf "$DATA_DIR"
}

uninstall_service() {
  UNIT_FILE="$UNIT_DIR/${SERVICE_NAME}.service"
  if systemctl list-unit-files "${SERVICE_NAME}.service" >/dev/null 2>&1 || [ -f "$UNIT_FILE" ]; then
    log "stopping service: $SERVICE_NAME"
    $SUDO systemctl disable --now "$SERVICE_NAME" >/dev/null 2>&1 || true
  fi
  $SUDO rm -f "$UNIT_FILE"
  $SUDO systemctl daemon-reload
  $SUDO systemctl reset-failed "$SERVICE_NAME" >/dev/null 2>&1 || true
  $SUDO rm -f "$BIN_DIR/initagent" "$MANAGED_BIN_DIR/initagent" "$MANAGED_BIN_DIR/initagent.previous"
  if [ "$PURGE" = 1 ]; then
    log "purging data directory: $DATA_DIR"
    safe_purge
    if id "$USER_NAME" >/dev/null 2>&1; then $SUDO userdel "$USER_NAME" >/dev/null 2>&1 || true; fi
  else
    log "kept data directory: $DATA_DIR"
    log "rerun with INITAGENT_PURGE=1 to remove data and service user"
  fi
  log "uninstalled"
}

case "$ACTION" in install) ;; uninstall|remove) uninstall_service; exit 0 ;; *) die "unknown action: $ACTION (use install or uninstall)" ;; esac

ARCH="$(uname -m)"
case "$ARCH" in x86_64|amd64) ARCH=amd64 ;; aarch64|arm64) ARCH=arm64 ;; *) die "unsupported architecture: $ARCH" ;; esac
ASSET="initagent_linux_$ARCH"
TMP_DIR="$(mktemp -d)"
cleanup() { rm -rf "$TMP_DIR"; }
trap cleanup EXIT INT TERM

install_packages() {
  missing=""
  for cmd in "$@"; do command -v "$cmd" >/dev/null 2>&1 || missing="$missing $cmd"; done
  [ -n "$missing" ] || return 0
  if command -v apt-get >/dev/null 2>&1; then
    log "installing packages:$missing"
    $SUDO env DEBIAN_FRONTEND=noninteractive apt-get update
    for package in $missing; do
      [ "$package" != curl ] || $SUDO env DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates curl
      [ "$package" != tar ] || $SUDO env DEBIAN_FRONTEND=noninteractive apt-get install -y tar
      [ "$package" != tmux ] || $SUDO env DEBIAN_FRONTEND=noninteractive apt-get install -y tmux
    done
    return
  fi
  if command -v dnf >/dev/null 2>&1; then $SUDO dnf install -y $missing; return; fi
  if command -v yum >/dev/null 2>&1; then $SUDO yum install -y $missing; return; fi
  die "cannot install missing commands:$missing; install them and rerun"
}

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'; else shasum -a 256 "$1" | awk '{print $1}'; fi
}

version_at_least() {
  awk -v have="$1" -v need="$2" '
    BEGIN {
      split(have, h, "."); split(need, n, ".")
      for (i = 1; i <= 3; i++) {
        if ((h[i] + 0) > (n[i] + 0)) exit 0
        if ((h[i] + 0) < (n[i] + 0)) exit 1
      }
      exit 0
    }'
}

ensure_go() {
  if command -v go >/dev/null 2>&1; then
    HAVE="$(go env GOVERSION 2>/dev/null | sed 's/^go//; s/[^0-9.].*$//')"
    if [ -n "$HAVE" ] && version_at_least "$HAVE" "$GO_VERSION"; then return; fi
  fi
  log "using temporary Go $GO_VERSION toolchain"
  GO_ARCHIVE="go${GO_VERSION}.linux-${ARCH}.tar.gz"
  curl -fL "https://go.dev/dl/$GO_ARCHIVE" -o "$TMP_DIR/$GO_ARCHIVE"
  if [ -z "$GO_SHA256" ]; then
    case "$GO_VERSION:$ARCH" in
      1.25.0:amd64) GO_SHA256=2852af0cb20a13139b3448992e69b868e50ed0f8a1e5940ee1de9e19a123b613 ;;
      1.25.0:arm64) GO_SHA256=05de75d6994a2783699815ee553bd5a9327d8b79991de36e38b66862782f54ae ;;
      *) die "set INITAGENT_GO_SHA256 when overriding INITAGENT_GO_VERSION" ;;
    esac
  fi
  ACTUAL="$(sha256_file "$TMP_DIR/$GO_ARCHIVE")"
  [ "$ACTUAL" = "$GO_SHA256" ] || die "Go checksum verification failed"
  mkdir -p "$TMP_DIR/toolchains"
  tar -xzf "$TMP_DIR/$GO_ARCHIVE" -C "$TMP_DIR/toolchains"
  PATH="$TMP_DIR/toolchains/go/bin:$PATH"
  export PATH
}

ensure_node() {
  if command -v node >/dev/null 2>&1 && command -v npm >/dev/null 2>&1; then
    HAVE_NODE="$(node -v | sed 's/^v//; s/\..*$//')"
    if [ "${HAVE_NODE:-0}" -ge "$NODE_MAJOR" ]; then return; fi
  fi
  log "using temporary Node $NODE_MAJOR toolchain"
  NODE_BASE="https://nodejs.org/dist/latest-v${NODE_MAJOR}.x"
  curl -fL "$NODE_BASE/SHASUMS256.txt" -o "$TMP_DIR/node-shasums.txt"
  NODE_ARCHIVE="$(awk -v arch="$ARCH" '$2 ~ ("^node-v[0-9.]+-linux-" arch "\\.tar\\.gz$") { print $2; exit }' "$TMP_DIR/node-shasums.txt")"
  [ -n "$NODE_ARCHIVE" ] || die "could not resolve Node $NODE_MAJOR for linux/$ARCH"
  EXPECTED="$(awk -v file="$NODE_ARCHIVE" '$2 == file { print $1; exit }' "$TMP_DIR/node-shasums.txt")"
  curl -fL "$NODE_BASE/$NODE_ARCHIVE" -o "$TMP_DIR/$NODE_ARCHIVE"
  ACTUAL="$(sha256_file "$TMP_DIR/$NODE_ARCHIVE")"
  [ "$ACTUAL" = "$EXPECTED" ] || die "Node checksum verification failed"
  mkdir -p "$TMP_DIR/toolchains/node"
  tar -xzf "$TMP_DIR/$NODE_ARCHIVE" -C "$TMP_DIR/toolchains/node" --strip-components=1
  PATH="$TMP_DIR/toolchains/node/bin:$PATH"
  export PATH
}

download_release_binary() {
  if [ "$VERSION" = latest ]; then URL="https://github.com/$REPO/releases/latest/download/$ASSET"; else URL="https://github.com/$REPO/releases/download/$VERSION/$ASSET"; fi
  log "trying release binary: $URL"
  if ! curl -fL "$URL" -o "$TMP_DIR/initagent"; then return 1; fi
  curl -fL "$(dirname "$URL")/checksums.txt" -o "$TMP_DIR/checksums.txt" || return 1
  EXPECTED="$(awk -v asset="$ASSET" '$2 == asset || $2 == "*" asset { print $1; exit }' "$TMP_DIR/checksums.txt")"
  [ -n "$EXPECTED" ] || die "checksums.txt does not contain $ASSET"
  ACTUAL="$(sha256_file "$TMP_DIR/initagent")"
  [ "$ACTUAL" = "$EXPECTED" ] || die "release checksum verification failed"
  chmod +x "$TMP_DIR/initagent"
}

build_from_source() {
  install_packages curl tar
  ensure_go
  ensure_node
  SRC="$TMP_DIR/src"
  SOURCE_REF="$REF"
  if [ "$VERSION" != latest ]; then SOURCE_REF="$VERSION"; fi
  log "building from source: $REPO@$SOURCE_REF"
  mkdir -p "$SRC"
  curl -fL "https://github.com/$REPO/archive/$SOURCE_REF.tar.gz" -o "$TMP_DIR/source.tar.gz"
  tar -xzf "$TMP_DIR/source.tar.gz" -C "$SRC" --strip-components=1
  (cd "$SRC/ui" && npm install --no-audit --no-fund && npm run build)
  rm -rf "$SRC/cmd/overseer/uidist"
  mkdir -p "$SRC/cmd/overseer/uidist"
  cp -R "$SRC/ui/dist/." "$SRC/cmd/overseer/uidist/"
  (cd "$SRC" && go build -ldflags "-s -w -X main.version=$SOURCE_REF" -o "$TMP_DIR/initagent" ./cmd/overseer)
  chmod +x "$TMP_DIR/initagent"
}

install_binary() {
  if [ -n "$LOCAL_BINARY" ]; then
    [ -f "$LOCAL_BINARY" ] || die "local binary not found: $LOCAL_BINARY"
    log "using local binary: $LOCAL_BINARY"
    cp "$LOCAL_BINARY" "$TMP_DIR/initagent"
    chmod +x "$TMP_DIR/initagent"
    return
  fi
  install_packages curl
  if [ "$INSTALL_SOURCE" != source ] && download_release_binary; then return; fi
  [ "$INSTALL_SOURCE" != binary ] || die "release binary was not available"
  log "release binary unavailable; falling back to source build"
  build_from_source
}

unit_escape() { printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g' -e 's/%/%%/g'; }

install_service() {
  install_packages tmux
  if ! id "$USER_NAME" >/dev/null 2>&1; then
    [ -x "$SERVICE_SHELL" ] || die "service shell not found: $SERVICE_SHELL"
    log "creating service user: $USER_NAME"
    $SUDO useradd --system --home "$DATA_DIR" --shell "$SERVICE_SHELL" "$USER_NAME"
  else
    current_shell="$(getent passwd "$USER_NAME" | awk -F: '{print $7}')"
    case "$current_shell" in */nologin|*/false) $SUDO usermod --shell "$SERVICE_SHELL" "$USER_NAME" ;; esac
  fi
  $SUDO install -d -m 700 -o "$USER_NAME" -g "$USER_NAME" "$DATA_DIR"
  $SUDO install -d -m 755 -o "$USER_NAME" -g "$USER_NAME" "$MANAGED_BIN_DIR"
  $SUDO install -d -m 755 "$BIN_DIR"
  $SUDO install -d -m 755 "$UNIT_DIR"
  $SUDO systemctl stop "$SERVICE_NAME" >/dev/null 2>&1 || true
  if [ -f "$MANAGED_BIN_DIR/initagent" ]; then $SUDO cp "$MANAGED_BIN_DIR/initagent" "$MANAGED_BIN_DIR/initagent.previous"; fi
  $SUDO install -m 755 -o "$USER_NAME" -g "$USER_NAME" "$TMP_DIR/initagent" "$MANAGED_BIN_DIR/initagent"
  $SUDO ln -sfn "$MANAGED_BIN_DIR/initagent" "$BIN_DIR/initagent"

  E_BIN="$(unit_escape "$MANAGED_BIN_DIR/initagent")"
  E_DATA="$(unit_escape "$DATA_DIR")"
  E_ADDR="$(unit_escape "$ADDR")"
  UNIT="$TMP_DIR/${SERVICE_NAME}.service"
  {
    cat <<EOF
[Unit]
Description=initagent hub
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$USER_NAME
WorkingDirectory=$E_DATA
EOF
    if [ -n "$TLS_DOMAIN" ]; then
      [ -n "$TLS_EMAIL" ] || die "INITAGENT_TLS_EMAIL is required when INITAGENT_TLS_DOMAIN is set"
      printf '%s\n' 'AmbientCapabilities=CAP_NET_BIND_SERVICE' 'CapabilityBoundingSet=CAP_NET_BIND_SERVICE'
      printf 'ExecStart="%s" serve --data-dir "%s" --tls-domain "%s" --tls-email "%s"\n' "$E_BIN" "$E_DATA" "$(unit_escape "$TLS_DOMAIN")" "$(unit_escape "$TLS_EMAIL")"
    else
      printf 'ExecStart="%s" serve --addr "%s" --data-dir "%s"\n' "$E_BIN" "$E_ADDR" "$E_DATA"
    fi
    cat <<EOF
Environment=INITAGENT_MANAGED=hub
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF
  } > "$UNIT"
  $SUDO install -m 644 "$UNIT" "$UNIT_DIR/${SERVICE_NAME}.service"
  $SUDO systemctl daemon-reload
  $SUDO systemctl enable "$SERVICE_NAME"
  $SUDO systemctl restart "$SERVICE_NAME"
}

install_binary
install_service
INSTALLED_VERSION="$("$MANAGED_BIN_DIR/initagent" version 2>/dev/null | sed 's/^initagent //' || true)"
[ -n "$INSTALLED_VERSION" ] && log "installed $INSTALLED_VERSION" || log "installed initagent"
log "service: $SERVICE_NAME"
log "status: systemctl status $SERVICE_NAME"
if [ -n "$TLS_DOMAIN" ]; then log "open: https://$TLS_DOMAIN"; else case "$ADDR" in :*) log "open: http://<this-host>$ADDR" ;; *) log "open: http://$ADDR" ;; esac; fi
