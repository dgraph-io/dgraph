#!/usr/bin/env bash
#
# SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
# SPDX-License-Identifier: Apache-2.0
#
# Smoke test for a locally-built dgraph binary. Brings up a single-node cluster
# (one zero + one alpha), exercises schema / mutation / query over the HTTP API,
# verifies the result, then tears everything down.
#
# Usage:
#   contrib/smoke-test.sh
#
# Environment overrides:
#   BIN    path to the dgraph binary    (default: ./dgraph/dgraph, else `dgraph` on PATH)
#   ROOT   working dir for cluster data (default: a fresh mktemp dir, removed on exit)

set -u

# Resolve the dgraph binary: prefer a local build, fall back to PATH.
if [ -z "${BIN:-}" ]; then
  if [ -x "./dgraph/dgraph" ]; then
    BIN="./dgraph/dgraph"
  elif command -v dgraph >/dev/null 2>&1; then
    BIN="dgraph"
  else
    echo "error: could not find dgraph binary (build it with 'make dgraph' or set BIN=...)" >&2
    exit 1
  fi
fi

# Resolve a path-form BIN to an absolute path (we cd into per-node data dirs below);
# leave a bare command name (found on PATH) untouched.
case "$BIN" in
  */*) BIN="$(cd "$(dirname "$BIN")" && pwd)/$(basename "$BIN")" ;;
esac

ROOT="${ROOT:-$(mktemp -d)}"
PASS=0; FAIL=0
ZERO_PID=""; ALPHA_PID=""

say() { printf '\n=== %s ===\n' "$*"; }
ok()  { echo "PASS: $*"; PASS=$((PASS + 1)); }
bad() { echo "FAIL: $*"; FAIL=$((FAIL + 1)); }

cleanup() {
  say "Tearing down"
  [ -n "$ALPHA_PID" ] && kill "$ALPHA_PID" 2>/dev/null
  [ -n "$ZERO_PID" ]  && kill "$ZERO_PID"  2>/dev/null
  wait 2>/dev/null
  rm -rf "$ROOT"
}
trap cleanup EXIT

mkdir -p "$ROOT/zero" "$ROOT/alpha"

say "dgraph version"
"$BIN" version 2>&1 | sed -n '1,12p'

say "Starting zero"
( cd "$ROOT/zero" && exec "$BIN" zero --my localhost:5080 ) >"$ROOT/zero.log" 2>&1 &
ZERO_PID=$!

say "Starting alpha"
( cd "$ROOT/alpha" && exec "$BIN" alpha --my localhost:7080 --zero localhost:5080 ) \
  >"$ROOT/alpha.log" 2>&1 &
ALPHA_PID=$!

say "Waiting for alpha /health"
healthy=0
for _ in $(seq 1 60); do
  if curl -sf localhost:8080/health >/dev/null 2>&1; then healthy=1; break; fi
  sleep 2
done
if [ "$healthy" = 1 ]; then
  ok "alpha healthy"
else
  bad "alpha did not become healthy"
  echo "--- alpha.log (tail) ---"; tail -30 "$ROOT/alpha.log"
  echo "--- zero.log (tail) ---";  tail -30 "$ROOT/zero.log"
  exit 1
fi

say "Set schema"
curl -sf -X POST localhost:8080/alter -d 'name: string @index(exact) .' >/dev/null \
  && ok "alter" || bad "alter"

say "Mutation"
# N-Quads must be newline-terminated, so embed real newlines via $'...'.
MUT=$(curl -s -X POST 'localhost:8080/mutate?commitNow=true' \
  -H 'Content-Type: application/rdf' \
  -d $'{\n  set {\n    _:alice <name> "Alice" .\n    _:bob <name> "Bob" .\n  }\n}')
echo "$MUT"
echo "$MUT" | grep -q '"code":"Success"' && ok "mutate" || bad "mutate"

say "Query"
RES=$(curl -s -X POST localhost:8080/query \
  -H 'Content-Type: application/dql' \
  -d '{ q(func: eq(name, "Alice")) { name } }')
echo "$RES"
echo "$RES" | grep -q '"name":"Alice"' && ok "query returned Alice" || bad "query did not return Alice"

say "Result: PASS=$PASS FAIL=$FAIL"
[ "$FAIL" = 0 ]
