#!/usr/bin/env bash
set -euo pipefail

log() {
  >&2 echo "[fund-manager-bootstrap] $*"
}

die() {
  >&2 echo "[fund-manager-bootstrap] ERROR: $*"
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

  local os
  local arch
  os="$(uname -s)"
  arch="$(uname -m)"

  case "$os/$arch" in
    Darwin/arm64|Darwin/aarch64)
      echo "aarch64-apple-darwin"
      ;;
    Darwin/x86_64)
      echo "x86_64-apple-darwin"
      ;;
    Linux/x86_64)
      echo "x86_64-unknown-linux-gnu"
      ;;
    Linux/aarch64|Linux/arm64)
      echo "aarch64-unknown-linux-gnu"
      ;;
    MINGW64_NT-*/x86_64|MSYS_NT-*/x86_64|CYGWIN_NT-*/x86_64)
      echo "x86_64-pc-windows-msvc"
      ;;
    *)
      die "unsupported platform: $os/$arch (supported: aarch64-apple-darwin, x86_64-apple-darwin, x86_64-unknown-linux-gnu, aarch64-unknown-linux-gnu, x86_64-pc-windows-msvc)"
      ;;
  esac
}

build_default_url() {
  local version="$1"
  local target="$2"
  local ext="$3"
  local base="${FUND_MANAGER_GITLAB_BASE_URL:-https://gitlab.com}"
  local project_id="${FUND_MANAGER_GITLAB_PROJECT_ID:-}"
  local package="${FUND_MANAGER_GITLAB_PACKAGE:-fund-manager}"
  local asset="fund-manager-v${version}-${target}.${ext}"

  [[ -n "$project_id" ]] || die "FUND_MANAGER_GITLAB_PROJECT_ID is required when FUND_MANAGER_CORE_URL is not set"

  echo "${base%/}/api/v4/projects/${project_id}/packages/generic/${package}/${version}/${asset}"
}

validate_cached_binary() {
  local bin_path="$1"
  local hash_path="${bin_path}.sha256"

  [[ -f "$bin_path" ]] || return 1
  [[ -x "$bin_path" ]] || return 1
  [[ -s "$bin_path" ]] || return 1

  if [[ -f "$hash_path" ]]; then
    local expected
    local current
    expected="$(cat "$hash_path")"
    current="$(sha256_file "$bin_path")"
    [[ "$expected" == "$current" ]] || return 1
  fi

  return 0
}

download_file() {
  local url="$1"
  local out="$2"
  local token="${FUND_MANAGER_GITLAB_TOKEN:-}"

  if [[ -n "$token" ]]; then
    curl -fL --retry 3 --retry-delay 1 -H "PRIVATE-TOKEN: ${token}" "$url" -o "$out"
  else
    curl -fL --retry 3 --retry-delay 1 "$url" -o "$out"
  fi
}

extract_archive() {
  local archive="$1"
  local ext="$2"
  local dst="$3"

  case "$ext" in
    tar.gz)
      require_cmd tar
      tar -xzf "$archive" -C "$dst"
      ;;
    zip)
      require_cmd unzip
      unzip -q "$archive" -d "$dst"
      ;;
    *)
      die "unsupported archive extension: $ext"
      ;;
  esac
}

install_binary_from_archive() {
  local archive="$1"
  local ext="$2"
  local binary_name="$3"
  local install_path="$4"

  local temp_dir
  temp_dir="$(mktemp -d)"

  extract_archive "$archive" "$ext" "$temp_dir"

  local found
  found="$(find "$temp_dir" -type f -name "$binary_name" | head -n 1 || true)"
  [[ -n "$found" ]] || die "binary '$binary_name' not found in archive"

  mkdir -p "$(dirname "$install_path")"
  cp "$found" "$install_path"
  chmod +x "$install_path"

  sha256_file "$install_path" > "${install_path}.sha256"
  rm -rf "$temp_dir"
}

main() {
  require_cmd curl

  local target
  local version
  local ext
  local binary_name
  local cache_root
  local install_dir
  local install_bin
  local url

  target="$(detect_target)"
  version="${FUND_MANAGER_VERSION:-latest}"

  if [[ "$target" == *windows* ]]; then
    ext="zip"
    binary_name="fund-manager.exe"
  else
    ext="tar.gz"
    binary_name="fund-manager"
  fi

  cache_root="${FUND_MANAGER_CACHE_DIR:-${XDG_CACHE_HOME:-$HOME/.cache}/fund-manager/bin}"
  install_dir="${cache_root}/${version}/${target}"
  install_bin="${install_dir}/${binary_name}"

  if [[ "${FUND_MANAGER_DRY_RUN:-0}" == "1" ]]; then
    if [[ -n "${FUND_MANAGER_CORE_URL:-}" ]]; then
      echo "${FUND_MANAGER_CORE_URL}"
    else
      build_default_url "$version" "$target" "$ext"
    fi
    return 0
  fi

  if validate_cached_binary "$install_bin"; then
    log "using cached binary: $install_bin"
    echo "$install_bin"
    return 0
  fi

  rm -f "$install_bin" "${install_bin}.sha256"

  if [[ -n "${FUND_MANAGER_CORE_URL:-}" ]]; then
    url="${FUND_MANAGER_CORE_URL}"
  else
    url="$(build_default_url "$version" "$target" "$ext")"
  fi

  local archive
  archive="$(mktemp)"

  log "downloading binary archive from: $url"
  download_file "$url" "$archive"

  if [[ -n "${FUND_MANAGER_CORE_SHA256:-}" ]]; then
    local archive_hash
    archive_hash="$(sha256_file "$archive")"
    [[ "$archive_hash" == "$FUND_MANAGER_CORE_SHA256" ]] || die "archive checksum mismatch"
  fi

  install_binary_from_archive "$archive" "$ext" "$binary_name" "$install_bin"
  rm -f "$archive"
  log "installed binary: $install_bin"
  echo "$install_bin"
}

main "$@"
