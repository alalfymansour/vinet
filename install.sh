#!/usr/bin/env bash
# ViNet prebuilt binary installer.
# curl -fsSL https://github.com/alalfymansour/vinet/raw/refs/heads/main/install.sh | bash
set -euo pipefail

REPO="alalfymansour/vinet"
RELEASE_BASE="https://github.com/${REPO}/releases/latest/download"
SCRIPT_URL="https://github.com/${REPO}/raw/refs/heads/main/install.sh"
ICON_URL="https://github.com/${REPO}/raw/refs/heads/main/vinet.svg"
INSTALL_DIR="/usr/local/bin"
LOG_DIR="/var/log/vinet"
DATA_DIR="/var/lib/vinet"
DB_GROUP="vinet"
ICON_DIR="/usr/share/icons/hicolor/scalable/apps"
DESKTOP_DIR="/usr/share/applications"

GREEN='\033[0;32m'; RED='\033[0;31m'; YELLOW='\033[0;33m'; NC='\033[0m'
info() { echo -e "${GREEN}==> $1${NC}"; }
warn() { echo -e "${YELLOW}==> $1${NC}"; }
die()  { echo -e "${RED}==> $1${NC}" >&2; exit 1; }
fetch() {
    if command -v curl >/dev/null 2>&1; then curl -fsSL "$1" -o "$2"
    elif command -v wget >/dev/null 2>&1; then wget -qO "$2" "$1"
    else die "curl or wget is required."; fi
}

for arg in "$@"; do
    case "$arg" in
        -h|--help)
            cat <<'EOF'
ViNet installer

Usage:
  curl -fsSL https://github.com/alalfymansour/vinet/raw/refs/heads/main/install.sh | bash
  sudo ./install.sh

Options:
  -h, --help      Show this help
EOF
            exit 0 ;;
        *) die "Unknown option: $arg (use --help for usage)" ;;
    esac
done

if [ "${EUID:-$(id -u)}" -ne 0 ]; then
    warn "Root privileges required. Re-running with sudo..."
    if [ -f "$0" ]; then exec sudo -E bash "$0" "$@"; fi
    TMP_SCRIPT="$(mktemp /tmp/vinet-install.XXXXXX.sh)"
    trap 'rm -f "$TMP_SCRIPT"' EXIT
    fetch "$SCRIPT_URL" "$TMP_SCRIPT"
    exec sudo -E bash "$TMP_SCRIPT" "$@"
fi

case "$(uname -m)" in
    x86_64|amd64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) die "Unsupported architecture: $(uname -m). Available binaries: amd64 and arm64." ;;
esac

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT
BINARY="$TMP_DIR/vinet"
info "Downloading ViNet Linux ${ARCH} release..."
fetch "${RELEASE_BASE}/vinet-linux-${ARCH}" "$BINARY"
chmod 0755 "$BINARY"

mkdir -p "$INSTALL_DIR" "$LOG_DIR" "$DATA_DIR" "$ICON_DIR" "$DESKTOP_DIR"
if ! getent group "$DB_GROUP" >/dev/null 2>&1; then groupadd --system "$DB_GROUP"; fi
if [ -n "${SUDO_USER:-}" ] && id "$SUDO_USER" >/dev/null 2>&1; then usermod -a -G "$DB_GROUP" "$SUDO_USER"; fi
chown root:"$DB_GROUP" "$DATA_DIR"; chmod 2770 "$DATA_DIR"; chmod 0750 "$LOG_DIR"
if [ -f "$DATA_DIR/data.db" ]; then chown root:"$DB_GROUP" "$DATA_DIR/data.db"; chmod 0660 "$DATA_DIR/data.db"; fi
install -m 0755 "$BINARY" "$INSTALL_DIR/vinet"
fetch "$ICON_URL" "$ICON_DIR/vinet.svg"

cat > "$DESKTOP_DIR/vinet.desktop" <<'EOF'
[Desktop Entry]
Type=Application
Name=ViNet
Comment=Monitor per-process network traffic
Exec=vinet
Icon=vinet
Terminal=true
StartupNotify=false
Categories=System;Utility;Monitor;
EOF
if command -v update-desktop-database >/dev/null 2>&1; then update-desktop-database "$DESKTOP_DIR" >/dev/null 2>&1 || true; fi

if [ -d /run/systemd/system ]; then
    info "Systemd detected. Installing service..."
    systemctl stop vinet >/dev/null 2>&1 || true
    cat > /etc/systemd/system/vinet.service <<'EOF'
[Unit]
Description=ViNet eBPF Network Tracker
After=network.target

[Service]
ExecStart=/usr/local/bin/vinet daemon
Restart=on-failure
Environment=VINET_DB=/var/lib/vinet/data.db
UMask=0007
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=read-only
ReadWritePaths=/var/lib/vinet /var/log/vinet
StandardOutput=append:/var/log/vinet/daemon.log
StandardError=append:/var/log/vinet/daemon.log

[Install]
WantedBy=multi-user.target
EOF
    systemctl daemon-reload; systemctl enable --now vinet
    info "ViNet installed and started via Systemd!"
elif command -v rc-update >/dev/null 2>&1; then
    warn "OpenRC detected. Install packaging/openrc/vinet from the source checkout."
else
    warn "No systemd or OpenRC detected. Start the daemon manually with:"
    echo "  nohup ${INSTALL_DIR}/vinet daemon >> ${LOG_DIR}/daemon.log 2>&1 &"
fi
echo; info "Done! Run 'vinet' to open the TUI, or 'vinet -l' for live traffic."
