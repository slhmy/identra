#!/usr/bin/env bash
set -euo pipefail

smoke_dir="$(mktemp -d)"
server_pid=""
cleanup() {
  if [ -n "$server_pid" ]; then
    kill "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
  fi
  rm -rf "$smoke_dir"
}
trap cleanup EXIT

export PERSISTENCE_TYPE=sqlite
export PERSISTENCE_SQLITE_PATH="$smoke_dir/identra.db"
export AUTH_RSA_PRIVATE_KEY_FILE="$smoke_dir/signing-key.pem"
export REDIS_URLS=localhost:6379
export LOG_LEVEL=error

go build -o "$smoke_dir/identra" ./cmd/identra
bootstrap_json="$($smoke_dir/identra bootstrap service-account --name smoke-admin --scope identra.admin --output json)"
export IDENTRA_CLIENT_ID="$(jq -r .client_id <<<"$bootstrap_json")"
export IDENTRA_CLIENT_SECRET="$(jq -r .client_secret <<<"$bootstrap_json")"

start_server() {
  "$smoke_dir/identra" serve >"$smoke_dir/server.log" 2>&1 &
  server_pid=$!
}

stop_server() {
  kill "$server_pid"
  wait "$server_pid"
  server_pid=""
}

start_server
token_json=""
for _ in $(seq 1 30); do
  if token_json="$($smoke_dir/identra token service 2>/dev/null)"; then break; fi
  sleep 1
done
service_token="$(jq -r .token.value <<<"$token_json")"
test -n "$service_token" && test "$service_token" != null
IDENTRA_SERVICE_TOKEN="$service_token" "$smoke_dir/identra" service-account list >/dev/null
stop_server

# The database and signing key are reused. A token issued before the restart
# must still authorize a management call after migrations and startup complete.
start_server
for _ in $(seq 1 30); do
  if IDENTRA_SERVICE_TOKEN="$service_token" "$smoke_dir/identra" service-account list >/dev/null 2>&1; then break; fi
  sleep 1
done
IDENTRA_SERVICE_TOKEN="$service_token" "$smoke_dir/identra" service-account list >/dev/null
"$smoke_dir/identra" server-info >/dev/null
