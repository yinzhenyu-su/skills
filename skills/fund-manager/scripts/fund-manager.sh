#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BOOTSTRAP_SCRIPT="$SCRIPT_DIR/bootstrap-fund-manager.sh"

if [[ ! -f "$BOOTSTRAP_SCRIPT" ]]; then
  echo "[fund-manager-wrapper] ERROR: bootstrap script is missing: $BOOTSTRAP_SCRIPT" >&2
  exit 1
fi

CORE_BIN="$(bash "$BOOTSTRAP_SCRIPT")"
exec "$CORE_BIN" "$@"
