# TASK-051: Implement Universal Auto-Installer & Service Manager (`install.sh`)

## Task Details
- **Requirement**: REQ-051
- **Status**: COMPLETED

## Tasks
1. Create `install.sh` with `uname` OS/Arch auto-detection logic.
2. Add binary installation to `/usr/local/bin/toron`.
3. Add configuration setup in `/etc/toron/` (`config.yaml`, `routes.yaml`, `/etc/toron/public/`).
4. Implement Linux systemd unit generation (`/etc/systemd/system/toron.service`).
5. Implement macOS launchd daemon plist generation (`/Library/LaunchDaemons/com.toron.edgegateway.plist`).
6. Add `--uninstall` routine for clean service teardown and file purging.
7. Update `Makefile` with `install` and `uninstall` targets.
8. Create `docs/wiki/features/installation-guide.md` documentation.
