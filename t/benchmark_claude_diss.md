# Shortest Path (SSSP) Benchmark — Dgraph vs. LDBC Graphalytics

This document describes how to implement and validate a Dijkstra shortest-path benchmark
for Dgraph using the LDBC Graphalytics benchmark suite as the correctness reference.

---

## Background

**Dgraph's `shortest` query block** finds the shortest path between two nodes using BFS
(unweighted) or Dijkstra's algorithm (weighted via edge facets).

**LDBC Graphalytics** is a separate LDBC benchmark from the Social Network Benchmark. It
benchmarks six graph algorithms — one of which is **SSSP (Single-Source Shortest Paths)**.
It provides publicly downloadable reference output files that a third-party implementation
can be validated against using a 0.01% epsilon tolerance.

**Important distinction:** Graphalytics SSSP computes distances from one source vertex to
*all* other vertices. Dgraph's `shortest` block is point-to-point (source → one target).
This benchmark has two separable goals:

- **Correctness validation**: Run `shortest` from the SSSP source vertex to every other
  vertex, collect all distances, and diff against the Graphalytics reference output.
- **Performance benchmarking**: Run `shortest` for randomly sampled (source, target) pairs
  and measure latency/throughput at scale.

These can be developed independently — correctness first, performance second.

---

## Step 1 — Choose a Graphalytics Dataset

Graphalytics uses its own graph datasets, not the LDBC SNB social network data already in
this repo. Use a **datagen** graph (the synthetic family that includes edge weights and
supports SSSP). Start small:

| Graph | Vertices | Edges | Size category |
|---|---|---|---|
| `datagen-7_5-fb` | ~633 K | ~34 M | M |
| `datagen-7_6-fb` | ~754 K | ~42 M | M |
| `datagen-8_5-fb` | ~2.1 M | ~106 M | L |

Avoid `graph500-*` graphs — they have no edge weights and SSSP is not defined for them.

Datasets are listed at: https://ldbcouncil.org/benchmarks/graphalytics/

---

## Step 2 — Download Graph Data and Reference Outputs

Each dataset is distributed as two archives:

```
datagen-7_5-fb.tar.zst          # graph data (vertices + weighted edges)
datagen-7_5-fb-validation.tar.zst  # reference SSSP outputs
```

Download both from the Graphalytics dataset page and extract them locally:

```bash
mkdir -p graphalytics/datagen-7_5-fb
cd graphalytics/datagen-7_5-fb

# download links are on the Graphalytics dataset listing page
tar --zstd -xf datagen-7_5-fb.tar.zst
tar --zstd -xf datagen-7_5-fb-validation.tar.zst
```

After extraction the directory structure looks like:

```
datagen-7_5-fb/
├── datagen-7_5-fb.v          # vertex list (one vertex ID per line)
├── datagen-7_5-fb.e          # edge list: "<src> <dst> <weight>"
├── datagen-7_5-fb.properties # metadata: directed/undirected, SSSP source vertex
└── validation/
    └── datagen-7_5-fb-SSSP   # reference output: "<vertex_id> <distance>"
```

Note the **SSSP source vertex ID** from the `.properties` file — you will need it when
loading data and when issuing queries:

```
# example .properties excerpt
graph.directed = false
algorithms.sssp.source-vertex = 1
```

---

## Step 3 — Design the Dgraph Schema

Map the Graphalytics edge-list format to a Dgraph schema. Edge weights become facets on
the relationship predicate so Dgraph's Dijkstra implementation can use them.

```graphql
# schema to apply via Dgraph alter before loading

graphalytics_id: int @index(int) .
distance: float .

type Vertex {
    graphalytics_id
}

connected: [uid] @reverse .
# weight stored as a facet on the `connected` edge, not a predicate
```

> The `connected` predicate is intentionally generic so this schema can host any
> Graphalytics graph. Rename it if you want per-dataset schemas.

Apply with pydgraph:

```python
import pydgraph

schema = """
    graphalytics_id: int @index(int) .
    connected: [uid] @reverse .

    type Vertex {
        graphalytics_id
    }
"""
op = pydgraph.Operation(schema=schema)
client.alter(op)
```

---

## Step 4 — Load the Graph into Dgraph

### 4a. Create vertex nodes

Read the `.v` file and bulk-upsert one node per vertex, storing `graphalytics_id` so you
can look up UIDs later:

```python
import pydgraph, json

def load_vertices(client, vertex_file, batch_size=5000):
    with open(vertex_file) as f:
        vertex_ids = [int(line.strip()) for line in f if line.strip()]

    for i in range(0, len(vertex_ids), batch_size):
        batch = vertex_ids[i:i+batch_size]
        mutations = [
            {"dgraph.type": "Vertex", "graphalytics_id": vid}
            for vid in batch
        ]
        txn = client.txn()
        try:
            txn.mutate(set_obj=mutations)
            txn.commit()
        finally:
            txn.discard()
```

### 4b. Build a graphalytics_id → Dgraph UID map

After loading vertices, query back all UIDs so edges can reference them:

```python
def build_uid_map(client):
    query = """{ vertices(func: type(Vertex)) { uid graphalytics_id } }"""
    txn = client.txn(read_only=True)
    res = json.loads(txn.query(query).json)
    return {v["graphalytics_id"]: v["uid"] for v in res["vertices"]}
```

For large graphs (>1 M vertices) paginate this query with `first` / `offset` or use
a DQL `@cascade` export.

### 4c. Load edges with weight facets

Read the `.e` file (`<src> <dst> <weight>`) and create `connected` edges with the weight
stored as a facet:

```python
def load_edges(client, edge_file, uid_map, directed=False, batch_size=2000):
    with open(edge_file) as f:
        edges = [line.split() for line in f if line.strip()]

    for i in range(0, len(edges), batch_size):
        batch = edges[i:i+batch_size]
        nquads = []
        for src_id, dst_id, weight in batch:
            src_uid = uid_map[int(src_id)]
            dst_uid = uid_map[int(dst_id)]
            # facet syntax in NQuad form
            nquads.append(
                f'<{src_uid}> <connected> <{dst_uid}> <_:e{i}> (weight={float(weight)}) .'
            )
            if not directed:
                nquads.append(
                    f'<{dst_uid}> <connected> <{src_uid}> <_:e{i}r> (weight={float(weight)}) .'
                )

        txn = client.txn()
        try:
            txn.mutate(set_nquads="\n".join(nquads))
            txn.commit()
        finally:
            txn.discard()
```

Check the `.properties` file for `graph.directed` to decide whether to insert reverse edges.

---

## Step 5 — Run SSSP Queries

Dgraph's `shortest` block with a weight facet runs Dijkstra's algorithm. To produce the
full SSSP output required for Graphalytics validation, run one query per target vertex
from the fixed source vertex.

```python
def shortest_path(client, src_uid, dst_uid):
    query = f"""
    {{
        path as shortest(from: {src_uid}, to: {dst_uid}, numpaths: 1) {{
            connected @facets(weight: weight)
        }}
        result(func: uid(path)) {{
            graphalytics_id
            _path_ {{
                graphalytics_id
                connected @facets(weight)
            }}
        }}
    }}
    """
    txn = client.txn(read_only=True, best_effort=True)
    try:
        res = json.loads(txn.query(query).json)
        # extract total distance from path facets
        return extract_distance(res)
    finally:
        txn.discard()
```

For unreachable vertices (disconnected components), Dgraph returns an empty `path` block —
these should be recorded as `Infinity` in the output.

### Running full SSSP

```python
def run_sssp(client, source_graphalytics_id, uid_map):
    src_uid = uid_map[source_graphalytics_id]
    results = {}

    for gid, uid in uid_map.items():
        if gid == source_graphalytics_id:
            results[gid] = 0.0
            continue
        dist = shortest_path(client, src_uid, uid)
        results[gid] = dist if dist is not None else float("inf")

    return results
```

> **Performance note:** Running N queries sequentially is slow for large graphs. Consider
> parallelising with a thread pool (`concurrent.futures.ThreadPoolExecutor`) to saturate
> Dgraph's query capacity. The correctness validation step can be done on a small graph
> (a few thousand vertices); full-scale SSSP is the performance benchmark.

---

## Step 6 — Write Output in Graphalytics Format

The reference validator expects `<vertex_id> <distance>`, one line per vertex, space
separated, `Infinity` for unreachable vertices:

```python
def write_output(results, output_file):
    with open(output_file, "w") as f:
        for vertex_id, distance in sorted(results.items()):
            if distance == float("inf"):
                f.write(f"{vertex_id} Infinity\n")
            else:
                f.write(f"{vertex_id} {distance}\n")
```

---

## Step 7 — Validate Against Reference Output

LDBC validates with a **0.01% epsilon tolerance** (`|actual - expected| ≤ 0.0001 × expected`).
`Infinity` must match exactly. Vertex sets must be identical.

Use the standalone Python validator from the Graphalytics repo, or use this equivalent:

```python
EPSILON = 0.0001

def validate(reference_file, actual_file):
    def load(path):
        result = {}
        with open(path) as f:
            for line in f:
                vid, dist = line.split()
                result[int(vid)] = float("inf") if dist == "Infinity" else float(dist)
        return result

    reference = load(reference_file)
    actual = load(actual_file)

    if set(reference.keys()) != set(actual.keys()):
        print(f"FAIL: vertex set mismatch")
        return False

    failures = []
    for vid, expected in reference.items():
        got = actual[vid]
        if expected == float("inf") and got == float("inf"):
            continue
        if expected == float("inf") or got == float("inf"):
            failures.append((vid, expected, got))
        elif abs(expected - got) > EPSILON * expected:
            failures.append((vid, expected, got))

    if failures:
        print(f"FAIL: {len(failures)} vertices out of tolerance")
        for vid, exp, got in failures[:10]:
            print(f"  vertex {vid}: expected {exp}, got {got}")
        return False

    print(f"PASS: all {len(reference)} vertices within epsilon")
    return True
```

Run:

```python
validate(
    "graphalytics/datagen-7_5-fb/validation/datagen-7_5-fb-SSSP",
    "output/datagen-7_5-fb-sssp-dgraph.txt"
)
```

---

## Step 8 — Performance Benchmark

Once correctness is confirmed on a small graph, switch to performance mode:

- Use a larger graph (L or XL scale factor)
- Sample random (source, target) pairs — vary by expected graph distance (nearby vs. far)
- Run concurrent queries with a thread pool and measure:
  - Median, p95, p99 latency
  - Queries per second at varying concurrency levels
- Compare weighted (Dijkstra) vs. unweighted (BFS) query times

This naturally plugs into the existing throughput test patterns in this repo (see
`throughputtest/throughPut.go` for the concurrency and stats model to follow).

---

## File Layout (suggested)

```
ldbc/shortest-path/
├── README.md               # this file
├── load.py                 # Steps 3–4: schema + data loading
├── sssp.py                 # Steps 5–6: run queries, write output
├── validate.py             # Step 7: correctness check
├── benchmark.py            # Step 8: performance runs
├── config.py               # Dgraph endpoint, dataset paths
└── results/                # output files and benchmark CSVs
```

---

## Quick Reference

| Thing | Value |
|---|---|
| Graphalytics datasets page | https://ldbcouncil.org/benchmarks/graphalytics/ |
| Graphalytics GitHub | https://github.com/ldbc/ldbc_graphalytics |
| Dgraph `shortest` docs | https://dgraph.io/docs/query-language/shortest-path/ |
| Reference implementation | https://github.com/ldbc/ldbc_graphalytics_platforms_graphblas |
| Output format | `<int vertex_id> <float distance or "Infinity">` per line |
| Epsilon tolerance | 0.01% (`abs(actual - expected) <= 0.0001 * expected`) |
| Recommended start dataset | `datagen-7_5-fb` (M scale, has SSSP source vertex defined) |
