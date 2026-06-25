#!/bin/bash

set -u

red='\033[0;31m'
green='\033[0;32m'
yellow='\033[0;33m'
plain='\033[0m'

GH_REPO="nicelic/kwor"
INSTALL_SCRIPT_URL="https://raw.githubusercontent.com/${GH_REPO}/main/install.sh"
DEFAULT_INSTALL_DIR="/opt/kwor"
SERVICE_NAME="kwor"
RUNTIME_SUPPORT_DIR_NAME="Promanager_data"

RELEASE=""
ARCH=""
TARGET_VERSION=""
INSTALL_DIR=""
INSTALL_SOURCE=""
RUNNING_BIN_PATH=""
SERVICE_BIN_PATH=""
SERVICE_FILE_PATH=""
DOWNLOAD_URL=""
ARCHIVE_PATH=""
WORK_DIR=""
STAGED_BIN_PATH=""
STAGED_INSTALL_SCRIPT_PATH=""
STAGED_SERVICE_FILE_PATH=""
TARGET_BIN_PATH=""
BACKUP_BIN_PATH=""
STOP_BIN_PATH=""
TARGET_BIN_NAME="kwor"
TARGET_SUPPORT_DIR=""
TARGET_INSTALL_SCRIPT_PATH=""
TARGET_SERVICE_COPY_PATH=""

cleanup() {
    if [[ -n "${ARCHIVE_PATH}" && -f "${ARCHIVE_PATH}" ]]; then
        rm -f "${ARCHIVE_PATH}"
    fi
    if [[ -n "${WORK_DIR}" && -d "${WORK_DIR}" ]]; then
        rm -f "${WORK_DIR}/kwor" 2>/dev/null || true
        rm -f "${WORK_DIR}/install.sh" 2>/dev/null || true
        rm -f "${WORK_DIR}/install.sh.latest" 2>/dev/null || true
        rm -f "${WORK_DIR}/kwor.service" 2>/dev/null || true
        rm -f "${WORK_DIR}/kwor/kwor" 2>/dev/null || true
        rm -f "${WORK_DIR}/kwor/install.sh" 2>/dev/null || true
        rm -f "${WORK_DIR}/kwor/kwor.service" 2>/dev/null || true
        rmdir "${WORK_DIR}/kwor" 2>/dev/null || true
        rmdir "${WORK_DIR}" 2>/dev/null || true
    fi
}

trap cleanup EXIT

log_info() {
    echo -e "${green}$1${plain}"
}

log_warn() {
    echo -e "${yellow}$1${plain}"
}

log_error() {
    echo -e "${red}$1${plain}" >&2
}

require_root() {
    if [[ "${EUID}" -ne 0 ]]; then
        log_error "Fatal error: please run this script with root privilege"
        exit 1
    fi
}

detect_os() {
    if [[ -f /etc/os-release ]]; then
        # shellcheck disable=SC1091
        source /etc/os-release
        RELEASE="${ID}"
    elif [[ -f /usr/lib/os-release ]]; then
        # shellcheck disable=SC1091
        source /usr/lib/os-release
        RELEASE="${ID}"
    else
        log_error "Failed to detect Linux distribution"
        exit 1
    fi
    log_info "Detected OS: ${RELEASE}"
}

detect_arch() {
    case "$(uname -m)" in
        x86_64 | x64 | amd64) ARCH="amd64" ;;
        armv8* | armv8 | arm64 | aarch64) ARCH="arm64" ;;
        *)
            log_error "Unsupported CPU architecture: $(uname -m). Only amd64 and arm64 are supported."
            exit 1
            ;;
    esac
    log_info "Detected architecture: ${ARCH}"
}

install_base_deps() {
    case "${RELEASE}" in
        centos | almalinux | rocky | oracle)
            yum -y update && yum install -y -q wget curl tar tzdata
            ;;
        fedora)
            dnf -y update && dnf install -y -q wget curl tar tzdata
            ;;
        arch | manjaro | parch)
            pacman -Syu --noconfirm wget curl tar tzdata
            ;;
        opensuse-tumbleweed)
            zypper refresh && zypper -q install -y wget curl tar timezone
            ;;
        *)
            apt-get update && apt-get install -y -q wget curl tar tzdata
            ;;
    esac
}

normalize_version_tag() {
    local raw_tag="$1"
    raw_tag="$(echo "${raw_tag}" | tr -d '\r' | xargs)"
    if [[ -z "${raw_tag}" ]]; then
        echo ""
        return
    fi
    if [[ "${raw_tag}" =~ ^v ]]; then
        echo "${raw_tag}"
    else
        echo "v${raw_tag}"
    fi
}

resolve_target_version() {
    if [[ $# -eq 0 || -z "${1:-}" ]]; then
        TARGET_VERSION="$(curl -Ls "https://api.github.com/repos/${GH_REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')"
        if [[ -z "${TARGET_VERSION}" ]]; then
            log_error "Failed to fetch latest kwor release version from GitHub"
            exit 1
        fi
        log_info "Using latest release: ${TARGET_VERSION}"
    else
        TARGET_VERSION="$(normalize_version_tag "$1")"
        log_info "Using specified release: ${TARGET_VERSION}"
    fi
}

find_running_pid() {
    local pid
    pid="$(pgrep -x kwor 2>/dev/null | head -n 1 || true)"
    if [[ -n "${pid}" ]]; then
        echo "${pid}"
        return
    fi
    pid="$(pgrep -x kwor_amd64 2>/dev/null | head -n 1 || true)"
    if [[ -n "${pid}" ]]; then
        echo "${pid}"
        return
    fi
    pid="$(pgrep -x kwor_arm64 2>/dev/null | head -n 1 || true)"
    if [[ -n "${pid}" ]]; then
        echo "${pid}"
        return
    fi
    echo ""
}

resolve_running_bin_path() {
    local pid exe_path
    pid="$(find_running_pid)"
    if [[ -z "${pid}" ]]; then
        return
    fi
    if [[ -L "/proc/${pid}/exe" ]]; then
        exe_path="$(readlink -f "/proc/${pid}/exe" 2>/dev/null || true)"
        if [[ -n "${exe_path}" ]]; then
            RUNNING_BIN_PATH="${exe_path}"
        fi
    fi
}

find_service_file() {
    local candidate
    for candidate in \
        "/etc/systemd/system/${SERVICE_NAME}.service" \
        "/lib/systemd/system/${SERVICE_NAME}.service" \
        "/usr/lib/systemd/system/${SERVICE_NAME}.service"
    do
        if [[ -f "${candidate}" ]]; then
            echo "${candidate}"
            return
        fi
    done
    echo ""
}

extract_execstart_path() {
    local service_path="$1"
    local line exec_value first_token
    line="$(grep -E '^ExecStart=' "${service_path}" 2>/dev/null | head -n 1 || true)"
    if [[ -z "${line}" ]]; then
        echo ""
        return
    fi
    exec_value="${line#ExecStart=}"
    exec_value="$(echo "${exec_value}" | tr -d '\r' | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
    if [[ "${exec_value}" == \"*\" ]]; then
        exec_value="${exec_value#\"}"
        first_token="${exec_value%%\"*}"
    else
        first_token="${exec_value%% *}"
    fi
    first_token="$(echo "${first_token}" | sed 's/\\x20/ /g')"
    if [[ -n "${first_token}" && -e "${first_token}" ]]; then
        echo "$(readlink -f "${first_token}" 2>/dev/null || echo "${first_token}")"
        return
    fi
    echo "${first_token}"
}

extract_working_directory() {
    local service_path="$1"
    local line value
    line="$(grep -E '^WorkingDirectory=' "${service_path}" 2>/dev/null | head -n 1 || true)"
    if [[ -z "${line}" ]]; then
        echo ""
        return
    fi
    value="${line#WorkingDirectory=}"
    value="$(echo "${value}" | tr -d '\r' | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
    value="${value%\"}"
    value="${value#\"}"
    value="$(echo "${value}" | sed 's/\\x20/ /g')"
    echo "${value}"
}

resolve_service_bin_path() {
    SERVICE_FILE_PATH="$(find_service_file)"
    if [[ -z "${SERVICE_FILE_PATH}" ]]; then
        return
    fi

    SERVICE_BIN_PATH="$(extract_execstart_path "${SERVICE_FILE_PATH}")"
    if [[ -n "${SERVICE_BIN_PATH}" ]]; then
        return
    fi

    local working_dir
    working_dir="$(extract_working_directory "${SERVICE_FILE_PATH}")"
    if [[ -n "${working_dir}" ]]; then
        if [[ -f "${working_dir}/kwor" ]]; then
            SERVICE_BIN_PATH="$(readlink -f "${working_dir}/kwor" 2>/dev/null || echo "${working_dir}/kwor")"
            return
        fi
        if [[ -f "${working_dir}/kwor_amd64" ]]; then
            SERVICE_BIN_PATH="$(readlink -f "${working_dir}/kwor_amd64" 2>/dev/null || echo "${working_dir}/kwor_amd64")"
            return
        fi
        if [[ -f "${working_dir}/kwor_arm64" ]]; then
            SERVICE_BIN_PATH="$(readlink -f "${working_dir}/kwor_arm64" 2>/dev/null || echo "${working_dir}/kwor_arm64")"
            return
        fi
    fi
}

resolve_install_dir() {
    resolve_running_bin_path
    if [[ -n "${RUNNING_BIN_PATH}" ]]; then
        INSTALL_DIR="$(dirname "${RUNNING_BIN_PATH}")"
        INSTALL_SOURCE="running process"
        STOP_BIN_PATH="${RUNNING_BIN_PATH}"
        return
    fi

    resolve_service_bin_path
    if [[ -n "${SERVICE_BIN_PATH}" ]]; then
        INSTALL_DIR="$(dirname "${SERVICE_BIN_PATH}")"
        INSTALL_SOURCE="systemd service"
        STOP_BIN_PATH="${SERVICE_BIN_PATH}"
        return
    fi

    INSTALL_DIR="${DEFAULT_INSTALL_DIR}"
    INSTALL_SOURCE="default"
    STOP_BIN_PATH=""
}

download_release_archive() {
    ARCHIVE_PATH="/tmp/kwor-${TARGET_VERSION}-${ARCH}.tar.gz"
    DOWNLOAD_URL="https://github.com/${GH_REPO}/releases/download/${TARGET_VERSION}/kwor-linux-${ARCH}.tar.gz"
    log_info "Downloading ${DOWNLOAD_URL}"
    if ! wget -q --show-progress --no-check-certificate -O "${ARCHIVE_PATH}" "${DOWNLOAD_URL}"; then
        log_error "Failed to download release archive: ${DOWNLOAD_URL}"
        exit 1
    fi
}

extract_release_archive() {
    WORK_DIR="$(mktemp -d /tmp/kwor-install.XXXXXX)"
    if ! tar -xzf "${ARCHIVE_PATH}" -C "${WORK_DIR}"; then
        log_error "Failed to extract archive: ${ARCHIVE_PATH}"
        exit 1
    fi

    if [[ -f "${WORK_DIR}/kwor" ]]; then
        STAGED_BIN_PATH="${WORK_DIR}/kwor"
    elif [[ -f "${WORK_DIR}/kwor/kwor" ]]; then
        STAGED_BIN_PATH="${WORK_DIR}/kwor/kwor"
    else
        log_error "Release archive does not contain kwor binary"
        exit 1
    fi

    if [[ -f "${WORK_DIR}/install.sh" ]]; then
        STAGED_INSTALL_SCRIPT_PATH="${WORK_DIR}/install.sh"
    elif [[ -f "${WORK_DIR}/kwor/install.sh" ]]; then
        STAGED_INSTALL_SCRIPT_PATH="${WORK_DIR}/kwor/install.sh"
    fi
}

prepare_install_dir() {
    mkdir -p "${INSTALL_DIR}"
    TARGET_SUPPORT_DIR="${INSTALL_DIR}/${RUNTIME_SUPPORT_DIR_NAME}"
    mkdir -p "${TARGET_SUPPORT_DIR}"
    if [[ -n "${SERVICE_BIN_PATH}" ]]; then
        TARGET_BIN_NAME="$(basename "${SERVICE_BIN_PATH}")"
    elif [[ -n "${RUNNING_BIN_PATH}" ]]; then
        TARGET_BIN_NAME="$(basename "${RUNNING_BIN_PATH}")"
    else
        TARGET_BIN_NAME="kwor"
    fi

    case "${TARGET_BIN_NAME}" in
        kwor | kwor_amd64 | kwor_arm64) ;;
        *) TARGET_BIN_NAME="kwor" ;;
    esac

    TARGET_BIN_PATH="${INSTALL_DIR}/${TARGET_BIN_NAME}"
    BACKUP_BIN_PATH="${TARGET_BIN_PATH}.bak"
    TARGET_INSTALL_SCRIPT_PATH="${TARGET_SUPPORT_DIR}/install.sh"
    TARGET_SERVICE_COPY_PATH="${TARGET_SUPPORT_DIR}/kwor.service"
}

download_latest_install_script() {
    local latest_path
    latest_path="${WORK_DIR}/install.sh.latest"
    if curl -fsSL "${INSTALL_SCRIPT_URL}" -o "${latest_path}"; then
        if grep -q 'GH_REPO="nicelic/kwor"' "${latest_path}"; then
            chmod 755 "${latest_path}" || true
            STAGED_INSTALL_SCRIPT_PATH="${latest_path}"
            return
        fi
        rm -f "${latest_path}"
        log_warn "Downloaded install.sh failed validation; using packaged install.sh if available"
        return
    fi
    rm -f "${latest_path}" 2>/dev/null || true
    log_warn "Failed to download latest install.sh; using packaged install.sh if available"
}

systemd_escape_unit_value() {
    local value="$1"
    value="${value//\\/\\\\}"
    value="${value//\"/\\\"}"
    value="${value//%/%%}"
    value="${value//$'\t'/\\x09}"
    value="${value//$'\r'/\\x0d}"
    value="${value//$'\n'/\\x0a}"
    value="${value// /\\x20}"
    printf '%s' "${value}"
}

write_staged_service_file() {
    STAGED_SERVICE_FILE_PATH="${WORK_DIR}/kwor.service"
    cat > "${STAGED_SERVICE_FILE_PATH}" <<EOF
[Unit]
Description=kwor Service
After=network.target nss-lookup.target

[Service]
Type=simple
WorkingDirectory=$(systemd_escape_unit_value "${INSTALL_DIR}")
ExecStart=$(systemd_escape_unit_value "${TARGET_BIN_PATH}")
Restart=on-failure
RestartSec=5s
LimitNOFILE=infinity

[Install]
WantedBy=multi-user.target
EOF
    chmod 644 "${STAGED_SERVICE_FILE_PATH}" || true
}

install_support_files() {
    if [[ -n "${STAGED_INSTALL_SCRIPT_PATH}" && -f "${STAGED_INSTALL_SCRIPT_PATH}" ]]; then
        if cp -f "${STAGED_INSTALL_SCRIPT_PATH}" "${TARGET_INSTALL_SCRIPT_PATH}"; then
            chmod 755 "${TARGET_INSTALL_SCRIPT_PATH}" || true
            rm -f "${INSTALL_DIR}/install.sh"
        else
            log_warn "Failed to place runtime install.sh into ${TARGET_INSTALL_SCRIPT_PATH}; keeping legacy copy if present"
        fi
    fi
    if [[ -n "${STAGED_SERVICE_FILE_PATH}" && -f "${STAGED_SERVICE_FILE_PATH}" ]]; then
        if cp -f "${STAGED_SERVICE_FILE_PATH}" "${TARGET_SERVICE_COPY_PATH}"; then
            chmod 644 "${TARGET_SERVICE_COPY_PATH}" || true
            rm -f "${INSTALL_DIR}/kwor.service"
        else
            log_warn "Failed to place runtime kwor.service into ${TARGET_SERVICE_COPY_PATH}; keeping legacy copy if present"
        fi
    fi
}

stop_existing_instance() {
    if [[ -z "${STOP_BIN_PATH}" || ! -x "${STOP_BIN_PATH}" ]]; then
        log_info "No existing running/service-managed installation detected; proceeding as fresh install"
        return
    fi
    log_info "Stopping existing instance using: ${STOP_BIN_PATH} stop"
    if ! "${STOP_BIN_PATH}" stop; then
        log_warn "Failed to stop via ${STOP_BIN_PATH} stop; falling back to systemctl/pkill"
    fi
    if command -v systemctl >/dev/null 2>&1; then
        systemctl stop "${SERVICE_NAME}" >/dev/null 2>&1 || true
    fi
    local name
    for name in kwor kwor_amd64 kwor_arm64; do
        pkill -TERM -x "${name}" >/dev/null 2>&1 || true
    done
    sleep 2
    for name in kwor kwor_amd64 kwor_arm64; do
        if pgrep -x "${name}" >/dev/null 2>&1; then
            pkill -KILL -x "${name}" >/dev/null 2>&1 || true
        fi
    done
}

install_binary() {
    if [[ -f "${TARGET_BIN_PATH}" ]]; then
        cp -f "${TARGET_BIN_PATH}" "${BACKUP_BIN_PATH}"
    fi
    cp -f "${STAGED_BIN_PATH}" "${TARGET_BIN_PATH}"
    chmod 755 "${TARGET_BIN_PATH}"

    case "${TARGET_BIN_NAME}" in
        kwor)
            rm -f "${INSTALL_DIR}/kwor_amd64" "${INSTALL_DIR}/kwor_arm64"
            ;;
        kwor_amd64 | kwor_arm64)
            rm -f "${INSTALL_DIR}/kwor"
            ;;
    esac
}

rollback_and_restart_previous() {
    if [[ ! -f "${BACKUP_BIN_PATH}" ]]; then
        return 1
    fi
    log_warn "New version failed to start, rolling back previous binary"
    cp -f "${BACKUP_BIN_PATH}" "${TARGET_BIN_PATH}"
    chmod 755 "${TARGET_BIN_PATH}"
    if "${TARGET_BIN_PATH}" start; then
        log_warn "Rollback start succeeded; previous version is running again"
        return 0
    fi
    return 1
}

start_with_repaired_systemd() {
    if ! command -v systemctl >/dev/null 2>&1; then
        return 1
    fi
    if [[ -z "${STAGED_SERVICE_FILE_PATH}" || ! -f "${STAGED_SERVICE_FILE_PATH}" ]]; then
        return 1
    fi
    mkdir -p /etc/systemd/system
    cp -f "${STAGED_SERVICE_FILE_PATH}" "/etc/systemd/system/${SERVICE_NAME}.service"
    chmod 644 "/etc/systemd/system/${SERVICE_NAME}.service" || true
    systemctl daemon-reload
    systemctl reset-failed "${SERVICE_NAME}" >/dev/null 2>&1 || true
    systemctl enable "${SERVICE_NAME}"
    systemctl restart "${SERVICE_NAME}"

    local i
    for i in $(seq 1 40); do
        if systemctl is-active --quiet "${SERVICE_NAME}"; then
            return 0
        fi
        sleep 0.3
    done
    return 1
}

repair_systemd_after_target_start() {
    if ! command -v systemctl >/dev/null 2>&1; then
        return
    fi
    if [[ -z "${STAGED_SERVICE_FILE_PATH}" || ! -f "${STAGED_SERVICE_FILE_PATH}" ]]; then
        return
    fi
    cp -f "${STAGED_SERVICE_FILE_PATH}" "/etc/systemd/system/${SERVICE_NAME}.service" || return
    chmod 644 "/etc/systemd/system/${SERVICE_NAME}.service" || true
    systemctl daemon-reload >/dev/null 2>&1 || true
    systemctl reset-failed "${SERVICE_NAME}" >/dev/null 2>&1 || true
    systemctl enable "${SERVICE_NAME}" >/dev/null 2>&1 || true
}

start_target_instance() {
    log_info "Starting ${TARGET_BIN_PATH} start"
    if [[ "${INSTALL_SOURCE}" != "default" ]] && start_with_repaired_systemd; then
        rm -f "${BACKUP_BIN_PATH}"
        return
    fi

    if "${TARGET_BIN_PATH}" start; then
        repair_systemd_after_target_start
        rm -f "${BACKUP_BIN_PATH}"
        return
    fi

    if rollback_and_restart_previous; then
        log_error "Upgrade aborted because the new version failed to start; previous version has been restored"
    else
        log_error "Upgrade failed and automatic rollback did not succeed"
    fi
    exit 1
}

print_summary() {
    echo
    log_info "kwor ${TARGET_VERSION} installation finished"
    echo -e "Install directory: ${green}${INSTALL_DIR}${plain}"
    echo -e "Detected install source: ${green}${INSTALL_SOURCE}${plain}"
    echo -e "Binary path: ${green}${TARGET_BIN_PATH}${plain}"
    echo -e "Run status check with: ${green}systemctl status kwor${plain}"
}

main() {
    require_root
    detect_os
    detect_arch
    install_base_deps
    resolve_target_version "${1:-}"
    resolve_install_dir
    log_info "Resolved install directory (${INSTALL_SOURCE}): ${INSTALL_DIR}"
    download_release_archive
    extract_release_archive
    prepare_install_dir
    download_latest_install_script
    write_staged_service_file
    stop_existing_instance
    install_binary
    install_support_files
    start_target_instance
    print_summary
}

main "${1:-}"
