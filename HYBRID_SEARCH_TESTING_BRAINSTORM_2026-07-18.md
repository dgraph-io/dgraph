# Hybrid-Search Testing Deep Dive — sp/hybrid-search

**Date:** 2026-07-18 · **HEAD:** `1ab735ee` (clean) · **Mode:** god (time-boxed) ·
**Contract:** correctness integration tests first, then performance attack; hybrid+BM25 scope; docker allowed; implement in-session.

## Method

89 blind test-attack questions (GPT-5 ×15, Gemini ×15, MiniMax ×12, 4 repo-grounded Claude
finders ×47) → merged/deduped to 35 (20 P0) → each premise verified in code by 9 grounding
agents (file:line evidence) → finalists falsified via opencode + live-cluster probes.
Artifacts: `scratchpad/bdd/{bank,verdicts,groups}.json`.

**Verdicts:** 17 CONFIRMED-VULNERABLE · 8 PLAUSIBLE (test discriminates) · 9 SAFE (cheap
regression pins) · 1 premise-false.

## Empirically confirmed bug (live cluster, before any new test code)

**Q1 — `bm25(...) + @filter + first:N` returns the wrong page.**
`query/query.go:1031` pushes `First` to the worker only when `len(sg.Filters)==0`; with a
filter the worker scores everything and the coordinator slices the uid-sorted matrix
(`query/query.go:2561`) — the page is the N **lowest-UID** matches, not the N top-scored.
Probe on the test corpus: score order `0x1f7(0.659), 0x1f5(0.375)`; filtered `first:2`
returned `{0x1f5, 0x1f6}` — the top document is silently dropped.

## Top-5 portfolio (ranked; prerequisites respected)

### 1. Silent-wrong-page suite — pagination/ordering integrity (P0, existing compose)
Q1 (CONFIRMED — static + empirical probe + opencode, triple-verified), Q3 (tie storms/page
partition/determinism), Q12 (part 1 CONFIRMED: `orderdesc: val(b)` on `uid(b,v)` silently
deletes uids unscored in `b`, `query/query.go:2777`; part 2 — multi-key var sort fallthrough
— REFUTED: parser rejects it at `dql/parser.go:3099`).
*Why now:* the exact silent-wrong-result class the branch cannot ship with.
*Acceptance:* filtered/paginated pages equal score-ordered oracle; page walk partitions
the corpus; vector-only uids survive ordering (or explicit error).
*First action:* `TestBM25FilterThenFirstPreservesScoreOrder` (fails today → drives fix).

### 2. Fusion/hybrid contract battery (P0, compose + unit)
Q5 (NaN/Inf query vector e2e), Q6 (hybrid topk candidate completeness vs exhaustive
oracle), Q7 (fuse-block modifiers silently ignored — parser must reject like hybrid does),
Q8 (channel validity: uid-var channels, empty channels, count-var phantom uids), Q9 (fusion
math pins: window-dependent normalization), Q29 (`fuse(a,a)` duplicate channel), Q30
(fuse-of-fuse, shared channels), Q11 (directive matrix: @cascade/@normalize/groupby).
*Acceptance:* every silent-degrade path either errors or matches documented semantics.

### 3. Score-binding oracle pins (P0/P1 regression, compose)
Q2/Q4 verified SAFE by code-read — pin them: per-uid `val(s)` equals unfiltered oracle
after var-bound `@filter`+pagination+two-path traversal, for bm25 AND similar_to; metric
math exact (`1/(1+d)` euclidean, raw cosine/dot). Q28 degenerate similar_to inputs
(k=0, dim mismatch, NaN vector components).
*Why:* the snapshot fix (`250b79be`) has zero e2e regression coverage; a refactor could
silently reintroduce misbinding.

### 4. Stats-integrity + lifecycle suite (P0/P1, compose then LocalCluster)
Compose-feasible now: Q13 (idempotent re-SET churn), Q15 (intra-txn parallel RMW), Q25
(alter cycles), Q27 (lang-tagged delete asymmetry — CONFIRMED hole).
LocalCluster wave (L-effort): Q16 (rebuild under live writes — falsification corrected the
mechanism: live writes between DropPrefix and the absolute flush RMW from an EMPTY bucket
and shadow the rebuild's total via MVCC → **undercount**; test assertion unchanged),
Q17/Q19 (multi-group parity, replica divergence), Q23 (bulk/live parity), Q22 (HNSW MVCC
lifecycle), Q24 (backup/restore). Q20 (crash-mid-rebuild forever-empty) was **REFUTED** by
opencode falsification — schema commits only after BuildIndexes and Raft WAL replay re-runs
the rebuild — downgraded to a Wave-2 crash-recovery sanity check.
*Acceptance:* stats equal recomputed oracle after every lifecycle transition; crash never
leaves silent-empty index.

### 5. Performance attack (P2, after correctness)
Q31 WAND effectiveness (Zipfian corpora; postings-decoded counters; first:k vs exhaustive
crossover), Q33 ingest contention (writers × bucket collisions; CONFIRMED: batch >32
docs/txn guarantees intra-txn conflicts), Q32 fusion overhead (channel size sweep), Q35
per-query floors (32-bucket stats read), Q34 scored-HNSW overhead pin (until `WantScores`
lands). Note: local image is built WITHOUT jemalloc — absolute numbers shift, relative
comparisons valid.

## Not eligible / deferred
- Q14 premise-false (WAND bounds are computed per-query from live postings — no stale-bound
  mechanism); cheap differential pin only.
- Q18 SAFE (channel-group failure propagates as whole-query error — pin later).
- Q21/Q26 SAFE (snapshot-consistent bucket reads; namespace-prefixed keys) — unit pins.
- #8 scored-path overhead fix itself remains blocked on protoc (separate item).

## Harness notes (established this session)
- `dgraph/dgraph:local` built via pure-Go cross-compile (`CGO_ENABLED=0 GOOS=linux`),
  bypassing sudo-blocked jemalloc; binary smoke-tested.
- Cluster: `COMPOSE_COMPATIBILITY=true LINUX_GOBIN=<dir-with-linux-binary> docker compose
  -p dgraph -f dgraph/docker-compose.yml up -d` (v1 underscore names required by
  `testutil.getContainer`).
- Run: `TEST_DOCKER_PREFIX=dgraph go test -tags integration ./query/ -run <T> -count=1`.
- Multi-group/kill: `dgraphtest.LocalCluster` (`WithNumAlphas`, `KillAlpha`) — feasible.

## Wave-1 outcome (implemented this session, commit 0b806236e)

~30 tests added across query/query_bm25_test.go, query/query_hybrid_test.go,
dql/fuse_parser_test.go, query/fuse_test.go, worker/bm25wand_test.go. Full
bm25+hybrid+fuse integration battery green against the live compose cluster.

Six confirmed bugs found and fixed, each gated by a test:
1. **Q1** ranker root + @filter + first:N returned the lowest-uid page, not the
   top-scored (applyPagination now pages by score for ranker roots).
2. **Q7/Q29** fuse() modifiers and duplicate channels silently ignored → parse errors.
3. **Q5** NaN/Inf: dropped at score snapshot; non-finite query vectors rejected.
4. **Q28** similar_to k<=0 panicked (OOM'd) the alpha → rejected before HNSW.
5. **Q8** uid-var fuse channel silently empty — root cause: ShardedMap.IsEmpty()
   only detects nil (fresh map has 30 empty shards) → rejected with clear error.
6. **Q27** @lang + @index(bm25) made tagged values silently unsearchable
   (lang-qualified keys; fulltext-parity) → schema combination rejected.

Predictions that did NOT survive testing: count-var sentinel phantom uid (passes),
groupby crash unreachable via DQL (parser rejects; defensive guard added anyway),
Q20 crash-mid-rebuild (refuted by falsification pass).

Remaining: Wave 2 (LocalCluster lifecycle: Q16 rebuild-undercount, Q17/Q19
multi-group, Q22-Q24) and Wave 3 (benchmarks Q31-Q35).

## Evidence that would change this ranking
- If Q1's fix is trivial and unblocks pushdown+rescore, suite 1 shrinks to regression pins.
- If opencode falsification refutes Q16/Q20 mechanics, the LocalCluster wave drops to P2.
- Telemetry on real query mixes (unverifiable-from-repo) would re-weight the perf attack.
