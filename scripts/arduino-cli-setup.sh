#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

CONFIG_FILE="$PROJECT_ROOT/arduino-cli.yaml"
DATA_DIR="$PROJECT_ROOT/data"
STAGING_DIR="$PROJECT_ROOT/data/staging"
WORKSPACE_DIR="$PROJECT_ROOT/workspace"

cd "$PROJECT_ROOT"

check_cmd() {
    command -v "$1" >/dev/null 2>&1 || { echo "required: $1" >&2; return 1; }
}

if ! command -v arduino-cli >/dev/null 2>&1; then
    exit 1
fi

if ! check_cmd jq; then
    exit 1
fi

mkdir -p \
    "$DATA_DIR" \
    "$STAGING_DIR" \
    "$WORKSPACE_DIR"

if [ ! -f "$CONFIG_FILE" ]; then
    cat > "$CONFIG_FILE" <<EOF
directories:
  data: $DATA_DIR
  downloads: $STAGING_DIR
  user: $WORKSPACE_DIR

board_manager:
  additional_urls:
    - https://raw.githubusercontent.com/espressif/arduino-esp32/gh-pages/package_esp32_index.json
EOF
else
    tmp_file="$(mktemp)"
    awk -v data="$DATA_DIR" -v downloads="$STAGING_DIR" -v user="$WORKSPACE_DIR" '
        /^[[:space:]]*data:[[:space:]]*/   { sub(/data:[[:space:]]*.*/, "data: " data); print; next }
        /^[[:space:]]*downloads:[[:space:]]*/ { sub(/downloads:[[:space:]]*.*/, "downloads: " downloads); print; next }
        /^[[:space:]]*user:[[:space:]]*/   { sub(/user:[[:space:]]*.*/, "user: " user); print; next }
        { print }
    ' "$CONFIG_FILE" > "$tmp_file"
    mv "$tmp_file" "$CONFIG_FILE"
fi

arduino-cli --config-file "$CONFIG_FILE" core update-index

ESP32_INSTALLED=$(arduino-cli --config-file "$CONFIG_FILE" core list --format json | jq -e '
    (if type == "object" then (.platforms // []) else . end)
    | any(.id == "esp32:esp32" and ((.installed_version // "") != ""))
' 2>/dev/null || echo "false")

if [ "$ESP32_INSTALLED" != "true" ]; then
    arduino-cli --config-file "$CONFIG_FILE" core install esp32:esp32
fi

BOARD_INFO=$(arduino-cli --config-file "$CONFIG_FILE" board listall | grep -i "XIAO_ESP32C3" || true)

echo
echo "========================================"
echo "Config : $CONFIG_FILE"
echo "Data   : $DATA_DIR"
echo "Board  : ${BOARD_INFO:-Not found}"
echo "========================================"