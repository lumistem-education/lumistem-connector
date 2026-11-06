#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

SERVER_DEFAULT="http://localhost:8080"
SERVER="${SERVER:-$SERVER_DEFAULT}"
BOARD_FQBN="${BOARD_FQBN:-esp32:esp32:XIAO_ESP32C3}"

CODE="void setup() { } void loop() { }"

check_cmd() {
  command -v "$1" >/dev/null 2>&1 || { echo "required: $1" >&2; return 1; }
}

if ! check_cmd curl || ! check_cmd jq; then
  echo "Please install curl and jq." >&2
  exit 1
fi

WS_CLIENT=""
if check_cmd websocat; then
  WS_CLIENT="websocat"
elif check_cmd wscat; then
  WS_CLIENT="wscat"
else
  echo "Install websocat (recommended) or wscat to use websocket client." >&2
  exit 1
fi

echo "Using server: $SERVER"
echo "Using board FQBN: $BOARD_FQBN"

PAYLOAD=$(jq -n --arg code "$CODE" --arg board "$BOARD_FQBN" '{code: $code, board: $board}')

echo "Creating compile job..."

HTTP_STATUS=$(curl -sS -o "$ROOT_DIR/compile_job_response.json" -w '%{http_code}' \
  -X POST "$SERVER/api/jobs/compile" \
  -H 'Content-Type: application/json' \
  -d "$PAYLOAD") || {
    echo "Không kết nối được tới $SERVER — server có đang chạy không?" >&2
    exit 1
  }

RESP=$(cat "$ROOT_DIR/compile_job_response.json")

rm "$ROOT_DIR/compile_job_response.json"

if [ "$HTTP_STATUS" -ge 400 ]; then
  echo "Server trả lỗi HTTP $HTTP_STATUS:" >&2
  echo "$RESP" >&2
  exit 1
fi

if ! echo "$RESP" | jq -e . >/dev/null 2>&1; then
  echo "Response không phải JSON hợp lệ (HTTP $HTTP_STATUS):" >&2
  echo "$RESP" >&2
  exit 1
fi

JOB_ID=$(echo "$RESP" | jq -r '.jobId // empty')
WS_TOKEN=$(echo "$RESP" | jq -r '.wsToken // empty')

if [ -z "$JOB_ID" ] || [ -z "$WS_TOKEN" ]; then
  echo "Failed to create job. Response:" >&2
  echo "$RESP" >&2
  exit 1
fi

echo "Job created: $JOB_ID"

if echo "$SERVER" | grep -q '^https://'; then
  WS_SCHEME="wss"
else
  WS_SCHEME="ws"
fi

WS_URL="$WS_SCHEME://$(echo "$SERVER" | sed -E 's#^https?://##')/ws/compile/$JOB_ID?token=$WS_TOKEN"

echo "Connecting to websocket: $WS_URL"

if [ "$WS_CLIENT" = "websocat" ]; then
  exec websocat -q "$WS_URL"
else
  exec wscat -c "$WS_URL"
fi
