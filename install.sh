#!/bin/bash
# kwor-owner:v1 resource=runtime-install-script

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
PACKAGE_MANAGER=""
SERVICE_BIN_PATH=""
SERVICE_FILE_PATH=""
RUNTIME_INSTALL_DIR=""
RUNTIME_BIN_PATH=""
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
PACKAGE_CACHE_REFRESHED=0
CURRENT_STAGE=""
LAST_COMMAND_OUTPUT=""

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

resolve_package_manager() {
    case "${RELEASE}" in
        centos | almalinux | rocky | oracle)
            PACKAGE_MANAGER="yum"
            ;;
        fedora)
            if command -v dnf >/dev/null 2>&1; then
                PACKAGE_MANAGER="dnf"
            else
                PACKAGE_MANAGER="yum"
            fi
            ;;
        arch | manjaro | parch)
            PACKAGE_MANAGER="pacman"
            ;;
        opensuse-tumbleweed | opensuse* | sles | suse)
            PACKAGE_MANAGER="zypper"
            ;;
        alpine)
            PACKAGE_MANAGER="apk"
            ;;
        *)
            if command -v apt-get >/dev/null 2>&1; then
                PACKAGE_MANAGER="apt-get"
            elif command -v apt >/dev/null 2>&1; then
                PACKAGE_MANAGER="apt"
            fi
            ;;
    esac

    if [[ -z "${PACKAGE_MANAGER}" ]]; then
        log_warn "Failed to detect package manager from OS; later dependency installation may be unavailable"
        return
    fi

    log_info "Detected package manager: ${PACKAGE_MANAGER}"
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

refresh_package_cache() {
    if [[ "${PACKAGE_CACHE_REFRESHED}" -eq 1 ]]; then
        return 0
    fi

    if [[ -z "${PACKAGE_MANAGER}" ]]; then
        log_warn "Package manager unavailable; cannot refresh package cache"
        return 1
    fi

    log_info "Refreshing package cache with ${PACKAGE_MANAGER} (will not upgrade installed system packages)"
    case "${PACKAGE_MANAGER}" in
        apt-get)
            apt-get update
            ;;
        apt)
            apt update
            ;;
        yum)
            yum makecache
            ;;
        dnf)
            dnf makecache
            ;;
        pacman)
            pacman -Sy --noconfirm
            ;;
        zypper)
            zypper refresh
            ;;
        apk)
            apk update
            ;;
        *)
            log_warn "Unsupported package manager: ${PACKAGE_MANAGER}"
            return 1
            ;;
    esac

    PACKAGE_CACHE_REFRESHED=1
    return 0
}

install_packages() {
    if [[ $# -eq 0 ]]; then
        return 0
    fi
    if [[ -z "${PACKAGE_MANAGER}" ]]; then
        log_warn "Package manager unavailable; cannot install packages: $*"
        return 1
    fi

    case "${PACKAGE_MANAGER}" in
        apt-get)
            apt-get install -y "$@"
            ;;
        apt)
            apt install -y "$@"
            ;;
        yum)
            yum install -y -q "$@"
            ;;
        dnf)
            dnf install -y -q "$@"
            ;;
        pacman)
            pacman -S --needed --noconfirm "$@"
            ;;
        zypper)
            zypper --non-interactive install "$@"
            ;;
        apk)
            apk add --no-cache "$@"
            ;;
        *)
            log_warn "Unsupported package manager: ${PACKAGE_MANAGER}"
            return 1
            ;;
    esac
}

confirm_action() {
    local prompt="${1:-Proceed?}"
    local answer
    while true; do
        read -r -p "${prompt} [y/N]: " answer
        case "${answer}" in
            [Yy] | [Yy][Ee][Ss])
                return 0
                ;;
            [Nn] | [Nn][Oo] | "")
                return 1
                ;;
            *)
                echo "Please answer yes or no."
                ;;
        esac
    done
}

reset_last_command_output() {
    LAST_COMMAND_OUTPUT=""
}

capture_command_output() {
    local output_path=""
    local output_text=""
    local status=0

    output_path="$(mktemp /tmp/kwor-cmd.XXXXXX)"
    if "$@" >"${output_path}" 2>&1; then
        status=0
    else
        status=$?
    fi

    output_text="$(cat "${output_path}" 2>/dev/null || true)"
    LAST_COMMAND_OUTPUT="${output_text}"
    rm -f "${output_path}" 2>/dev/null || true

    if [[ -n "${output_text}" ]]; then
        if [[ "${status}" -eq 0 ]]; then
            printf '%s\n' "${output_text}"
        else
            printf '%s\n' "${output_text}" >&2
        fi
    fi

    return "${status}"
}

run_target_start_command() {
    # Preserve first-run prompts and other interactive output when the installer
    # is attached to a terminal. Buffer output only for non-interactive sessions.
    if [[ -t 0 && -t 1 ]]; then
        reset_last_command_output
        "${TARGET_BIN_PATH}" start
        return $?
    fi

    capture_command_output "${TARGET_BIN_PATH}" start
}

normalize_missing_dependency_name() {
    case "$1" in
        systemctl | systemd-analyze | systemd-run)
            echo "systemctl"
            ;;
        curl | wget | tar)
            echo "$1"
            ;;
        *)
            echo ""
            ;;
    esac
}

infer_missing_dependency_from_output() {
    local text="$1"
    local raw_dep=""

    if [[ -z "${text}" ]]; then
        echo ""
        return
    fi

    if [[ "${text}" =~ exec:\ \"([^\"]+)\":\ executable\ file\ not\ found ]]; then
        raw_dep="${BASH_REMATCH[1]}"
    elif [[ "${text}" =~ ([[:alnum:]_./+-]+):[[:space:]]+command[[:space:]]+not[[:space:]]+found ]]; then
        raw_dep="${BASH_REMATCH[1]}"
        raw_dep="${raw_dep##*/}"
    elif [[ "${text}" =~ fork/exec[[:space:]]+([^[:space:]]+):[[:space:]]+no[[:space:]]+such[[:space:]]+file[[:space:]]+or[[:space:]]+directory ]]; then
        raw_dep="${BASH_REMATCH[1]}"
        raw_dep="${raw_dep##*/}"
    fi

    normalize_missing_dependency_name "${raw_dep}"
}

package_candidates_for_dependency() {
    case "$1" in
        curl)
            echo "curl"
            ;;
        wget)
            echo "wget"
            ;;
        tar)
            echo "tar"
            ;;
        systemctl)
            case "${PACKAGE_MANAGER}" in
                apt-get | apt)
                    echo "systemd"
                    ;;
                yum | dnf)
                    echo "systemd"
                    ;;
                pacman)
                    echo "systemd"
                    ;;
                zypper)
                    echo "systemd"
                    ;;
                apk)
                    echo ""
                    ;;
                *)
                    echo ""
                    ;;
            esac
            ;;
        *)
            echo "$1"
            ;;
    esac
}

install_dependency_interactive() {
    local dep="$1"
    local packages
    packages="$(package_candidates_for_dependency "${dep}")"
    if [[ -z "${packages}" ]]; then
        log_warn "Cannot map dependency '${dep}' to installable package names automatically"
        return 1
    fi

    log_warn "Detected missing dependency: ${dep}"
    if ! confirm_action "Install dependency '${dep}' now?"; then
        log_warn "User declined installing dependency '${dep}'"
        return 1
    fi

    if ! refresh_package_cache; then
        log_warn "Failed to refresh package cache; cannot install dependency '${dep}'"
        return 1
    fi

    if install_packages ${packages}; then
        return 0
    fi

    log_warn "Failed to install dependency '${dep}'"
    return 1
}

install_fallback_base_deps() {
    local deps=()
    local extra_dep=""
    extra_dep="$(dependency_for_failed_stage "${CURRENT_STAGE}")"
    case "${PACKAGE_MANAGER}" in
        zypper)
            deps=(wget curl tar timezone)
            ;;
        *)
            deps=(wget curl tar)
            ;;
    esac

    if [[ -n "${extra_dep}" ]]; then
        local extra_packages
        extra_packages="$(package_candidates_for_dependency "${extra_dep}")"
        if [[ -n "${extra_packages}" ]]; then
            # shellcheck disable=SC2206
            local extra_array=(${extra_packages})
            deps+=("${extra_array[@]}")
        fi
    fi

    log_warn "Direct install failed, and no more single missing dependency could be resolved automatically"
    log_warn "Entering final fallback dependency installation: refresh package cache, install minimal installer dependencies, then retry kwor installation"
    if ! refresh_package_cache; then
        log_error "Failed to refresh package cache for fallback dependency installation"
        return 1
    fi
    install_packages "${deps[@]}"
}

fetch_text_via_available_tool() {
    local url="$1"
    local output_path="$2"

    if command -v curl >/dev/null 2>&1; then
        curl -fsSL "${url}" -o "${output_path}"
        return $?
    fi
    if command -v wget >/dev/null 2>&1; then
        wget -qO "${output_path}" "${url}"
        return $?
    fi
    return 127
}

fetch_release_version_latest() {
    local meta_path tag
    mkdir -p "${WORK_DIR}"
    meta_path="${WORK_DIR}/release-latest.json"
    if ! fetch_text_via_available_tool "https://api.github.com/repos/${GH_REPO}/releases/latest" "${meta_path}"; then
        return 1
    fi
    tag="$(grep '"tag_name":' "${meta_path}" | sed -E 's/.*"([^"]+)".*/\1/' | head -n 1)"
    if [[ -z "${tag}" ]]; then
        return 1
    fi
    echo "${tag}"
    return 0
}

download_file_with_fallback() {
    local url="$1"
    local output_path="$2"

    if command -v wget >/dev/null 2>&1; then
        wget -q --show-progress --no-check-certificate -O "${output_path}" "${url}" && return 0
    fi
    if command -v curl >/dev/null 2>&1; then
        curl -fL --progress-bar "${url}" -o "${output_path}" && return 0
    fi
    return 127
}

extract_archive_with_available_tool() {
    local archive_path="$1"
    local target_dir="$2"
    tar -xzf "${archive_path}" -C "${target_dir}"
}

dependency_for_failed_stage() {
    local inferred_dep=""
    inferred_dep="$(infer_missing_dependency_from_output "${LAST_COMMAND_OUTPUT}")"
    if [[ -n "${inferred_dep}" ]]; then
        echo "${inferred_dep}"
        return
    fi

    case "$1" in
        resolve_target_version)
            if ! command -v curl >/dev/null 2>&1 && ! command -v wget >/dev/null 2>&1; then
                echo "curl"
                return
            fi
            ;;
        download_release_archive)
            if ! command -v wget >/dev/null 2>&1 && ! command -v curl >/dev/null 2>&1; then
                echo "wget"
                return
            fi
            ;;
        extract_release_archive)
            if ! command -v tar >/dev/null 2>&1; then
                echo "tar"
                return
            fi
            ;;
        start_target_instance)
            if ! command -v systemctl >/dev/null 2>&1; then
                echo "systemctl"
                return
            fi
            ;;
    esac
    echo ""
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
        TARGET_VERSION="$(fetch_release_version_latest || true)"
        if [[ -z "${TARGET_VERSION}" ]]; then
            log_error "Failed to fetch latest kwor release version from GitHub"
            return 1
        fi
        log_info "Using latest release: ${TARGET_VERSION}"
    else
        TARGET_VERSION="$(normalize_version_tag "$1")"
        log_info "Using specified release: ${TARGET_VERSION}"
    fi
    return 0
}

normalize_binary_path() {
    local path="${1:-}"
    path="${path% (deleted)}"
    if [[ -z "${path}" ]]; then
        echo ""
        return
    fi
    if [[ -e "${path}" || -L "${path}" ]]; then
        readlink -f "${path}" 2>/dev/null || echo "${path}"
        return
    fi
    echo "${path}"
}

is_supported_panel_binary_path() {
    local path="${1:-}"
    [[ "${path}" == /* ]] || return 1
    case "$(basename "${path}")" in
        kwor | kwor_amd64 | kwor_arm64) return 0 ;;
        *) return 1 ;;
    esac
}

path_has_kwor_install_evidence() {
    local install_dir="${1:-}"
    local candidate
    for candidate in \
        "${install_dir}/${RUNTIME_SUPPORT_DIR_NAME}/install.sh" \
        "${install_dir}/install.sh"
    do
        if [[ -f "${candidate}" ]] && grep -Eq 'kwor-owner:v1 resource=runtime-install-script|GH_REPO="nicelic/kwor"' "${candidate}" 2>/dev/null; then
            return 0
        fi
    done
    candidate="${install_dir}/${RUNTIME_SUPPORT_DIR_NAME}/kwor.service"
    [[ -f "${candidate}" ]] && grep -Fq 'kwor-owner:v1 resource=panel-systemd' "${candidate}" 2>/dev/null
}

select_existing_panel_binary() {
    local install_dir="${1:-}"
    local candidate
    for candidate in \
        "${install_dir}/kwor" \
        "${install_dir}/kwor_amd64" \
        "${install_dir}/kwor_arm64"
    do
        if [[ -f "${candidate}" ]]; then
            normalize_binary_path "${candidate}"
            return
        fi
    done
    echo ""
}

find_service_file() {
    local candidate
    for candidate in \
        "/etc/systemd/system/${SERVICE_NAME}.service" \
        "/run/systemd/system/${SERVICE_NAME}.service" \
        "/usr/local/lib/systemd/system/${SERVICE_NAME}.service" \
        "/usr/lib/systemd/system/${SERVICE_NAME}.service" \
        "/lib/systemd/system/${SERVICE_NAME}.service"
    do
        if [[ ! -f "${candidate}" ]]; then
            continue
        fi
        if service_file_is_kwor_install "${candidate}"; then
            echo "${candidate}"
            return 0
        fi
        log_error "Refusing to replace unverified systemd service: ${candidate}"
        return 1
    done
    echo ""
    return 0
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

service_file_is_kwor_install() {
    local service_path="${1:-}"
    local binary_path working_dir binary_dir
    binary_path="$(extract_execstart_path "${service_path}")"
    if ! is_supported_panel_binary_path "${binary_path}"; then
        return 1
    fi

    working_dir="$(extract_working_directory "${service_path}")"
    binary_dir="$(dirname "${binary_path}")"
    working_dir="${working_dir%/}"
    binary_dir="${binary_dir%/}"
    if [[ -n "${working_dir}" && "${working_dir}" != "${binary_dir}" ]]; then
        return 1
    fi

    if grep -Fq 'kwor-owner:v1 resource=panel-systemd' "${service_path}" 2>/dev/null; then
        return 0
    fi
    if ! grep -Eq '^Description=kwor Service\r?$' "${service_path}" 2>/dev/null || \
       ! grep -Eq '^Type=simple\r?$' "${service_path}" 2>/dev/null || \
       ! grep -Eq '^Restart=on-failure\r?$' "${service_path}" 2>/dev/null; then
        return 1
    fi
    if [[ "${binary_dir}" == "${DEFAULT_INSTALL_DIR}" || "${binary_dir}" == "/usr/local/kwor" ]]; then
        return 0
    fi
    path_has_kwor_install_evidence "${binary_dir}"
}

resolve_service_bin_path() {
    if ! SERVICE_FILE_PATH="$(find_service_file)"; then
        return 1
    fi
    if [[ -z "${SERVICE_FILE_PATH}" ]]; then
        return 0
    fi

    SERVICE_BIN_PATH="$(extract_execstart_path "${SERVICE_FILE_PATH}")"
    if [[ -n "${SERVICE_BIN_PATH}" ]]; then
        return 0
    fi

    local working_dir
    working_dir="$(extract_working_directory "${SERVICE_FILE_PATH}")"
    if [[ -n "${working_dir}" ]]; then
        if [[ -f "${working_dir}/kwor" ]]; then
            SERVICE_BIN_PATH="$(readlink -f "${working_dir}/kwor" 2>/dev/null || echo "${working_dir}/kwor")"
            return 0
        fi
        if [[ -f "${working_dir}/kwor_amd64" ]]; then
            SERVICE_BIN_PATH="$(readlink -f "${working_dir}/kwor_amd64" 2>/dev/null || echo "${working_dir}/kwor_amd64")"
            return 0
        fi
        if [[ -f "${working_dir}/kwor_arm64" ]]; then
            SERVICE_BIN_PATH="$(readlink -f "${working_dir}/kwor_arm64" 2>/dev/null || echo "${working_dir}/kwor_arm64")"
            return 0
        fi
    fi
    return 0
}

resolve_runtime_install_dir() {
    local script_path script_dir candidate_dir candidate_bin
    RUNTIME_INSTALL_DIR=""
    RUNTIME_BIN_PATH=""

    script_path="${BASH_SOURCE[0]:-}"
    if [[ -f "${script_path}" ]]; then
        script_path="$(readlink -f "${script_path}" 2>/dev/null || echo "${script_path}")"
        script_dir="$(dirname "${script_path}")"
        candidate_dir=""
        if [[ "$(basename "${script_dir}")" == "${RUNTIME_SUPPORT_DIR_NAME}" ]]; then
            candidate_dir="$(dirname "${script_dir}")"
        elif [[ -d "${script_dir}/${RUNTIME_SUPPORT_DIR_NAME}" ]]; then
            candidate_dir="${script_dir}"
        fi
        if [[ -n "${candidate_dir}" ]] && path_has_kwor_install_evidence "${candidate_dir}"; then
            candidate_bin="$(select_existing_panel_binary "${candidate_dir}")"
            RUNTIME_INSTALL_DIR="${candidate_dir}"
            RUNTIME_BIN_PATH="${candidate_bin}"
            return
        fi
    fi

    if path_has_kwor_install_evidence "${DEFAULT_INSTALL_DIR}"; then
        RUNTIME_INSTALL_DIR="${DEFAULT_INSTALL_DIR}"
        RUNTIME_BIN_PATH="$(select_existing_panel_binary "${DEFAULT_INSTALL_DIR}")"
    fi
}

resolve_install_dir() {
    resolve_service_bin_path || return 1
    if [[ -n "${SERVICE_BIN_PATH}" ]]; then
        INSTALL_DIR="$(dirname "${SERVICE_BIN_PATH}")"
        INSTALL_SOURCE="systemd service"
        STOP_BIN_PATH="${SERVICE_BIN_PATH}"
        return
    fi

    resolve_runtime_install_dir
    if [[ -n "${RUNTIME_INSTALL_DIR}" ]]; then
        INSTALL_DIR="${RUNTIME_INSTALL_DIR}"
        INSTALL_SOURCE="runtime install script"
        STOP_BIN_PATH="${RUNTIME_BIN_PATH}"
        return
    fi

    INSTALL_DIR="${DEFAULT_INSTALL_DIR}"
    INSTALL_SOURCE="default"
    STOP_BIN_PATH="$(select_existing_panel_binary "${DEFAULT_INSTALL_DIR}")"
}

download_release_archive() {
    ARCHIVE_PATH="/tmp/kwor-${TARGET_VERSION}-${ARCH}.tar.gz"
    DOWNLOAD_URL="https://github.com/${GH_REPO}/releases/download/${TARGET_VERSION}/kwor-linux-${ARCH}.tar.gz"
    log_info "Downloading ${DOWNLOAD_URL}"
    if ! download_file_with_fallback "${DOWNLOAD_URL}" "${ARCHIVE_PATH}"; then
        log_error "Failed to download release archive: ${DOWNLOAD_URL}"
        return 1
    fi
    return 0
}

extract_release_archive() {
    if ! extract_archive_with_available_tool "${ARCHIVE_PATH}" "${WORK_DIR}"; then
        log_error "Failed to extract archive: ${ARCHIVE_PATH}"
        return 1
    fi

    if [[ -f "${WORK_DIR}/kwor" ]]; then
        STAGED_BIN_PATH="${WORK_DIR}/kwor"
    elif [[ -f "${WORK_DIR}/kwor/kwor" ]]; then
        STAGED_BIN_PATH="${WORK_DIR}/kwor/kwor"
    else
        log_error "Release archive does not contain kwor binary"
        return 1
    fi

    if [[ -f "${WORK_DIR}/install.sh" ]]; then
        STAGED_INSTALL_SCRIPT_PATH="${WORK_DIR}/install.sh"
    elif [[ -f "${WORK_DIR}/kwor/install.sh" ]]; then
        STAGED_INSTALL_SCRIPT_PATH="${WORK_DIR}/kwor/install.sh"
    fi
    return 0
}

prepare_install_dir() {
    mkdir -p "${INSTALL_DIR}"
    TARGET_SUPPORT_DIR="${INSTALL_DIR}/${RUNTIME_SUPPORT_DIR_NAME}"
    mkdir -p "${TARGET_SUPPORT_DIR}"
    if [[ -n "${SERVICE_BIN_PATH}" ]]; then
        TARGET_BIN_NAME="$(basename "${SERVICE_BIN_PATH}")"
    elif [[ -n "${STOP_BIN_PATH}" ]]; then
        TARGET_BIN_NAME="$(basename "${STOP_BIN_PATH}")"
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
    if fetch_text_via_available_tool "${INSTALL_SCRIPT_URL}" "${latest_path}"; then
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

perform_install_attempt() {
    WORK_DIR="$(mktemp -d /tmp/kwor-install.XXXXXX)"
    ARCHIVE_PATH=""
    STAGED_BIN_PATH=""
    STAGED_INSTALL_SCRIPT_PATH=""
    STAGED_SERVICE_FILE_PATH=""
    INSTALL_DIR=""
    INSTALL_SOURCE=""
    SERVICE_BIN_PATH=""
    SERVICE_FILE_PATH=""
    RUNTIME_INSTALL_DIR=""
    RUNTIME_BIN_PATH=""
    STOP_BIN_PATH=""
    reset_last_command_output
    CURRENT_STAGE="resolve_target_version"
    resolve_target_version "${1:-}" || return 1

    resolve_install_dir || return 1
    log_info "Resolved install directory (${INSTALL_SOURCE}): ${INSTALL_DIR}"

    CURRENT_STAGE="download_release_archive"
    download_release_archive || return 1

    CURRENT_STAGE="extract_release_archive"
    extract_release_archive || return 1

    prepare_install_dir
    download_latest_install_script
    write_staged_service_file
    stop_existing_instance || return 1
    install_binary
    install_support_files
    CURRENT_STAGE="start_target_instance"
    start_target_instance || return 1
    CURRENT_STAGE=""
    return 0
}

run_install_with_strategy() {
    local install_arg="${1:-}"
    local dep=""

    while true; do
        CURRENT_STAGE=""
        if perform_install_attempt "${install_arg}"; then
            return 0
        fi

        dep="$(dependency_for_failed_stage "${CURRENT_STAGE}")"
        if [[ -n "${dep}" ]] && install_dependency_interactive "${dep}"; then
            cleanup
            continue
        fi

        if install_fallback_base_deps; then
            cleanup
            perform_install_attempt "${install_arg}" && return 0
        fi

        return 1
    done
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
# kwor-owner:v1 resource=panel-systemd
[Unit]
Description=kwor Service
After=network.target nss-lookup.target

[Service]
Type=simple
Environment=KWOR_INTERNAL_SYSTEMD=1
ExecCondition=$(systemd_escape_unit_value "${TARGET_BIN_PATH}") lifecycle-guard
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

process_executable_path() {
    local pid="${1:-}"
    local path=""
    if [[ -z "${pid}" || ! -L "/proc/${pid}/exe" ]]; then
        echo ""
        return
    fi
    path="$(readlink "/proc/${pid}/exe" 2>/dev/null || true)"
    normalize_binary_path "${path}"
}

process_start_time() {
    local pid="${1:-}"
    local stat_content suffix
    local -a fields=()
    if [[ -z "${pid}" || ! -r "/proc/${pid}/stat" ]]; then
        return 1
    fi
    stat_content="$(<"/proc/${pid}/stat")"
    if [[ "${stat_content}" != *") "* ]]; then
        return 1
    fi
    suffix="${stat_content##*) }"
    read -r -a fields <<< "${suffix}"
    if [[ "${#fields[@]}" -le 19 || -z "${fields[19]}" ]]; then
        return 1
    fi
    echo "${fields[19]}"
}

process_matches_binary_path() {
    local pid="${1:-}"
    local expected actual
    expected="$(normalize_binary_path "${2:-}")"
    actual="$(process_executable_path "${pid}")"
    [[ -n "${expected}" && -n "${actual}" && "${actual}" == "${expected}" ]]
}

process_identity_matches_binary_path() {
    local pid="${1:-}"
    local expected_start="${2:-}"
    local expected_path="${3:-}"
    local current_start
    current_start="$(process_start_time "${pid}" 2>/dev/null || true)"
    [[ -n "${current_start}" && "${current_start}" == "${expected_start}" ]] || return 1
    process_matches_binary_path "${pid}" "${expected_path}"
}

binary_path_has_running_process() {
    local expected_path="${1:-}"
    local proc_exe pid
    for proc_exe in /proc/[0-9]*/exe; do
        [[ -L "${proc_exe}" ]] || continue
        pid="${proc_exe#/proc/}"
        pid="${pid%/exe}"
        if process_matches_binary_path "${pid}" "${expected_path}"; then
            return 0
        fi
    done
    return 1
}

stop_processes_by_binary_path() {
    local expected_path="${1:-}"
    local normalized_path proc_exe pid start_time identity
    local attempt live
    local -a identities=()
    normalized_path="$(normalize_binary_path "${expected_path}")"
    if [[ -z "${normalized_path}" ]]; then
        return 0
    fi

    for proc_exe in /proc/[0-9]*/exe; do
        [[ -L "${proc_exe}" ]] || continue
        pid="${proc_exe#/proc/}"
        pid="${pid%/exe}"
        if ! process_matches_binary_path "${pid}" "${normalized_path}"; then
            continue
        fi
        start_time="$(process_start_time "${pid}" 2>/dev/null || true)"
        if [[ -z "${start_time}" ]]; then
            log_error "Cannot capture process identity for PID ${pid}; refusing name-based fallback"
            return 1
        fi
        identities+=("${pid}:${start_time}")
    done

    if [[ "${#identities[@]}" -eq 0 ]]; then
        return 0
    fi
    for identity in "${identities[@]}"; do
        pid="${identity%%:*}"
        start_time="${identity#*:}"
        if process_identity_matches_binary_path "${pid}" "${start_time}" "${normalized_path}"; then
            kill -TERM "${pid}" 2>/dev/null || true
        fi
    done

    for ((attempt = 0; attempt < 20; attempt++)); do
        live=0
        for identity in "${identities[@]}"; do
            pid="${identity%%:*}"
            start_time="${identity#*:}"
            if process_identity_matches_binary_path "${pid}" "${start_time}" "${normalized_path}"; then
                live=1
                break
            fi
        done
        [[ "${live}" -eq 0 ]] && return 0
        sleep 0.1
    done

    for identity in "${identities[@]}"; do
        pid="${identity%%:*}"
        start_time="${identity#*:}"
        if process_identity_matches_binary_path "${pid}" "${start_time}" "${normalized_path}"; then
            kill -KILL "${pid}" 2>/dev/null || true
        fi
    done
    for ((attempt = 0; attempt < 20; attempt++)); do
        live=0
        for identity in "${identities[@]}"; do
            pid="${identity%%:*}"
            start_time="${identity#*:}"
            if process_identity_matches_binary_path "${pid}" "${start_time}" "${normalized_path}"; then
                live=1
                break
            fi
        done
        [[ "${live}" -eq 0 ]] && return 0
        sleep 0.1
    done

    log_error "A process executing ${normalized_path} is still running"
    return 1
}

stop_existing_instance() {
    local candidate_path candidate_path_existing existing_path
    local -a stop_paths=()
    if [[ -z "${STOP_BIN_PATH}" ]]; then
        log_info "No existing binary file detected; checking verified install paths for deleted running processes"
    else
        stop_paths+=("${STOP_BIN_PATH}")
    fi

    for candidate_path in \
        "${INSTALL_DIR}/kwor" \
        "${INSTALL_DIR}/kwor_amd64" \
        "${INSTALL_DIR}/kwor_arm64"
    do
        existing_path=0
        for candidate_path_existing in "${stop_paths[@]}"; do
            if [[ "${candidate_path_existing}" == "${candidate_path}" ]]; then
                existing_path=1
                break
            fi
        done
        [[ "${existing_path}" -eq 0 ]] && stop_paths+=("${candidate_path}")
    done

    if [[ -n "${STOP_BIN_PATH}" ]]; then
        log_info "Stopping verified existing instance at: ${STOP_BIN_PATH}"
    fi
    if [[ -n "${SERVICE_FILE_PATH}" ]] && command -v systemctl >/dev/null 2>&1; then
        systemctl stop "${SERVICE_NAME}" >/dev/null 2>&1 || true
    fi
    for candidate_path in "${stop_paths[@]}"; do
        stop_processes_by_binary_path "${candidate_path}" || return 1
        if binary_path_has_running_process "${candidate_path}"; then
            log_error "Existing kwor process remains active at ${candidate_path}"
            return 1
        fi
    done
    if [[ -n "${SERVICE_FILE_PATH}" ]] && command -v systemctl >/dev/null 2>&1 && systemctl is-active --quiet "${SERVICE_NAME}"; then
        log_error "Existing verified kwor systemd service remains active"
        return 1
    fi
    return 0
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
    if "${TARGET_BIN_PATH}" start && wait_for_target_runtime; then
        log_warn "Rollback start succeeded; previous version is running again"
        return 0
    fi
    return 1
}

wait_for_target_runtime() {
    local main_pid=""
    local i
    for i in $(seq 1 40); do
        if command -v systemctl >/dev/null 2>&1 && systemctl is-active --quiet "${SERVICE_NAME}"; then
            main_pid="$(systemctl show "${SERVICE_NAME}" --property=MainPID --value 2>/dev/null || true)"
            if [[ -n "${main_pid}" && "${main_pid}" != "0" ]] && process_matches_binary_path "${main_pid}" "${TARGET_BIN_PATH}"; then
                return 0
            fi
        fi
        if binary_path_has_running_process "${TARGET_BIN_PATH}"; then
            return 0
        fi
        sleep 0.3
    done
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
    if ! capture_command_output systemctl daemon-reload; then
        return 1
    fi
    systemctl reset-failed "${SERVICE_NAME}" >/dev/null 2>&1 || true
    if ! capture_command_output systemctl enable "${SERVICE_NAME}"; then
        return 1
    fi
    if ! capture_command_output systemctl restart "${SERVICE_NAME}"; then
        return 1
    fi

    wait_for_target_runtime
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
    local failure_output=""
    log_info "Starting ${TARGET_BIN_PATH} start"
    if [[ "${INSTALL_SOURCE}" != "default" ]] && start_with_repaired_systemd; then
        rm -f "${BACKUP_BIN_PATH}"
        return 0
    fi
    if [[ -n "${LAST_COMMAND_OUTPUT}" ]]; then
        failure_output="${LAST_COMMAND_OUTPUT}"
    fi

    if run_target_start_command; then
        repair_systemd_after_target_start
        if wait_for_target_runtime; then
            rm -f "${BACKUP_BIN_PATH}"
            return 0
        fi
        LAST_COMMAND_OUTPUT="Target start returned success, but no process is executing ${TARGET_BIN_PATH}"
    fi
    if [[ -n "${LAST_COMMAND_OUTPUT}" ]]; then
        failure_output="${LAST_COMMAND_OUTPUT}"
    fi

    if rollback_and_restart_previous; then
        LAST_COMMAND_OUTPUT="${failure_output}"
        log_error "Upgrade aborted because the new version failed to start; previous version has been restored"
    else
        LAST_COMMAND_OUTPUT="${failure_output}"
        log_error "Upgrade failed and automatic rollback did not succeed"
    fi
    return 1
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
    resolve_package_manager
    detect_arch
    log_info "Trying direct kwor installation first without installing system packages"
    if ! run_install_with_strategy "${1:-}"; then
        log_error "kwor installation failed"
        exit 1
    fi
    print_summary
}

main "${1:-}"
