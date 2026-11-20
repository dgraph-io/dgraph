#!/usr/bin/env bash
# Loads a layered graph into the local alpha and runs a query whose result fans out, to exercise memwatch.
#
#   ./spike.sh          # N=60 nodes per layer, DEPTH=3 hops: about 13M result nodes, an OOM kill at 1g
#   N=40 ./spike.sh     # about 2.6M result nodes: triggers a memwatch capture without killing the alpha
#   LOAD=0 ./spike.sh   # rerun the query without reloading (loading drops all data first)
#
# Every node in layer k links to every node in layer k+1, so the result has N^(DEPTH+1) nodes. Dgraph builds the
# whole result in memory before sending it.
set -euo pipefail
ALPHA="${ALPHA:-http://localhost:8080}"
N="${N:-60}"
DEPTH="${DEPTH:-3}"
LOAD="${LOAD:-1}"
LAYERS=$((DEPTH + 1))

# Dgraph reports most failures as HTTP 200 with an "errors" field, so check the body as well as the status.
dg() {
  local out
  if ! out=$(curl -sS --fail-with-body "$@"); then
    echo "request failed: ${out:0:300}" >&2
    exit 1
  fi
  if [[ "$out" == *'"errors"'* ]]; then
    echo "request failed: ${out:0:300}" >&2
    exit 1
  fi
}

if [[ "$LOAD" == 1 ]]; then
  # Start from an empty database so N and DEPTH mean what they say.
  dg "$ALPHA/alter" -d '{"drop_all": true}'
  dg "$ALPHA/alter" -d $'name: string .\nlayer: int @index(int) .\nlink: [uid] .\n'
  rdf=$(mktemp)
  trap 'rm -f "$rdf"' EXIT
  {
    echo "{ set {"
    for ((l = 0; l < LAYERS; l++)); do
      for ((i = 0; i < N; i++)); do
        echo "_:n${l}_${i} <name> \"n${l}_${i}\" ."
        echo "_:n${l}_${i} <layer> \"${l}\" ."
        if ((l + 1 < LAYERS)); then
          for ((j = 0; j < N; j++)); do
            echo "_:n${l}_${i} <link> _:n$((l + 1))_${j} ."
          done
        fi
      done
    done
    echo "} }"
  } >"$rdf"
  echo "loading $((N * LAYERS)) nodes and $((N * N * DEPTH)) edges"
  dg "$ALPHA/mutate?commitNow=true" -H 'Content-Type: application/rdf' --data-binary "@$rdf"
fi

sel="name"
for ((d = 0; d < DEPTH; d++)); do
  sel="name link { $sel }"
done
query="{ q(func: eq(layer, 0)) { $sel } }"
echo "query returns about $(awk -v n="$N" -v l="$LAYERS" 'BEGIN { printf "%d", n ^ l }') nodes: $query"
start=$(date +%s)
resp=$(mktemp)
code=$(curl -s -o "$resp" -w '%{http_code}' --max-time 300 "$ALPHA/query" -H 'Content-Type: application/dql' \
  -d "$query" || true)
echo "HTTP ${code}, $(wc -c <"$resp" | tr -d ' ') bytes after $(($(date +%s) - start))s" \
  "(000 means the connection dropped, e.g. the alpha was OOM-killed)"
if grep -q '"errors"' "$resp"; then
  head -c 300 "$resp"
  echo
fi
rm -f "$resp"
