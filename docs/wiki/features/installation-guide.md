# 📦 Toron Edge Gateway — Universal Installation & Service Manager Guide

Toron provides a production-grade, universal auto-installation script (`install.sh`) designed for zero-dependency deployment across **Linux (systemd)** and **macOS (launchd)** on both **ARM64** and **AMD64** architectures.

---

## ⚡ Quick Installation

Run the universal installer with `sudo` privileges:

```bash
# Clone or navigate to the Toron repository
cd toron_v3

# Run the installer
sudo ./install.sh

# Or via Makefile shortcut
sudo make install
```

---

## 🔍 What `install.sh` Does Automatically

1. **OS & Architecture Auto-Detection**:
   - Detects Operating System: `Linux` or `Darwin` (macOS).
   - Detects CPU Architecture: `amd64` (x86_64) or `arm64` (Apple Silicon / aarch64).
   - Selects the exact pre-compiled binary (e.g., `toron-linux-amd64`, `toron-darwin-arm64`).

2. **Binary Installation (`/usr/local/bin/toron`)**:
   - Places executable in system PATH at `/usr/local/bin/toron`.
   - Sets execution permissions (`chmod 755`).

3. **System Configuration (`/etc/toron/`)**:
   - Creates system configuration directory `/etc/toron/`.
   - Copies `config.yaml` and `routes.yaml`.
   - Installs static Web Control Center UI dashboard files to `/etc/toron/public/`.

4. **Background Service Registration**:
   - **Linux (`systemd`)**: Creates `/etc/systemd/system/toron.service`, reloads systemctl, enables, and starts the service.
   - **macOS (`launchd`)**: Creates `/Library/LaunchDaemons/com.toron.edgegateway.plist`, loads, and starts the daemon.

5. **Daily Log Rotation & Compression**:
   - **Linux (`logrotate`)**: Writes `/etc/logrotate.d/toron` for daily rotation, keeping 7 archives with `gzip` compression.
   - **macOS (`newsyslog`)**: Writes `/etc/newsyslog.d/toron.conf` for daily midnight (`$D0`) rotation, keeping 7 archives with `gzip` (`Z`) compression.

---

## ⚙️ System Service Management

### Linux (`systemd`)
```bash
# Check service status
sudo systemctl status toron

# Restart service after configuration changes
sudo systemctl restart toron

# View live systemd service logs
sudo journalctl -u toron -f
```

### macOS (`launchd`)
```bash
# Check running daemon status
sudo launchctl list | grep toron

# Stop / Start daemon
sudo launchctl unload -w /Library/LaunchDaemons/com.toron.edgegateway.plist
sudo launchctl load -w /Library/LaunchDaemons/com.toron.edgegateway.plist

# View log files
tail -f /var/log/toron/toron.log
tail -f /var/log/toron/toron.error.log
```

---

## 🗑️ Uninstallation

To cleanly remove Toron, stop services, and purge system configuration directories:

```bash
sudo ./install.sh --uninstall

# Or via Makefile shortcut
sudo make uninstall
```
