#!/usr/bin/env bash

set -euo pipefail

"$DGRAPH_BIN" version

readonly TEST_PATH="$PWD/_tmp"

readonly DATA_PATH="$TEST_PATH/data"
readonly LOGS_PATH="$TEST_PATH/logs"
readonly DGRAPH_PATH="$TEST_PATH/dgraph"

readonly ENCRYPTION_KEY_PATH="$DGRAPH_PATH/encryption_key_file"
readonly ACL_SECRET_PATH="$DGRAPH_PATH/acl_secret_file"
readonly TLS_PATH="$DGRAPH_PATH/tls"

readonly DATASET_1MILLION_FILE_URL='https://github.com/dgraph-io/benchmarks/blob/master/data/1million.rdf.gz?raw=true'
readonly DATASET_1MILLION_FILE_PATH="$DATA_PATH/1million.rdf.gz"

readonly DATASET_1MILLION_SCHEMA_URL='https://github.com/dgraph-io/benchmarks/blob/master/data/1million.schema?raw=true'
readonly DATASET_1MILLION_SCHEMA_PATH="$DATA_PATH/1million.schema"

source "log.sh"

function dataset::1million::download() {
  if ! [ -f "$DATASET_1MILLION_FILE_PATH" ]; then
    log::debug "Downloading from $DATASET_1MILLION_FILE_URL."
    curl -L "$DATASET_1MILLION_FILE_URL" --output "$DATASET_1MILLION_FILE_PATH"
  fi

  if ! [ -f "$DATASET_1MILLION_SCHEMA_PATH" ]; then
    log::debug "Downloading from $DATASET_1MILLION_SCHEMA_URL."
    curl -L "$DATASET_1MILLION_SCHEMA_URL" --output "$DATASET_1MILLION_SCHEMA_PATH"
  fi
}

function dataset::1million::verify() {
  local count_names_exp=197408
  count_names_got=$(
    curl \
      -SsX POST \
      -H 'Content-Type: application/json' \
      -d '{ "query": "query { test(func: has(name@.)) { count(uid) } }" }' \
      'localhost:8081/query' | jq '.data.test[0].count'
  )

  if [ "$count_names_got" -ne "$count_names_exp" ]; then
    log::error "Could not verify 1million, expected: $count_names_exp, got: $count_names_got"
    return 1
  fi
}

function portkill() {
  local pids
  if pids="$(lsof -nti ":$1")"; then
    echo "$pids" | xargs kill -9
  fi
}

function dgraph::killall() {
  while pkill -x 'dgraph'; do
    log::debug 'Killing running Dgraph instances.'
    sleep 1
  done
}

function dgraph::start_zeros() {
  local -r n="$1"

  for i in $(seq "$n"); do
    log::debug "Starting Zero $i."

    local grpc_port=$((5080 + i))
    local http_port=$((6080 + i))

    for port in "$grpc_port" "$http_port"; do
      portkill "$port"
    done

    local zero_args_default=(--cwd "$DGRAPH_PATH/zero$i" --idx "$i" --port_offset "$i")

    if [ "$i" -ne 1 ]; then
      zero_args_default+=(--peer 'localhost:5081')
    fi

    "$DGRAPH_BIN" zero "${zero_args_default[@]}" "${@:2}" &>"$LOGS_PATH/zero$i" &
    sleep 1
  done
}

function dgraph::start_alphas() {
  local -r n="$1"

  for i in $(seq "$n"); do
    log::debug "Starting Alpha $i."

    local internal_port=$((7080 + i))
    local http_port=$((8080 + i))
    local grpc_port=$((9080 + i))

    for port in "$internal_port" "$http_port" "$grpc_port"; do
      portkill "$port"
    done

    "$DGRAPH_BIN" \
      alpha \
      --cwd "$DGRAPH_PATH/alpha$i" \
      --port_offset "$i" \
      --zero 'localhost:5081' \
      "${@:2}" &>"$LOGS_PATH/alpha$i" &
    sleep 1
  done
}

function dgraph::generate_encryption_key() {
  dd if=/dev/random bs=1 count=32 of="$ENCRYPTION_KEY_PATH"
}

function dgraph::generate_acl_secret() {
  dd if=/dev/random bs=1 count=256 of="$ACL_SECRET_PATH"
}

function dgraph::generate_tls() {
  "$DGRAPH_BIN" cert --cwd "$DGRAPH_PATH" --nodes localhost
}

function dgraph::healthcheck_zero() {
  local -r i="$1"
  local -r http_port=$((6080 + i))
  local response

  while true; do
    response="$(curl -Ss "localhost:$http_port/health")"
    if [ "$response" == "Please retry again, server is not ready to accept requests" ]; then
      log::warn "Zero $i is not ready, retrying in 1s."
      sleep 1
    else
      break
    fi
  done

  if [ "$response" != "OK" ]; then
    log::error "Zero $i is not healthy."
    echo "$response"
    return 1
  fi

  log::debug "Zero $i is healthy."
}

function dgraph::healthcheck_alpha() {
  local -r i="$1"
  local -r http_port=$((8080 + i))
  local response

  while true; do
    response="$(curl -Ss "localhost:$http_port/health")"
    if [ "$response" == "Please retry again, server is not ready to accept requests" ]; then
      log::warn "Alpha $i is not ready, retrying in 1s."
      sleep 1
    else
      break
    fi
  done

  if [ "$(echo "$response" | jq '.[0].status')" != '"healthy"' ]; then
    log::error "Alpha $i is not healthy."
    echo "$response" | jq || echo "$response"
    return 1
  fi

  log::debug "Alpha $i is healthy."
}

function dgraph::healthcheck_alpha_tls() {
  local -r i="$1"
  local -r http_port=$((8080 + i))
  local response

  while true; do
    response="$(curl --insecure -Ss "https://localhost:$http_port/health")"
    if [ "$response" == "Please retry again, server is not ready to accept requests" ]; then
      log::warn "Alpha $i is not ready, retrying in 1s."
      sleep 1
    else
      break
    fi
  done

  if [ "$(echo "$response" | jq '.[0].status')" != '"healthy"' ]; then
    log::error "Alpha $i is not healthy."
    echo "$response" | jq || echo "$response"
    return 1
  fi

  log::debug "Alpha $i is healthy."
}

function setup() {
  dgraph::killall

  log::debug 'Removing old test files.'

  rm -rf "$LOGS_PATH"
  mkdir -p "$LOGS_PATH"

  rm -rf "$DGRAPH_PATH"
  mkdir -p "$DGRAPH_PATH"

  mkdir -p "$DATA_PATH"
}

function cleanup() {
  dgraph::killall

  log::debug 'Removing old test files.'
  rm -rf "$TEST_PATH"
}

function test::manual_start() {
  local -r n_zeros=2
  local -r n_alphas=4

  dgraph::start_zeros "$n_zeros"
  dgraph::start_alphas "$n_alphas"

  for i in $(seq "$n_zeros"); do
    dgraph::healthcheck_zero "$i"
  done

  sleep 5

  for i in $(seq "$n_alphas"); do
    dgraph::healthcheck_alpha "$i"
  done
}

function test::manual_start_encryption() {
  dgraph::generate_encryption_key

  local -r n_zeros=2
  local -r n_alphas=4

  dgraph::start_zeros "$n_zeros"
  dgraph::start_alphas "$n_alphas" --encryption_key_file "$ENCRYPTION_KEY_PATH"

  for i in $(seq "$n_zeros"); do
    dgraph::healthcheck_zero "$i"
  done

  sleep 5

  for i in $(seq "$n_alphas"); do
    dgraph::healthcheck_alpha "$i"
  done
}

function test::manual_start_acl() {
  dgraph::generate_acl_secret

  local -r n_zeros=2
  local -r n_alphas=4

  dgraph::start_zeros "$n_zeros"
  dgraph::start_alphas "$n_alphas" --acl_secret_file "$ACL_SECRET_PATH"

  for i in $(seq "$n_zeros"); do
    dgraph::healthcheck_zero "$i"
  done

  sleep 5

  for i in $(seq "$n_alphas"); do
    dgraph::healthcheck_alpha "$i"
  done
}

function test::manual_start_tls() {
  dgraph::generate_tls

  local -r n_zeros=2
  local -r n_alphas=4

  dgraph::start_zeros "$n_zeros"
  dgraph::start_alphas "$n_alphas" --tls_cacert "$TLS_PATH"/ca.crt --tls_node_cert "$TLS_PATH"/node.crt --tls_node_key "$TLS_PATH"/node.key

  for i in $(seq "$n_zeros"); do
    dgraph::healthcheck_zero "$i"
  done

  sleep 5

  for i in $(seq "$n_alphas"); do
    dgraph::healthcheck_alpha_tls "$i"
  done
}

function test::manual_start_encryption_acl_tls() {
  dgraph::generate_encryption_key
  dgraph::generate_acl_secret
  dgraph::generate_tls

  local -r n_zeros=2
  local -r n_alphas=4

  dgraph::start_zeros "$n_zeros"
  dgraph::start_alphas "$n_alphas" \
    --acl_secret_file "$ACL_SECRET_PATH" \
    --encryption_key_file "$ENCRYPTION_KEY_PATH" \
    --tls_cacert "$TLS_PATH"/ca.crt --tls_node_cert "$TLS_PATH"/node.crt --tls_node_key "$TLS_PATH"/node.key

  for i in $(seq "$n_zeros"); do
    dgraph::healthcheck_zero "$i"
  done

  sleep 5

  for i in $(seq "$n_alphas"); do
    dgraph::healthcheck_alpha_tls "$i"
  done
}

function test::live_loader() {
  dataset::1million::download

  dgraph::start_zeros 1
  dgraph::start_alphas 2

  sleep 5

  log::debug 'Running live loader.'
  "$DGRAPH_BIN" \
    live \
    --alpha 'localhost:9081' \
    --cwd "$DGRAPH_PATH/live" \
    --files "$DATASET_1MILLION_FILE_PATH" \
    --schema "$DATASET_1MILLION_SCHEMA_PATH" \
    --zero 'localhost:5081' &>"$LOGS_PATH/live"

  dataset::1million::verify
}

function test::bulk_loader() {
  dataset::1million::download

  dgraph::start_zeros 1

  sleep 5

  log::debug 'Running bulk loader.'
  "$DGRAPH_BIN" \
    bulk \
    --cwd "$DGRAPH_PATH/bulk" \
    --files "$DATASET_1MILLION_FILE_PATH" \
    --schema "$DATASET_1MILLION_SCHEMA_PATH" \
    --map_shards 1 \
    --reduce_shards 1 \
    --zero 'localhost:5081' &>"$LOGS_PATH/bulk"

  mkdir -p "$DGRAPH_PATH/alpha1"
  cp -r "$DGRAPH_PATH/bulk/out/0/p" "$DGRAPH_PATH/alpha1"

  dgraph::start_alphas 1
  sleep 5

  dataset::1million::verify
  log::info "Bulk load succeeded."

  log::debug "Exporting data."

  local export_result
  export_result=$(curl -Ss 'localhost:8081/admin/export')

  if [ "$(echo "$export_result" | jq '.code')" != '"Success"' ]; then
    log::error 'Export failed.'
    echo "$export_result" | jq || echo "$export_result"
    return 1
  else
    log::info "Export succeeded."
  fi

  log::debug "Backing up data."

  local -r backup_path="$TEST_PATH/backup"
  rm -rf "$backup_path"
  mkdir -p "$backup_path"

  local backup_result
  backup_result=$(curl -SsX POST -H 'Content-Type: application/json' -d "
    {
      \"query\": \"mutation { backup(input: {destination: \\\"$backup_path\\\"}) { response { message code } } }\"
    }" 'http://localhost:8081/admin')

  if [ "$(echo "$backup_result" | jq '.data.backup.response.code')" != '"Success"' ]; then
    log::error 'Backup failed.'
    echo "$backup_result" | jq || echo "$backup_result"
    return 1
  else
    log::info "Backup succeeded."
  fi

  setup

  dgraph::start_zeros 1

  sleep 5

  log::info "Restoring data."
  "$DGRAPH_BIN" \
    restore \
    --cwd "$DGRAPH_PATH/restore" \
    --location "$backup_path" \
    --postings "$DGRAPH_PATH" \
    --zero 'localhost:5081' &>"$LOGS_PATH/restore"

  mkdir -p "$DGRAPH_PATH/alpha1"
  mv "$DGRAPH_PATH/p1" "$DGRAPH_PATH/alpha1/p"

  dgraph::start_alphas 1
  sleep 5

  dataset::1million::verify
  log::info "Restore succeeded."
}

function dgraph::run_tests() {
  local passed=0
  local failed=0

  for test in $(compgen -A function test::); do
    log::info "$test starting."

    setup
    if "$test"; then
      log::info "$test succeeded."
      ((passed += 1))
    else
      log::error "$test failed."
      ((failed += 1))

      if [ "${EXIT_ON_FAILURE:-0}" -eq 1 ]; then
        return 1
      fi
    fi
  done

  local -r summary="$passed tests passed, $failed failed."
  if [ "$failed" -ne 0 ]; then
    log::error "$summary"
  else
    log::info "$summary"
  fi
}

function main() {
  cleanup
  dgraph::run_tests
  cleanup
}

main "$@"
