#!/usr/bin/env bash
# ==============================================================================
# 👑 Toron Edge Gateway — Universal Auto-Installer & Service Manager
# Supports Linux (systemd) and macOS (launchd) on ARM64 & AMD64 architectures
# ==============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TORON_VERSION="$(cat "${SCRIPT_DIR}/VERSION" 2>/dev/null || echo "1.5.29")"
INSTALL_BIN_DIR="/usr/local/bin"
INSTALL_CONF_DIR="/etc/toron"
INSTALL_LOG_DIR="/var/log/toron"
BIN_NAME="toron"
SERVICE_NAME="toron"

# Colors for terminal output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m' # No Color

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

# Print Banner
echo -e "${BOLD}${BLUE}"
echo "=============================================================================="
echo " 👑 Toron Web Server & Edge Gateway — Universal Installer (v${TORON_VERSION})"
echo "=============================================================================="
echo -e "${NC}"

# Check for root/sudo privileges
check_privileges() {
    if [[ $EUID -ne 0 ]]; then
        log_error "This script must be run as root or with sudo privileges.\n       Example: sudo ./install.sh"
    fi
}

# Auto-detect OS and Architecture
detect_os_arch() {
    local os_raw arch_raw
    os_raw="$(uname -s)"
    arch_raw="$(uname -m)"

    case "${os_raw}" in
        Linux*)  OS="linux" ;;
        Darwin*) OS="darwin" ;;
        *)       log_error "Unsupported operating system: ${os_raw}. Toron supports Linux and macOS." ;;
    esac

    case "${arch_raw}" in
        x86_64|amd64)  ARCH="amd64" ;;
        arm64|aarch64) ARCH="arm64" ;;
        *)             log_error "Unsupported CPU architecture: ${arch_raw}. Toron supports amd64 and arm64." ;;
    esac

    TARGET_EXECUTABLE="toron-${OS}-${ARCH}"
    if [[ "${OS}" == "windows" ]]; then
        TARGET_EXECUTABLE="${TARGET_EXECUTABLE}.exe"
    fi

    log_info "Detected OS: ${BOLD}${OS}${NC} | Architecture: ${BOLD}${ARCH}${NC}"
    log_info "Target Binary Name: ${BOLD}${TARGET_EXECUTABLE}${NC}"
}

# Uninstall Toron service and files
uninstall_toron() {
    check_privileges
    detect_os_arch
    log_warn "Starting uninstallation of Toron Edge Gateway..."

    if [[ "${OS}" == "linux" ]]; then
        if command -v systemctl &>/dev/null && systemctl is-active --quiet toron.service 2>/dev/null; then
            log_info "Stopping systemd service toron.service..."
            systemctl stop toron.service || true
            systemctl disable toron.service || true
        fi
        if [[ -f "/etc/systemd/system/toron.service" ]]; then
            rm -f "/etc/systemd/system/toron.service"
            systemctl daemon-reload || true
            log_info "Removed /etc/systemd/system/toron.service"
        fi
    elif [[ "${OS}" == "darwin" ]]; then
        local plist_path="/Library/LaunchDaemons/com.toron.edgegateway.plist"
        if [[ -f "${plist_path}" ]]; then
            log_info "Unloading launchd daemon com.toron.edgegateway..."
            launchctl unload -w "${plist_path}" 2>/dev/null || true
            rm -f "${plist_path}"
            log_info "Removed ${plist_path}"
        fi
    fi

    rm -f "${INSTALL_BIN_DIR}/${BIN_NAME}"
    log_info "Removed binary ${INSTALL_BIN_DIR}/${BIN_NAME}"

    if [[ -d "${INSTALL_CONF_DIR}" ]]; then
        rm -rf "${INSTALL_CONF_DIR}"
        log_info "Removed configuration directory ${INSTALL_CONF_DIR}"
    fi

    if [[ -d "${INSTALL_LOG_DIR}" ]]; then
        rm -rf "${INSTALL_LOG_DIR}"
        log_info "Removed log directory ${INSTALL_LOG_DIR}"
    fi

    rm -f "/etc/logrotate.d/toron" 2>/dev/null || true
    rm -f "/etc/newsyslog.d/toron.conf" 2>/dev/null || true

    log_success "Toron Edge Gateway uninstallation complete!"
    exit 0
}

# Handle command-line arguments
if [[ "${1:-}" == "--uninstall" || "${1:-}" == "-u" ]]; then
    uninstall_toron
fi

check_privileges
detect_os_arch

# Step 1: Locate or compile binary
locate_binary() {
    log_info "Locating executable binary for ${TARGET_EXECUTABLE}..."
    local SCRIPT_DIR
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

    BINARY_SOURCE=""

    # Check local bin/ folder first
    if [[ -f "${SCRIPT_DIR}/bin/${TARGET_EXECUTABLE}" ]]; then
        BINARY_SOURCE="${SCRIPT_DIR}/bin/${TARGET_EXECUTABLE}"
    elif [[ -f "${SCRIPT_DIR}/bin/toron" && "${OS}" == "darwin" && "${ARCH}" == "arm64" ]]; then
        BINARY_SOURCE="${SCRIPT_DIR}/bin/toron"
    elif [[ -f "${SCRIPT_DIR}/toron" ]]; then
        BINARY_SOURCE="${SCRIPT_DIR}/toron"
    fi

    # Build if go is available and binary not found
    if [[ -z "${BINARY_SOURCE}" ]]; then
        if command -v go &>/dev/null; then
            log_info "Local binary not found. Compiling ${TARGET_EXECUTABLE} with Go..."
            mkdir -p "${SCRIPT_DIR}/bin"
            CGO_ENABLED=0 GOOS="${OS}" GOARCH="${ARCH}" go build -o "${SCRIPT_DIR}/bin/${TARGET_EXECUTABLE}" "${SCRIPT_DIR}/cmd/toron"
            BINARY_SOURCE="${SCRIPT_DIR}/bin/${TARGET_EXECUTABLE}"
        else
            log_error "Could not find binary ${TARGET_EXECUTABLE} in ./bin/ and Go is not installed to compile it."
        fi
    fi

    log_success "Found executable binary: ${BINARY_SOURCE}"
}

# Step 2: Install binary to /usr/local/bin
install_binary() {
    log_info "Installing binary to ${INSTALL_BIN_DIR}/${BIN_NAME}..."
    mkdir -p "${INSTALL_BIN_DIR}"
    cp "${BINARY_SOURCE}" "${INSTALL_BIN_DIR}/${BIN_NAME}"
    chmod 755 "${INSTALL_BIN_DIR}/${BIN_NAME}"
    log_success "Binary installed to ${INSTALL_BIN_DIR}/${BIN_NAME}"
}

# Step 3: Set up configuration & public assets in /etc/toron
install_configuration() {
    log_info "Setting up configuration directory at ${INSTALL_CONF_DIR}..."
    local SCRIPT_DIR
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

    mkdir -p "${INSTALL_CONF_DIR}"
    mkdir -p "${INSTALL_CONF_DIR}/public"
    mkdir -p "${INSTALL_LOG_DIR}"

    # Copy config.yaml if not present
    if [[ -f "${SCRIPT_DIR}/config.yaml" ]]; then
        if [[ ! -f "${INSTALL_CONF_DIR}/config.yaml" ]]; then
            cp "${SCRIPT_DIR}/config.yaml" "${INSTALL_CONF_DIR}/config.yaml"
            log_success "Installed ${INSTALL_CONF_DIR}/config.yaml"
        else
            log_warn "Existing ${INSTALL_CONF_DIR}/config.yaml preserved."
        fi
    fi

    # Copy routes.yaml if present
    if [[ -f "${SCRIPT_DIR}/routes.yaml" ]]; then
        if [[ ! -f "${INSTALL_CONF_DIR}/routes.yaml" ]]; then
            cp "${SCRIPT_DIR}/routes.yaml" "${INSTALL_CONF_DIR}/routes.yaml"
            log_success "Installed ${INSTALL_CONF_DIR}/routes.yaml"
        else
            log_warn "Existing ${INSTALL_CONF_DIR}/routes.yaml preserved."
        fi
    fi

    # Copy public dashboard assets
    if [[ -d "${SCRIPT_DIR}/public" ]]; then
        cp -r "${SCRIPT_DIR}/public/"* "${INSTALL_CONF_DIR}/public/"
        log_success "Installed static dashboard UI assets to ${INSTALL_CONF_DIR}/public/"
    fi

    # Update static dir path in /etc/toron/config.yaml to absolute /etc/toron/public if needed
    if [[ -f "${INSTALL_CONF_DIR}/config.yaml" ]]; then
        sed -i.bak 's|dir: "./public"|dir: "/etc/toron/public"|g' "${INSTALL_CONF_DIR}/config.yaml" 2>/dev/null || true
        rm -f "${INSTALL_CONF_DIR}/config.yaml.bak" 2>/dev/null || true
    fi
}

# Step 4: Install System Service (systemd for Linux, launchd for macOS)
install_service() {
    log_info "Installing background system service for ${OS}..."

    if [[ "${OS}" == "linux" ]]; then
        local service_file="/etc/systemd/system/toron.service"
        log_info "Creating systemd unit file at ${service_file}..."

        cat <<EOF > "${service_file}"
[Unit]
Description=Toron Edge Gateway & Web Control Center
After=network.target remote-fs.target
Wants=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=/etc/toron
ExecStart=${INSTALL_BIN_DIR}/${BIN_NAME} -config /etc/toron/config.yaml
Restart=on-failure
RestartSec=3s
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF

        chmod 644 "${service_file}"
        if command -v systemctl &>/dev/null; then
            log_info "Reloading systemd daemon, enabling and starting toron.service..."
            systemctl daemon-reload
            systemctl enable toron.service
            systemctl restart toron.service || true
            log_success "systemd service toron.service installed and started!"
        fi

    elif [[ "${OS}" == "darwin" ]]; then
        local plist_file="/Library/LaunchDaemons/com.toron.edgegateway.plist"
        log_info "Creating launchd daemon plist at ${plist_file}..."

        cat <<EOF > "${plist_file}"
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.toron.edgegateway</string>
    <key>ProgramArguments</key>
    <array>
        <string>${INSTALL_BIN_DIR}/${BIN_NAME}</string>
        <string>-config</string>
        <string>/etc/toron/config.yaml</string>
    </array>
    <key>WorkingDirectory</key>
    <string>/etc/toron</string>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>${INSTALL_LOG_DIR}/toron.log</string>
    <key>StandardErrorPath</key>
    <string>${INSTALL_LOG_DIR}/toron.error.log</string>
</dict>
</plist>
EOF

        chmod 644 "${plist_file}"
        log_info "Loading launchd service com.toron.edgegateway..."
        launchctl unload -w "${plist_file}" 2>/dev/null || true
        launchctl load -w "${plist_file}"
        log_success "launchd service com.toron.edgegateway installed and loaded!"
    fi
}

# Step 5: Install Log Rotation Configuration (daily rolling + gzip compression)
install_log_rotation() {
    log_info "Configuring daily log rotation and gzip compression for ${OS}..."

    if [[ "${OS}" == "linux" ]]; then
        if [[ -d "/etc/logrotate.d" ]]; then
            local logrotate_file="/etc/logrotate.d/toron"
            log_info "Writing logrotate configuration at ${logrotate_file}..."

            cat <<EOF > "${logrotate_file}"
/var/log/toron/*.log {
    daily
    rotate 7
    compress
    delaycompress
    missingok
    notifempty
    copytruncate
}
EOF
            chmod 644 "${logrotate_file}"
            log_success "Linux logrotate configuration installed at ${logrotate_file} (daily, 7 rotations, gzip compressed)"
        fi

    elif [[ "${OS}" == "darwin" ]]; then
        if [[ -d "/etc/newsyslog.d" ]]; then
            local newsyslog_file="/etc/newsyslog.d/toron.conf"
            log_info "Writing newsyslog configuration at ${newsyslog_file}..."

            cat <<'EOF' > "${newsyslog_file}"
# logfilename                      [owner:group]  mode count size when  flags [/pid_file] [sig_num]
/var/log/toron/*.log                              644  7     *    $D0   Z
EOF
            chmod 644 "${newsyslog_file}"
            log_success "macOS newsyslog configuration installed at ${newsyslog_file} (daily midnight \$D0, 7 rotations, gzip compressed)"
        fi
    fi
}

# Main Execution Flow
locate_binary
install_binary
install_configuration
install_service
install_log_rotation

echo ""
echo -e "${BOLD}${GREEN}=============================================================================="
echo " 🎉 Toron Edge Gateway Installation Completed Successfully!"
echo "=============================================================================="
echo -e "${NC}"
echo -e "  • ${BOLD}Binary Location:${NC}     ${INSTALL_BIN_DIR}/${BIN_NAME}"
echo -e "  • ${BOLD}Config Directory:${NC}    ${INSTALL_CONF_DIR}/ (config.yaml, routes.yaml)"
echo -e "  • ${BOLD}Web Dashboard:${NC}       http://localhost:8080/internal/dashboard/"
echo -e "  • ${BOLD}Log Files:${NC}           ${INSTALL_LOG_DIR}/"
if [[ "${OS}" == "linux" ]]; then
echo -e "  • ${BOLD}Service Management:${NC}  systemctl status toron | systemctl restart toron"
elif [[ "${OS}" == "darwin" ]]; then
echo -e "  • ${BOLD}Service Management:${NC}  sudo launchctl list | grep toron"
fi
echo -e "  • ${BOLD}Uninstallation:${NC}      sudo ./install.sh --uninstall"
echo ""
