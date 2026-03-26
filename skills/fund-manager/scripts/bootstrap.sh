#!/usr/bin/env bash
#
# bootstrap.sh - Fund Manager binary bootstrap script
#
# Downloads the fund-manager binary from GitHub Releases and installs it locally.
#
# Environment variables:
#   FUND_MANAGER_VERSION    - Version to install (default: latest)
#   FUND_MANAGER_REPO       - GitHub repository (default: YinZ-510/skills)
#   FUND_MANAGER_CACHE_DIR  - Cache directory (default: ~/.cache/fund-manager/bin)
#   FUND_MANAGER_GITHUB_TOKEN - GitHub token for API access
#   FUND_MANAGER_DRY_RUN    - If set to 1, only print the download URL
#   FUND_MANAGER_CORE_URL  - Override download URL directly
#

set -euo pipefail

SCRIPT_NAME="fund-manager"

log() {
    >&2 echo "[${SCRIPT_NAME}-bootstrap] $*"
}

die() {
    >&2 echo "[${SCRIPT_NAME}-bootstrap] ERROR: $*"
    exit 1
}

require_cmd() {
    command -v "$1" >/dev/null 2>&1 || die "missing required command: $1"
}

sha256_file() {
    local file="$1"
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$file" | awk '{print $1}'
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "$file" | awk '{print $1}'
    else
        die "missing checksum tool: sha256sum or shasum"
    fi
}

detect_target() {
    if [[ -n "${FUND_MANAGER_TARGET_OVERRIDE:-}" ]]; then
        echo "$FUND_MANAGER_TARGET_OVERRIDE"
        return
    fi

    local os arch
    os="$(uname -s)"
    arch="$(uname -m)"

    case "$os/$arch" in
        Darwin/arm64|Darwin/aarch64)  echo "aarch64-apple-darwin" ;;
        Darwin/x86_64)                echo "x86_64-apple-darwin" ;;
        Linux/x86_64)                echo "x86_64-unknown-linux-gnu" ;;
        Linux/aarch64|Linux/arm64)   echo "aarch64-unknown-linux-gnu" ;;
        MINGW*_NT-*/x86_64|MSYS_NT-*/x86_64|CYGWIN_NT-*/x86_64)
            echo "x86_64-pc-windows-gnu" ;;
        MINGW*_NT-*/aarch64|MSYS_NT-*/aarch64|CYGWIN_NT-*/aarch64|ARM64_NT-*/aarch64)
            echo "aarch64-pc-windows-gnullvm" ;;
        *)
            die "unsupported platform: $os/$arch"
            ;;
    esac
}

get_archive_ext() {
    local target="$1"
    case "$target" in
        *windows*) echo "zip" ;;
        *)         echo "tar.gz" ;;
    esac
}

get_binary_name() {
    local target="$1"
    case "$target" in
        *windows*) echo "${SCRIPT_NAME}.exe" ;;
        *)         echo "$SCRIPT_NAME" ;;
    esac
}

build_download_url() {
    local version="$1"
    local target="$2"
    local ext="$3"
    local repo="${FUND_MANAGER_REPO:-YinZ-510/skills}"
    local base="https://github.com"
    local asset="${SCRIPT_NAME}-v${version}-${target}.${ext}"

    if [[ "$version" == "latest" ]]; then
        echo "${base}/${repo}/releases/latest/download/${asset}"
    else
        echo "${base}/${repo}/releases/download/v${version}/${asset}"
    fi
}

get_latest_version() {
    local repo="${FUND_MANAGER_REPO:-YinZ-510/skills}"
    curl -sSL "https://github.com/${repo}/releases/latest" | \
        grep -o 'releases/tag/[^"]*' | head -1 | sed 's/releases\/tag\///'
}

validate_cached_binary() {
    local bin_path="$1"
    local hash_path="${bin_path}.sha256"

    [[ -f "$bin_path" && -x "$bin_path" && -s "$bin_path" ]] || return 1

    if [[ -f "$hash_path" ]]; then
        [[ "$(cat "$hash_path")" == "$(sha256_file "$bin_path")" ]] || return 1
    fi
    return 0
}

download_file() {
    local url="$1"
    local out="$2"
    local token="${FUND_MANAGER_GITHUB_TOKEN:-${GITHUB_TOKEN:-}}"

    if [[ -n "$token" ]]; then
        curl -fL --retry 3 --retry-delay 1 -H "Authorization: Bearer ${token}" "$url" -o "$out"
    else
        curl -fL --retry 3 --retry-delay 1 "$url" -o "$out"
    fi
}

extract_archive() {
    local archive="$1"
    local ext="$2"
    local dst="$3"

    case "$ext" in
        tar.gz) require_cmd tar; tar -xzf "$archive" -C "$dst" ;;
        zip)    require_cmd unzip; unzip -q "$archive" -d "$dst" ;;
        *)      die "unsupported archive extension: $ext" ;;
    esac
}

main() {
    require_cmd curl

    local target version ext binary_name cache_root install_dir install_bin url

    target="$(detect_target)"
    version="${FUND_MANAGER_VERSION:-latest}"
    ext="$(get_archive_ext "$target")"
    binary_name="$(get_binary_name "$target")"

    if [[ "$version" == "latest" ]]; then
        version="$(get_latest_version)"
    fi

    cache_root="${FUND_MANAGER_CACHE_DIR:-${XDG_CACHE_HOME:-$HOME/.cache}/fund-manager/bin}"
    install_dir="${cache_root}/${version}/${target}"
    install_bin="${install_dir}/${binary_name}"

    # Dry run mode
    if [[ "${FUND_MANAGER_DRY_RUN:-0}" == "1" ]]; then
        if [[ -n "${FUND_MANAGER_CORE_URL:-}" ]]; then
            echo "${FUND_MANAGER_CORE_URL}"
        else
            echo "$(build_download_url "$version" "$target" "$ext")"
        fi
        return 0
    fi

    # Use cached binary if valid
    if validate_cached_binary "$install_bin"; then
        log "using cached binary: $install_bin"
        exec "$install_bin" "$@"
    fi

    # Remove stale cache
    rm -f "$install_bin" "${install_bin}.sha256"

    # Download
    if [[ -n "${FUND_MANAGER_CORE_URL:-}" ]]; then
        url="${FUND_MANAGER_CORE_URL}"
    else
        url="$(build_download_url "$version" "$target" "$ext")"
    fi

    local archive
    archive="$(mktemp)"
    log "downloading binary archive from: $url"
    download_file "$url" "$archive"

    # Verify checksum if provided
    if [[ -n "${FUND_MANAGER_CORE_SHA256:-}" ]]; then
        [[ "$(sha256_file "$archive")" == "$FUND_MANAGER_CORE_SHA256" ]] || \
            die "archive checksum mismatch"
    fi

    # Install
    mkdir -p "$install_dir"
    extract_archive "$archive" "$ext" "$install_dir"

    local found
    found="$(find "$install_dir" -type f -name "$binary_name" | head -n 1 || true)"
    [[ -n "$found" ]] || die "binary '$binary_name' not found in archive"

    cp "$found" "$install_bin"
    chmod +x "$install_bin"
    sha256_file "$install_bin" > "${install_bin}.sha256"
    rm -f "$archive"

    log "installed binary: $install_bin"
    # exec the binary with any passed arguments
    exec "$install_bin" "$@"
}

main "$@"
