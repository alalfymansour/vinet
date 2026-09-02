<div align="center">

<img src="vinet.svg" alt="ViNet logo" width="180">

# ViNet

**Per-process network usage for Linux.**

[![Release](https://img.shields.io/github/v/release/alalfymansour/vinet?style=flat-square&label=release)](https://github.com/alalfymansour/vinet/releases/latest)
[![License](https://img.shields.io/badge/license-GPL--2.0-blue?style=flat-square)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-linux-lightgrey?style=flat-square)](#requirements)
[![Arch](https://img.shields.io/badge/arch-amd64%20%7C%20arm64-lightgrey?style=flat-square)](#requirements)

ViNet uses eBPF to collect network traffic, SQLite to store it locally, and a
terminal interface to display usage by process and destination.

---

</div>

## Install

One-liner — downloads the prebuilt binary for Linux `amd64` or `arm64` and
starts the collector service when systemd is available. No build tools needed.

```bash
curl -fsSL https://github.com/alalfymansour/vinet/raw/refs/heads/main/install.sh | bash
```

## Help

All commands, options, and keyboard shortcuts are available through the
built-in help:

```bash
vinet -h
```

## Requirements

| | |
|---|---|
| **OS** | Linux with eBPF support |
| **Arch** | `amd64` or `arm64` |
| **Privileges** | Root, for installation and collection |

## Development

Release binaries are built automatically for `amd64` and `arm64` when a
version tag is pushed. To build locally, install **Go 1.26+**, **Clang**,
**LLVM**, and libbpf development headers, then run:

```bash
go generate ./...
go build -o vinet .
```

## Uninstall

```bash
sudo systemctl disable --now vinet 2>/dev/null || true
sudo rm -f /usr/local/bin/vinet /etc/systemd/system/vinet.service
sudo rm -f /usr/share/applications/vinet.desktop
sudo rm -f /usr/share/icons/hicolor/scalable/apps/vinet.svg
```

---

<div align="center">

ViNet is licensed under **GPL-2.0** — see [LICENSE](LICENSE).

</div>

