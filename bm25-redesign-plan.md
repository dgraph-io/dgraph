# BM25 Redesign — Implementation Spec

Reworks the BM25 feature per the maintainer's review (decline of the block-storage
PR). Endorsed independently by GPT-5 and Gemini. Goal: BM25 rides Dgraph's standard
posting-list machinery (MVCC, deltas, rollup, splits, backup, snapshot) instead of a
parallel storage+retrieval stack.

## What gets deleted
- `posting/bm25block/` and `posting/bm25enc/` (parallel block format).
- `LocalCache.bm25Writes`, `ReadBM25Blob`/`WriteBM25Blob` (second write path).
- `BitBM25Data` user-meta + the BM25 commit branch in `posting/mvcc.go`.
- `bm25_score` pseudo-predicate + `__bm25_scores__` `ParentVars` threading in `query/query.go`.
- Legacy-format fallback / block dir+block keys in `x/keys.go`.

## Storage model (standard posting lists)
- **Term postings**: one standard index posting list per term at
  `IndexKey(attr, IdentBM25 || term)`. Each posting: `Uid = docUID`,
  `Value = encodeBM25(tf, docLen)`, `ValType = INT`. Written via `plist.addMutation`
  (the normal delta path) → inherits rollup/splits/backup.
  - **Rollup-survival fix (linchpin)**: `NewPosting` makes any edge with `ValueId != 0`
    a `REF` posting, and `List.encode()` (rollup) keeps a posting's `Value` only when
    `Facets != nil || PostingType != REF`. A plain valued REF index posting would have
    its TF **stripped at rollup**. Fix: one-line change in `encode()` to also retain
    postings that carry a non-empty `Value`. This is the faithful realization of the
    maintainer's "TF as the value", and matches how faceted postings already coexist
    in both `Pack` (uid) and `Postings` (value). Covered by a forced-rollup regression test.
- **Doc length**: packed into the posting value alongside TF (`encodeBM25(tf, docLen)`),
  NOT a separate per-predicate doclen list. Rationale: a single doclen list is a write-
  conflict hotspot (every doc mutation writes the same key) and forces a query-time random
  read per candidate. Packing makes scoring read `(uid, tf, docLen)` in one shot,
  contention-free. Cost: docLen duplicated across a doc's unique terms (acceptable; a doc's
  postings are all rewritten together on update anyway).
- **Corpus stats** (`N` docs, `totalTerms` → `avgDL`): conflict-free **bucketed** stats.
  `BM25StatsKey(attr, bucket)`, `bucket = docUID % numBuckets` (B=32). Each bucket holds
  `(docCount_b, totalTerms_b)`. Mutations touch only their bucket → ~B-fold less contention
  than a single hot key. Read path sums across buckets. BM25 tolerates the slight staleness.

## Value codec `encodeBM25(tf, docLen)`
Two unsigned varints: `tf` then `docLen`. Decoded during scoring. Small file
`posting/bm25.go` (no new package) holds encode/decode + index-mutation logic.

## Query path (no pseudo-predicate)
- `bm25(attr, "query", [k], [b])` parses to `bm25SearchFn` (unchanged keyword).
- `worker/task.go handleBM25Search`: tokenize query, read bucketed stats → `N`, `avgDL`,
  load each term's standard posting `List` via the cache, run WAND, emit `UidMatrix`
  (uids asc) + `ValueMatrix` (float64 scores aligned to uids).
- **Surfacing/ordering the score**: via Dgraph's existing **value-variable** (`val()`)
  mechanism — the function's `ValueMatrix` populates a value var the user binds and orders
  by. No `bm25_score` pseudo-predicate, no new `ParentVars` channel.

## WAND on the standard iterator (no parallel block format)
Dgraph loads a whole posting list (or split-part) into memory on `Get`. So:
- For each query term, one `List.Iterate` pass materializes a sorted cursor of
  `(uid, tf, docLen)`, plus `df`, term `maxTF`, and per-chunk (128) `maxTF`/`minDocLen`
  for Block-Max upper bounds — all computed from the in-memory list, **no storage-format
  change**.
- WAND / Block-Max WAND DAAT with a top-k min-heap (reuse scoring + heap from the existing
  `worker/bm25wand.go`, swapping the block-reading cursor for the standard-list cursor).
- (Future optimization, out of scope now: persist per-block maxTF at rollup to avoid
  recomputing for hot terms.)

## Scoring
`idf = log1p((N - df + 0.5)/(df + 0.5))`; `score = Σ idf·(k+1)·tf / (k·(1-b+b·dl/avgDL) + tf)`.
Defaults `k=1.2`, `b=0.75`.

## Implementation phases
1. Storage+index: `encode()` retention fix; `posting/bm25.go` (value codec + mutations);
   bucketed stats; delete bm25block/bm25enc, bm25Writes, BitBM25Data, mvcc branch.
2. Keys: trim `x/keys.go` to `BM25IndexKey` + bucketed `BM25StatsKey`.
3. Tokenizer: keep `BM25Tokenizer` + query tokens (minor cleanup).
4. Query+WAND: rewrite `worker/bm25wand.go` over standard lists; rewrite `handleBM25Search`;
   remove pseudo-predicate/ParentVars from `query/query.go`; wire value-var scoring.
5. Tests: forced-rollup TF-survival test; bucketed-stats test; WAND unit tests over standard
   lists; adapt `query/query_bm25_test.go`; build + run.
