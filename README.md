<p align="center">
  <img src="vinet.svg" alt="ViNet logo" width="180">
</p>

<h1 align="center">ViNet</h1>

ViNet is a small Linux tool that shows which processes are using your
network and how much data they send and receive. It uses eBPF for collection,
SQLite for local storage, and a terminal dashboard for viewing the results.

## Install

The installer detects whether the machine is Linux `amd64` or `arm64`, then
downloads the matching prebuilt ViNet release binary. Go, Clang, LLVM, and
libbpf development headers are not required on the target machine.

### Install the latest release

On Debian, Fedora, Arch, or Gentoo:

```bash
curl -fsSL https://github.com/alalfymansour/vinet/raw/refs/heads/main/install.sh | bash
```

The installer asks for root access, detects the CPU architecture, downloads
the matching release asset, installs the command and icon, and starts the
background collector.

### Install from a checkout

```bash
sudo ./install.sh
```

This uses the same prebuilt release flow. Only `curl` or `wget` is required
for downloading, plus `sudo` for installation.

### Build from source for development

Release users should use the prebuilt installer above. Developers building
locally need Go 1.26+, Clang, LLVM, libbpf development headers, and the usual
build tools:

```bash
go generate ./...
go build -o vinet .
```

Tagged releases are built automatically for Linux `amd64` and `arm64` and
publish `vinet-linux-amd64`, `vinet-linux-arm64`, and `SHA256SUMS` assets.

## What gets installed

- `/usr/local/bin/vinet` — the CLI, TUI, and daemon
- `/var/lib/vinet/data.db` — collected traffic
- `/var/log/vinet/daemon.log` — daemon log
- `vinet.service` on systemd, or an OpenRC service
- `/usr/share/applications/vinet.desktop` — desktop launcher
- `/usr/share/icons/hicolor/scalable/apps/vinet.svg` — application icon

The desktop launcher opens the terminal dashboard.

## Use ViNet

Open the dashboard:

```bash
vinet
```

Choose a time range, optionally for one process:

```bash
vinet -d              # today, opens the dashboard
vinet -w firefox      # Firefox this week
vinet -m firefox      # Firefox this month
```

Useful shortcuts:

```bash
vinet -l              # live traffic, refreshed every two seconds
vinet -l firefox      # live traffic for Firefox
vinet -s              # collector and database status
vinet -v              # installed version
vinet -h              # help and examples
```

The same long options are available: `--live`, `--status`, `--version`, and
`--help`.

## Export and import

Export today’s recorded traffic as JSON or CSV. The extension chooses the
format:

```bash
vinet export traffic.json
vinet export traffic.csv
```

Without a filename, JSON is printed to the terminal. Import an export later:

```bash
vinet import traffic.json
vinet import traffic.csv
```

Imports add records to the database in one transaction and do not replace
existing records.

## Check the collector

```bash
vinet -s
```

Example:

```text
ViNet status: running
Service: systemd: active
Database: /var/lib/vinet/data.db
Records: 3262
Last heartbeat: 2026-09-02 12:44:20 UTC
Collector error: none
```

For systemd:

```bash
sudo systemctl status vinet
sudo journalctl -u vinet -f
```

For OpenRC:

```bash
sudo rc-service vinet status
sudo rc-service vinet restart
```

## Troubleshooting

If the dashboard is empty, check the daemon first:

```bash
vinet -s
```

Traffic is collected only while the daemon is running. A new installation
starts with an empty database, so data appears after network activity occurs.

If eBPF fails to load, run the daemon in the foreground to see the full error:

```bash
sudo vinet daemon
```

The daemon needs a Linux kernel with eBPF support and permission to attach
kernel probes.

## Build from source

```bash
go generate ./...
go build -o vinet .
./vinet -h
```

To make the command available everywhere after a local build:

```bash
sudo install -m 0755 vinet /usr/local/bin/vinet
```

## Uninstall

Stop and remove the service and binary:

```bash
sudo systemctl disable --now vinet 2>/dev/null || true
sudo rc-service vinet stop 2>/dev/null || true
sudo rm -f /etc/systemd/system/vinet.service /etc/init.d/vinet
sudo rm -f /usr/local/bin/vinet
```

Remove the database, logs, desktop launcher, and icon if you no longer need
them:

```bash
rm -rf ~/.config/vinet
sudo rm -rf /var/lib/vinet /var/log/vinet
sudo rm -f /usr/share/applications/vinet.desktop
sudo rm -f /usr/share/icons/hicolor/scalable/apps/vinet.svg
```

ViNet is licensed under GPL-2.0. See [LICENSE](LICENSE).
