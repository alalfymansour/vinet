#!/usr/bin/env bash
# ViNet Universal Installer
#
# Pipeable one-liner:
#   curl -fsSL https://raw.githubusercontent.com/alalfymansour/vinet/main/install.sh | bash
#
# Or from a checkout:
#   chmod +x install.sh && sudo ./install.sh
#
# The script compiles ViNet from source (eBPF bytecode must be built on
# the target machine), installs it to /usr/local/bin, and registers a
# background daemon with systemd, OpenRC, or prints manual instructions.
set -euo pipefail

SCRIPT_URL="https://raw.githubusercontent.com/alalfymansour/vinet/main/install.sh"
REPO_TARBALL="https://github.com/alalfymansour/vinet/archive/refs/heads/main.tar.gz"
INSTALL_DIR="/usr/local/bin"
LOG_DIR="/var/log/vinet"
DATA_DIR="/var/lib/vinet"
DB_GROUP="vinet"
ICON_DIR="/usr/share/icons/hicolor/scalable/apps"
DESKTOP_DIR="/usr/share/applications"

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
NC='\033[0m'

info() { echo -e "${GREEN}==> $1${NC}"; }
warn() { echo -e "${YELLOW}==> $1${NC}"; }
die()  { echo -e "${RED}==> $1${NC}" >&2; exit 1; }

install_dependencies() {
    info "Installing build dependencies..."
    if command -v apt-get >/dev/null 2>&1; then
        apt-get update
        apt-get install -y golang clang llvm libbpf-dev curl tar
    elif command -v dnf >/dev/null 2>&1; then
        dnf install -y golang clang llvm libbpf-devel curl tar
    elif command -v pacman >/dev/null 2>&1; then
        pacman -Sy --needed --noconfirm go clang llvm libbpf curl tar
    elif command -v emerge >/dev/null 2>&1; then
        emerge dev-lang/go llvm-core/clang dev-libs/libbpf net-misc/curl app-arch/tar
    else
        die "Could not detect a supported package manager. Install Go, Clang, LLVM, libbpf headers, curl, and tar manually."
    fi
}

INSTALL_DEPS=0
for arg in "$@"; do
    case "$arg" in
        --install-deps) INSTALL_DEPS=1 ;;
        -h|--help)
            cat <<'EOF'
ViNet installer

Usage:
  sudo ./install.sh
  curl -fsSL https://raw.githubusercontent.com/alalfymansour/vinet/main/install.sh | bash

Options:
  --install-deps  Install missing build packages using the detected package manager
  -h, --help      Show this help
EOF
            exit 0
            ;;
        *) die "Unknown option: $arg (use --help for usage)" ;;
    esac
done

# --------------------------------------------------------------------------
# 1. Elevate to root
# --------------------------------------------------------------------------
# Must support "curl ... | bash": when piped, the script is read from stdin
# and "$0" is not a file, so re-download it to a temp file before using sudo.
if [ "${EUID:-$(id -u)}" -ne 0 ]; then
    warn "Root privileges required. Re-running with sudo..."
    if [ -f "$0" ]; then
        exec sudo -E bash "$0" "$@"
    fi
    TMP_SCRIPT="$(mktemp /tmp/vinet-install.XXXXXX.sh)"
    command -v curl >/dev/null 2>&1 || die "curl is required to run the piped installer."
    curl -fsSL "$SCRIPT_URL" -o "$TMP_SCRIPT"
    exec sudo -E bash "$TMP_SCRIPT" "$@"
fi

# --------------------------------------------------------------------------
# 2. Check dependencies
# --------------------------------------------------------------------------
info "Checking dependencies..."
MISSING=0

command -v go >/dev/null 2>&1 || { echo -e "${RED}Error: Go (go) is not installed.${NC}"; MISSING=1; }
command -v clang >/dev/null 2>&1 || { echo -e "${RED}Error: Clang (clang) is not installed.${NC}"; MISSING=1; }
command -v tar >/dev/null 2>&1 || { echo -e "${RED}Error: tar is not installed.${NC}"; MISSING=1; }
command -v curl >/dev/null 2>&1 || command -v wget >/dev/null 2>&1 || { echo -e "${RED}Error: curl (or wget) is required.${NC}"; MISSING=1; }

# The eBPF C program includes bpf_helpers.h from libbpf's headers.
if [ ! -f /usr/include/bpf/bpf_helpers.h ]; then
    echo -e "${RED}Error: libbpf development headers not found in /usr/include/bpf.${NC}"
    MISSING=1
fi

if [ "$MISSING" -eq 1 ]; then
    if [ "$INSTALL_DEPS" -eq 1 ]; then
        install_dependencies
        exec "$0" "$@"
    fi
    echo
    warn "Please install the missing dependencies and try again:"
    echo "  Debian/Ubuntu:  sudo apt install golang clang llvm libbpf-dev curl"
    echo "  Fedora/RHEL:    sudo dnf install golang clang llvm libbpf-devel curl"
    echo "  Arch:           sudo pacman -S go clang llvm libbpf curl"
    echo "  Gentoo:         sudo emerge dev-lang/go llvm-core/clang dev-libs/libbpf net-misc/curl"
    exit 1
fi

# go.mod requires Go 1.26 or newer; warn early with a clear message.
GO_MINOR="$(go env GOVERSION 2>/dev/null | sed -n 's/^go1\.\([0-9]*\).*/\1/p' || true)"
if [ -n "$GO_MINOR" ] && [ "$GO_MINOR" -lt 26 ]; then
    warn "Go 1.26+ is required (found $(go env GOVERSION)). The build step may fail."
fi

# --------------------------------------------------------------------------
# 3. Select or fetch the source
# --------------------------------------------------------------------------
fetch() { # fetch <url> <outfile>
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL "$1" -o "$2"
    else
        wget -qO "$2" "$1"
    fi
}

SOURCE_DIR=""
if [ -f "$PWD/go.mod" ] && [ -f "$PWD/bpf/tracker.c" ]; then
    SOURCE_DIR="$PWD"
elif [ -f "$0" ]; then
    SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
    if [ -f "$SCRIPT_DIR/go.mod" ] && [ -f "$SCRIPT_DIR/bpf/tracker.c" ]; then
        SOURCE_DIR="$SCRIPT_DIR"
    fi
fi

if [ -n "$SOURCE_DIR" ]; then
    info "Using local ViNet source at $SOURCE_DIR..."
    cd "$SOURCE_DIR"
else
    TMP_DIR="$(mktemp -d)"
    trap 'rm -rf "$TMP_DIR"' EXIT
    cd "$TMP_DIR"
    info "Downloading ViNet source..."
    fetch "$REPO_TARBALL" source.tar.gz
    tar xzf source.tar.gz
    cd vinet-main
fi

# --------------------------------------------------------------------------
# 4. Compile (eBPF bytecode + Go binary)
# --------------------------------------------------------------------------
info "Compiling eBPF bytecode and Go binary (this can take a minute)..."
go generate ./...
go build -o vinet .

# --------------------------------------------------------------------------
# 5. Install binary
# --------------------------------------------------------------------------
info "Installing binary to ${INSTALL_DIR}..."
mkdir -p "$INSTALL_DIR" "$LOG_DIR" "$DATA_DIR" "$ICON_DIR" "$DESKTOP_DIR"
if ! getent group "$DB_GROUP" >/dev/null 2>&1; then
    groupadd --system "$DB_GROUP"
fi
if [ -n "${SUDO_USER:-}" ] && id "$SUDO_USER" >/dev/null 2>&1; then
    usermod -a -G "$DB_GROUP" "$SUDO_USER"
fi
chown root:"$DB_GROUP" "$DATA_DIR"
chmod 2770 "$DATA_DIR"
if [ -f "$DATA_DIR/data.db" ]; then
    chown root:"$DB_GROUP" "$DATA_DIR/data.db"
    chmod 0660 "$DATA_DIR/data.db"
fi
chmod 0750 "$LOG_DIR"
install -m 0755 vinet "$INSTALL_DIR/vinet"
install -m 0644 vinet.svg "$ICON_DIR/vinet.svg"
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
if command -v update-desktop-database >/dev/null 2>&1; then
    update-desktop-database "$DESKTOP_DIR" >/dev/null 2>&1 || true
fi

# --------------------------------------------------------------------------
# 6. Register a background daemon with the init system
# --------------------------------------------------------------------------
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
    systemctl daemon-reload
    systemctl enable --now vinet
    info "ViNet installed and started via Systemd!"

elif command -v rc-update >/dev/null 2>&1; then
    info "OpenRC detected. Installing service..."
    rc-service vinet stop >/dev/null 2>&1 || true
    install -m 0755 packaging/openrc/vinet /etc/init.d/vinet
    chmod +x /etc/init.d/vinet
    rc-update add vinet default
    rc-service vinet start
    info "ViNet installed and started via OpenRC!"

else
    warn "No systemd or OpenRC detected."
    echo "Start the daemon manually in the background with:"
    echo "  nohup ${INSTALL_DIR}/vinet daemon >> ${LOG_DIR}/daemon.log 2>&1 &"
fi

echo
info "Done! Run 'vinet' to open the TUI, or 'vinet <process>' for a quick CLI query."
