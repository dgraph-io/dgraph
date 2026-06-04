# Native Hybrid Search with Score Fusion — Design Spec

**Branch:** `sp/hybrid-search` (off `sp/bm25`)
**Date:** 2026-06-03
**Status:** Approved (Option C)

## 1. Problem

Dgraph can rank text (BM25, on `sp/bm25`) and find nearest vectors (HNSW `similar_to`),
but cannot **combine** them. A consumer wanting hybrid retrieval (the standard RAG
pattern: dense vector + sparse/keyword, fused into one ranked list) must issue
separate DQL queries and fuse the results in application code.

Concretely, the reference consumer (modelhub, a GraphRAG product running on Dgraph)
issues **three** separate DQL queries per search — (1) `similar_to` vector, (2)
BM25-ish term, (3) entity-anchored term — and fuses them in Python with Reciprocal
Rank Fusion (`score = Σ 1/(k+rank)`, k=60), sometimes with a linear combination
(`α·vec + (1-α)·text`, scores max-normalized first).

This spec brings that fusion **natively into a single DQL query**.

## 2. Approach (Option C)

Two pieces, agreed after consulting GPT-5 and Gemini (both recommended the N-way
primitive as the core; Option C adds a thin convenience wrapper):

1. **`fuse()`** — an N-way fusion combinator over already-scored DQL **value
   variables**. The general primitive. Handles any number of channels and any
   scored signal, not just bm25/vector.
2. **`hybrid()`** — convenience sugar for the common 2-channel bm25+vector case,
   expanded at query-rewrite time into two channel blocks + a `fuse()`. No new
   executor path.

A prerequisite makes `fuse()` useful with vectors:

3. **Surface `similar_to` similarity scores as a value variable** (today it returns
   only uids). Required so vector results can be a fusion channel.

### Why this fits Dgraph's architecture

- DQL already has **value variables** carrying uid→score maps (`var(func: bm25(...))`
  binds per-doc scores into a `varValue{Uids, Vals}` — see `query/query.go`
  `populateUidValVar`, the `bm25` case).
- `ProcessQuery` already has a **variable-dependency scheduler** (`canExecute` over
  `QueryVars.Needs`/`.Defines`): a block runs only once the variables it needs are
  populated. A `fuse()` block that *needs* its channel vars and *defines* the fused
  var is scheduled automatically after its inputs — no new sequencing logic.
- Fusion is a **coordinator-side** operation over resolved variables (like `math()`
  and `uid(a,b)`), which is correct under predicate sharding: each channel may be
  computed on a different Raft group; their value variables are already merged to
  the coordinator before fusion runs.

## 3. DQL Surface

### 3.1 `fuse()` — N-way primitive

```
v as var(func: bm25(text, "quick brown fox"))
e as var(func: similar_to(emb, 100, $queryVec))

f as var(func: fuse(v, e, method: "rrf", k: 60))

{
  result(func: uid(f), orderdesc: val(f), first: 10) {
    uid
    val(f)
  }
}
```

- **Positional args**: two or more value-variable references (the channels). These
  become the block's `NeedsVar`.
- **Named options** (parsed like `similar_to`'s `ef:`/`distance_threshold:`):
  - `method`: `"rrf"` (default) or `"linear"`.
  - `k`: RRF rank constant (default `60`). Ignored for linear.
  - `weights`: comma-separated per-channel weights for linear, aligned positionally
    with the channel args (e.g. `weights: "0.3,0.7"`). Default: `1.0` each. Ignored
    for RRF.
  - `normalize`: linear-only score normalization, `"max"` (default) or `"none"`.
  - `topk`: optional cap on the number of fused results emitted (default: unbounded;
    downstream `first/offset` still applies). Bounds coordinator work.
- **Output**: binds `f` as a value variable — both the **union** uid set
  (`uid(f)`) and the uid→fusedScore map (`val(f)`, `orderdesc: val(f)`), exactly
  like the bm25 ranker var.

### 3.2 `hybrid()` — 2-channel sugar

```
f as var(func: hybrid(text, "quick brown fox", emb, $queryVec, 100, method: "rrf", k: 60))

{ result(func: uid(f), orderdesc: val(f), first: 10) { uid val(f) } }
```

Positional: `textPredicate, "queryText", vectorPredicate, $queryVec, topk`. Named
options as for `fuse()`. Rewritten at query-build time to:

```
__hybrid_0_<id> as var(func: bm25(text, "quick brown fox"))
__hybrid_1_<id> as var(func: similar_to(emb, 100, $queryVec))
f as var(func: fuse(__hybrid_0_<id>, __hybrid_1_<id>, method: "rrf", k: 60))
```

The synthetic var names are unique per hybrid block. After rewrite there is no
distinct `hybrid` execution path — it is purely a parser/builder transformation.

## 4. Fusion Semantics (the core, `query/fuse.go`)

Pure function over channels, fully unit-testable, no I/O:

```go
type fuseChannel struct {
    scores map[uint64]float64 // uid -> raw channel score (higher = better)
    weight float64            // linear weight (default 1.0)
}

func fuseRRF(channels []fuseChannel, k float64, topk int) []scoredUid
func fuseLinear(channels []fuseChannel, normalize string, topk int) []scoredUid
```

### Outer-join / union semantics (the key correctness point)

Both models independently flagged this. Fusion is a **set union** of candidate uids
across channels, NOT an intersection. For a uid present in some channels but not
others:

- **RRF**: only ranked channels contribute. `fused[u] = Σ_{c: u∈c} 1/(k + rank_c(u))`.
  A channel that doesn't contain `u` adds nothing (equivalent to rank = ∞).
- **Linear**: missing channel contributes `0`. `fused[u] = Σ_c weight_c · norm_c(score_c(u))`,
  where `norm_c(missing) = 0`.

Standard DQL `math()` across var blocks aligns/intersects on uid and would drop or
NaN-poison such uids — `fuse()` must not.

### RRF ranks

Each channel is independently sorted by raw score **descending**, tie-broken by uid
**ascending**, to assign 1-based ranks. (Deterministic; matches the bm25/HNSW
`sorted()` tie-break already in the codebase.)

### Linear normalization

`"max"` (default): divide each channel's scores by that channel's max
(`|max|`, guard against 0 → channel contributes 0). Brings heterogeneous score
scales (BM25 ∈ [0,∞), cosine ∈ [-1,1]) onto a comparable range before weighting.
`"none"`: use raw scores (caller asserts they're comparable).

### Output ordering

Fused results sorted by fused score descending, tie-broken by uid ascending. If
`topk > 0`, truncate to `topk`. The output is emitted to the value variable as the
union uid set (ascending, per the query pipeline contract) + positionally-aligned
fused scores — identical shape to the bm25 var binding.

## 5. Vector score surfacing (prerequisite)

`similar_to` currently emits only `UidMatrix` (worker/task.go ~L417). Change:

- Extend `index.SearchPathResult` with `Distances []float64` parallel to `Neighbors`,
  populated from the search-layer heap (which already holds metric-domain distances,
  `n.value`) in `addFinalNeighbors`.
- In the `similarToFn` worker path, when the function is bound to a value variable,
  use the path-returning search to obtain distances and emit a `ValueMatrix` of
  **similarity** scores (higher = better):
  - **cosine**: cosine similarity in [-1,1], as-is.
  - **dotproduct**: dot product, as-is.
  - **euclidean**: `1/(1 + d)` where `d` is the (non-squared) Euclidean distance,
    mapping to (0,1] so higher = better and linear normalization is well-behaved.
- Bind in `populateUidValVar` exactly like the `bm25` case (reuse/generalize that
  branch). When `similar_to` is **not** bound to a var, behavior is unchanged
  (uids only) — zero overhead for existing queries.

Orientation contract: **all rankers surface higher-is-better scores**, so RRF and
linear fusion compose without per-channel sign handling.

## 6. Execution flow

1. Parse: `fuse`/`hybrid` recognized as functions in `dql/parser.go`; channel args →
   `NeedsVar`, named options stored on the function. `hybrid` expands to channel
   blocks + `fuse`. The block `Defines` its output var, `Needs` its channel vars.
2. Schedule: `ProcessQuery`'s `canExecute` runs channel blocks first; the `fuse`
   block becomes runnable once all channel vars are in `req.Vars`.
3. Compute: the `fuse` block is **coordinator-only** — like the existing
   `similar_to`-empty/`IsEmpty` cases, it is **not** dispatched to a worker. Fusion
   is computed in `populateVarMap` (new `fuse` case) reading channel `varValue`s
   from `doneVars`, producing the fused `varValue`.
4. Consume: downstream `uid(f)` / `orderdesc: val(f)` / `first`/`offset` work via the
   existing value-variable machinery.

## 7. Validation & errors

- ≥1 channel var required (2+ to be meaningful; 1 is allowed and passes through).
- Unknown `method` → error. `k <= 0` → error. `weights` count must match channel
  count when provided → error. Malformed `weights` floats → error.
- Empty channels (no matches) are valid and contribute nothing.
- A channel var that is a **uid variable without scores** (no `Vals`): for RRF, rank
  by the var's intrinsic order if any, else treat as unscored → error with a clear
  message ("fuse channel %q has no scores; use a ranker like bm25/similar_to"). MVP:
  require scored channels.

## 8. Testing

**Unit (`query/fuse_test.go`)** — pure fusion core:
- RRF: known ranks → known `Σ 1/(k+rank)`; default k=60; custom k.
- Linear: max-normalize, weights, `normalize:none`.
- Union semantics: uid in 1 of N channels; disjoint channels; full overlap.
- Ties (equal scores → uid-ascending rank); empty channels; single channel.
- `topk` truncation; determinism.

**Worker (`worker/`)** — vector score surfacing:
- `similar_to` bound to a var emits similarity scores with correct orientation per
  metric; unbound `similar_to` unchanged.

**Integration (systest/DQL)** — end-to-end:
- `fuse()` RRF over bm25 + vector; ordering matches hand-computed RRF.
- `fuse()` linear with weights.
- 3-channel fusion (modelhub's shape).
- Pagination (`first`/`offset`) on the consuming block.
- Missing-uid union correctness.
- `hybrid()` produces results identical to the equivalent explicit `fuse()`.
- Error paths (unknown method, bad weights, unscored channel).

## 9. Out of scope (future)

- Filter pushdown into HNSW (pre-filtered ANN) — separate gap.
- Worker-side fusion pushdown / `topk` propagation into channel funcs.
- Additional methods (weighted-RRF, ISR, distribution-based fusion).
- Reranking primitives.

## 9a. Adversarial review outcomes (GPT-5 + Gemini)

Both models deep-reviewed the diff. Findings triaged and resolved:

- **Behavior preservation for `similar_to` (High, both):** the worker no longer
  routes plain (no-option) vector queries through the options path. New
  `SearchScored` / `SearchWithUidScored` mirror `Search` / `SearchWithUid` exactly
  and just also return scores, so existing queries' neighbor selection is unchanged;
  the `*Options*` scored variants are used only when ef/distance-threshold is given.
- **NaN/Inf safety (both):** `scoresFromVar` drops non-finite scores and
  `channelMaxAbs` ignores them, so a pathological score can never break the sort
  comparator's strict-weak-ordering or poison a linear sum.
- **"Ascending Uids destroys ranking" (both, flagged Critical):** not a bug — this
  follows the same value-variable contract as bm25 (Uids is the unordered set for
  `uid(var)`; ranked order is recovered via `orderdesc: val(var)`; `topk` selection
  happens before the ascending sort). Documented in `computeFuse`.
- **Undefined channel var (GPT-5 Critical "stall"):** not a stall — `checkDependency`
  rejects it at parse time ("variables used but not defined"). Regression test added.
- **Synthetic var collision (both, Low):** `__hybrid` is now a reserved prefix;
  user vars using it are rejected with a clear message.
- **`@filter(similar_to(...))` + ValueMatrix (both, Med):** follows the proven bm25
  precedent (bm25 emits ValueMatrix and is used in filters); to be confirmed by the
  vector integration suite in CI.
- **Coordinator GC churn in `channelRanks` (Gemini, perf):** acknowledged; acceptable
  at expected channel sizes, noted as future optimization (buffer pooling) if needed.

## 10. Files touched

| Area | File(s) |
|---|---|
| Fusion core (new) | `query/fuse.go`, `query/fuse_test.go` |
| Parser | `dql/parser.go` (recognize `fuse`/`hybrid`, args+opts), `dql/state.go` if needed |
| hybrid rewrite | `query/query.go` (ToSubGraph/build) or `dql` transform |
| Var binding | `query/query.go` `populateUidValVar` (fuse case; generalize bm25 case) |
| Scheduler skip-worker | `query/query.go` `ProcessQuery` (fuse → no worker dispatch) |
| Vector scores | `tok/index/search_path.go` (`Distances`), `tok/hnsw/persistent_hnsw.go` (populate), `worker/task.go` (`similar_to` ValueMatrix) |
| Docs | DQL docs for `fuse`/`hybrid` |
