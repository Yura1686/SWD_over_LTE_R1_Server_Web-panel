#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
SERVER_DIR="$ROOT_DIR/backend/server"

if [[ ! -f "$SERVER_DIR/lte-swd-server" ]]; then
  echo "backend binary not found, building..."
  (cd "$SERVER_DIR" && GOCACHE=/tmp/go-cache go build -o lte-swd-server ./cmd/lte-swd-server)
fi

export HTTP_ADDR="${HTTP_ADDR:-127.0.0.1:8080}"
export HTTPS_ADDR="${HTTPS_ADDR:-}"
export TLS_CERT_FILE="${TLS_CERT_FILE:-}"
export TLS_KEY_FILE="${TLS_KEY_FILE:-}"
export OPERATOR_PASSWORD="${OPERATOR_PASSWORD:-lte_swd_admin}"
export DEVICE_ENROLL_KEY="${DEVICE_ENROLL_KEY:-r1-enroll-key}"
export DATA_FILE="${DATA_FILE:-$SERVER_DIR/data/state.json}"
export STATIC_DIR="${STATIC_DIR:-$ROOT_DIR/web/panel}"
export FLEET_LIMIT="${FLEET_LIMIT:-10}"
export OPERATOR_TOKEN_TTL="${OPERATOR_TOKEN_TTL:-12h}"
export DEVICE_OFFLINE_AFTER="${DEVICE_OFFLINE_AFTER:-90s}"
export MAX_JSON_BYTES="${MAX_JSON_BYTES:-65536}"
export MAX_ARTIFACT_BYTES="${MAX_ARTIFACT_BYTES:-12582912}"
export API_RATE_PER_MINUTE="${API_RATE_PER_MINUTE:-180}"
export LOGIN_RATE_PER_MINUTE="${LOGIN_RATE_PER_MINUTE:-20}"
export LOGIN_BURST="${LOGIN_BURST:-5}"
export TRUST_PROXY_HEADERS="${TRUST_PROXY_HEADERS:-true}"

exec "$SERVER_DIR/lte-swd-server"
