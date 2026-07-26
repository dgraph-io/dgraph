# Mutation-pipeline TODO

## STATUS: intra-transaction apply-path work is COMPLETE

Six commits, each measured on an `i4i.16xlarge` (32 physical cores / 64 vCPU),
25 `[uid] @reverse` predicates, 20k triples/txn, 64 threads, 300 s,
`badger_write_bytes_user`:

| commit | change | MB/s |
|---|---|---|
| — | branch base | 9.63 |
| `4a02d8eeb` | parallel reverse write; lock-free store at any grant | 10.58 |
| `612ae8cd5` | lock-free forward write at a one-worker grant | 11.44 |
| `5b69ac2b4` | batch conflict-key emission into one lock acquisition | 12.45 |
| `016a03a0a` | snapshot uid lease once per mutation | 12.53 (flat) |
| `04944f125` | size `LockedShardedMap` per machine; drop hash alloc | **13.17** |

**+36.8% cumulative.** Blocked pipeline goroutines fell from ~86% (stock, on the
global cache lock) to **10.5%**; runnable rose to **75.6%**.

### The rule that governs any further work here

CPU utilisation is **~10% of 64 vCPU** and barely moved across all six commits,
because `processApplyCh` applies one Raft entry at a time. So **a change that
only reduces CPU or allocations will not move throughput** — `016a03a0a` removed
~3.5% of CPU for exactly zero gain. Rank candidates by share of **blocked**
goroutines (from `/debug/pprof/goroutine?debug=2`), not CPU. Use a CPU profile to
*explain* a blocker once found, not to pick one.

Bottleneck migration observed, each fix exposing the next — which is why the
order mattered: global `cache.Lock` 86% → `txn.conflicts` 71% →
`Deltas.AddToDeltas` 58% → nothing above ~10%.

**Everything in §2b/§3 below that is CPU-or-allocation-only was skipped on
purpose.** The next real lever is cross-transaction concurrency in
`processApplyCh`, not more intra-transaction work.

(The same summary is in the local, gitignored `CLAUDE.md` so it auto-loads.)

---


Working list for the intra-predicate pipeline work on
`rahst12/hybrid-pipeline-reverse-parallel`. Ordered by what actually blocks
throughput, not by effort.

---

## 1. Simplify the flags

The pipeline now has **four** superflag knobs, and a fifth is proposed. That is
too many for something whose correct setting is not obvious to an operator.

Current (`worker/server_state.go:45-48`, `dgraph/cmd/alpha/run.go:267-310`):

| flag | default | what it really controls |
|---|---|---|
| `mutations-pipeline-threshold` | 1 | use pipeline vs legacy path |
| `mutations-pipeline-goroutines` | 30 | L2 intra-predicate budget (`-1` = auto) |
| `mutations-pipeline-goroutines-fraction` | 1.0 | AUTO only: fraction of GOMAXPROCS |
| `mutations-pipeline-min-edges-per-worker` | 256 | AUTO only: edges/worker cap |
| *(proposed)* `mutations-pipeline-min-targets-per-worker` | 256 | reverse split threshold |

Problems:
- Two of the four are **inert unless `goroutines == -1`**, which is not the
  default — so most operators are tuning flags that do nothing.
- The fixed default of **30 sits exactly on the `allocateWorkers` cliff**: with
  25 predicates it yields a `{1:20, 2:5}` grant, i.e. 20 of 25 predicates get a
  one-worker grant. A budget below ~2x the predicate count effectively disables
  intra-predicate parallelism, silently.
- `reverseParallelMinTargets` is currently a compile-time `const`
  (`posting/index.go`), so it cannot be tuned at all.

Proposed:
- Make **AUTO the default** (`goroutines=-1`) so the budget scales with the box
  instead of a magic 30 that is wrong on both an 8-core and a 148-core machine.
- Collapse `goroutines-fraction` + `min-edges-per-worker` into the auto policy,
  or document them as advanced/diagnostic only.
- Express the reverse threshold as **targets-per-worker**, consistent with the
  edges-per-worker vocabulary already used, rather than an absolute constant.
- Emit a `glog.V(2)` line or a metric when a batch's grant is all-1s, so the
  silent-disable case is observable.

### 1a. `WorkerOptions.String()` hides the pipeline config
`x/config.go:206-210` hand-formats the struct and stops at `Audit:%v}` — none of
the `MutationsPipeline*` fields are printed. The startup log line
`glog.Infof("x.WorkerConfig: %+v")` therefore does **not** show the effective
pipeline settings. This cost real time during live debugging: the log could not
confirm whether the budget was 30 or 0. Add the fields to `String()`.

---

## 2. Performance follow-ups (ranked by measured gain / risk)

1. ~~**Apply the same gate fix to `ProcessList` and `ProcessSingle`.**~~
   **DONE — `612ae8cd5`.** Took the production default from 10.58 → **11.44 MB/s**
   (+18.8% cumulative over stock) and removed the global `cache.Lock` from the
   blocking profile entirely.
   `ProcessReverse` now uses the lock-free store whenever the budget is enabled,
   not only when `workers > 1`. The forward-write paths still tie those two
   decisions together, so a one-worker grant sends them back to the global
   `txn.cache.Lock()`. Measured (perf review): reverse gate alone 1.24x, all
   three gates 1.84x. Two thirds of the available gain is here.
2. ~~**Shard `txn.conflicts`**~~ **DONE differently — `5b69ac2b4` batched it
   instead of sharding.** A line-level profile showed ~80% of the cost was
   lock acquire/release and only ~15% the map insert, so batching to one
   acquisition per `Process*` captured the win; sharding would have chased a
   ~1% residual. Result: 11.44 → **12.45 MB/s (+8.8%)**, conflict path
   68.5% → **0.0%** of pipeline goroutines. Full write-up in
   `conflict-key-batching-plan.md`. Sharding stays closed unless
   `flushConflicts` self-CPU >3% or mutex-BLOCKED >5%.

   Original entry kept for context:
   **Shard `txn.conflicts`** (`posting/index.go` `addConflictKeyWithUid`). One
   `map[uint64]struct{}` under one global `txn.Lock()`, hit once per edge by
   every worker. Microbenchmark ceiling: stubbing it out took the parallel path
   from 36.7 ms to 19.1 ms. Touches the conflict-detection contract that feeds
   `FillContext` -> Zero, so route through the raft expert; assert the emitted
   `ctx.Keys` slice is byte-identical before/after.

   **RE-CONFIRMED at real core count — this is a genuine #2.** Measured on an
   `i4i.16xlarge` (32 physical cores / 64 vCPU), 25 `[uid] @reverse` predicates,
   20k triples/txn, 64 writer threads, after the reverse fix:
   `addConflictKeyWithUid` holds **30.7%** of parked pipeline goroutines
   (up from **0.4%** on stock, because the reverse fix removes the cache-lock
   convoy that was masking it).

   **After `612ae8cd5` (forward gate) it is 71.2%** — now unambiguously the #1
   bottleneck, with the global cache lock gone from the profile entirely.
   Progression across the three builds: **0.4% → 31% → 71.2%**.

   Note the core-count sensitivity — it is why laptop numbers must not be used
   to rank this work: on a 4-physical-core laptop the same measurement showed
   only **2.7%** and I wrongly demoted this item. Contention on a single mutex
   scales with the number of contenders; small boxes cannot see it.
3. **`LockedShardedMap.getShardIndex`** (`types/locked_sharded_map.go:39`) does
   `farm.Fingerprint64([]byte(k))`, which heap-allocates on every Get/Set —
   ~12.9% of all allocations in a 20k-edge batch. `maphash.String` is 0-alloc
   and ~3x faster. Near-free win, also helps the index pass.
4. **`SortAndDedupPostings`** (`posting/oracle.go:75`) uses `sort.Slice`: 2
   allocs + reflection per call, ~28k calls per 20k-edge batch.
   `slices.SortStableFunc` is ~6x faster at n=3 *and* makes the existing
   "last wins" dedup deterministic instead of arbitrary.
5. **`concStore` fast paths** — skip the prior-delta probe when the txn has no
   earlier-proposal deltas; avoid the copy when there is no prior delta. Needs
   an audit that no earlier pass in the same `Process` call can write a reverse
   key (star-delete in particular).
6. **Skew-aware partitioning** — workers currently take contiguous ranges of the
   target snapshot. Speedup is capped at `1/p_max` where `p_max` is the largest
   target's share of postings; a batch can have 8,000 targets and still have one
   target holding half the postings. A chunked atomic counter self-balances at
   ~zero cost.

---

## 2b. Validated on real hardware

See `ec2-validation-results.md`. On an `i4i.16xlarge` (32 physical cores), 25
`[uid] @reverse` predicates, 20k triples/txn, 64 threads, measured by
`badger_write_bytes_user`:

- **+9.9% MB/s at the shipped default** (`goroutines=30`), reproducible to ≤1%.
- **+16.6% MB/s at `goroutines=-1` (auto)**.
- Raising the budget alone on **stock** gains nothing (+0.5%) — auto only pays
  once the reverse write can consume the workers. The "free win from a bigger
  budget" idea is dead.
- CPU stays at **6–8% of 64 vCPU**. A single Alpha cannot saturate a big box
  because `processApplyCh` is serial across Raft entries — worth remembering
  before attributing any future shortfall to intra-transaction parallelism.

## 2c. Beyond the posting pipeline — where the CPU actually goes now

After the three apply-path commits, the post-change profile is no longer
dominated by `posting/`. Measured on the 32-core box:

| | cum | note |
|---|---|---|
| `sync/atomic.(*Int32).Add` | 11–12% | 94% of it is `RWMutex.RLock`/`RUnlock` traffic |
| `worker.(*groupi).Tablet` | 6.7–8.9% | membership lookup, 83% from `proposeAndWait`; ~62% of its own cost is `SafeMutex.RLock`/`RUnlock` |
| `runtime.mallocgc` | ~10% | allocation pressure |
| ~~`MaxLeaseId` / `verifyUid`~~ | ~~3.5%~~ | **fixed in `016a03a0a`** — now absent |

- **DONE `016a03a0a`** — `ExtractBlankUIDs` called `MaxLeaseId` (a global RWMutex
  read for one uint64) per subject AND per object, ~40k acquisitions per 20k-nquad
  mutation. Snapshotted once per call. Removed ~3.5% CPU; **throughput unchanged**,
  because at ~9% CPU on 64 vCPU the Alpha is not CPU-bound.
- **NEXT — `worker.(*groupi).Tablet`.** A read-mostly membership map behind
  `x.SafeMutex`, hit once per proposal. Candidate for an `atomic.Value` snapshot
  or a per-request cache. Largest single remaining item.
- **`NumShards = 30`** (`types/sharded_map.go:18`) — thin for 64 vCPU, and
  `Deltas.AddToDeltas` is the top blocker inside the pipeline now.

## 3. Good to do, not needed now

- **Calibrate `reverseParallelMinTargets`.** 256 is an assumption by analogy to
  `min-edges-per-worker`, not a measurement. Measured behavior at 5 targets
  (serial, correct) and 600/1250/8000 (parallel, 1.15-1.87x), but nothing
  between 5 and 600 — the real crossover is unknown. Sweep on the 148-core box.
- **Cover the uncovered branches** in the parallel reverse path: the
  `sync.Once`/`firstErr` error propagation, and the `k > len(uidList)` clamp.
  Neither is reachable today, but the error path abandons the rest of a worker's
  sub-range, which differs from the serial path's behavior — worth a deliberate
  decision rather than an accident.
- **Guard the `handleDeleteAll` happens-before with a test**, not just the
  comment now in `ProcessReverse`. The lock-free write's single-writer invariant
  depends on `close(pred.edges)` ordering; if star-delete handling ever moves
  off the dispatcher this becomes silent reverse-index corruption with no panic
  and no race-detector hit.
- **Wire mutex/block profiling to the live endpoints.** `--profile_mode mutex`
  did not enable `/debug/pprof/mutex` (it came back with `sampling period=0`),
  so the contended mutex had to be inferred from goroutine stacks instead of
  named directly. A small `--debug-profile-rates` style flag calling
  `runtime.SetMutexProfileFraction` / `SetBlockProfileRate` would make future
  contention work much faster.
- **Fix `active_mutations_total` not existing until the first mutation**
  (issue #9671) — unrelated to this work but trivial and it breaks dashboards.
- **Parallelize `commitOrAbort`'s per-txn `CommitToDisk` loop**
  (`worker/draft.go`) across the txns in an `OracleDelta`. Independent of the
  apply-side work here. Note Badger's `CommitAt` is already async, so the gain
  is CPU parallelism over iterating `cache.deltas` — which is large precisely
  because high-cardinality reverse predicates generate thousands of delta keys.
- **Factor the duplicated `concStore`.** `ProcessList` and `ProcessReverse` now
  carry near-identical read-merge-sort-write closures. A shared helper would
  stop them drifting, but doing it before the gate fixes land would just cause
  churn.

---

## 3b. Build / ops papercuts (found deploying to a clean RHEL 9 EC2 box)

- **`make dgraph` fails confusingly when `bzip2` is missing.** The `jemalloc`
  target (`dgraph/Makefile:103`) does `tar xjf jemalloc.tar.bz2` then
  `cd jemalloc-5.3.1`. Without `bzip2` the extract fails *silently*, the `cd`
  fails, and `sudo make install` then runs in `/tmp/jemalloc-temp` — producing
  `No rule to make target 'install'`, which points nowhere near the real cause.
  Add `bzip2` to the documented build prerequisites, or use `tar xjf --checkpoint`
  / check the extract exit status and fail with a clear message.
- **The build needs a git checkout for the version string.** Building from an
  exported tree prints `fatal: not a git repository` and produces a binary with
  an empty version. Harmless but noisy; guard the `git describe` call.
- **`--profile_mode mutex` does not enable the live `/debug/pprof/mutex`
  endpoint** (returns `sampling period=0`). See the item in section 3.

## 4. Benchmarking / harness lessons (so they are not re-learned)

- **Dgraph returns HTTP 200 with an `{"errors":[...]}` body on rejection.** A
  load generator that checks only the status code will happily report
  hundreds of "successful" batches that did nothing. Cost a full invalid run.
- **Explicit uids require a lease first**
  (`curl "localhost:6080/assign?what=uids&num=N"`), otherwise every mutation
  fails with `Uid: [N] cannot be greater than lease: [0]`.
- **Cross-predicate lock contention needs multiple predicates per
  transaction, not multiple client threads.** The apply loop is serial across
  transactions, so client concurrency cannot produce it. A batch file containing
  a single predicate yields `P=1` and cannot exercise the convoy at all.
- **Sequential A/B on a laptop is invalid** — thermal drift produced a spurious
  43% "regression" on a code path that had not changed. Build two test binaries
  and interleave them.

---

## 5. Regression evidence (2026-07-26, 32-core EC2)

### Query-based differential — the strongest gate we have

A mixed-workload corpus (60 batches x 400 entities x 17 predicates, ~408k
triples) was ingested into four FRESH clusters and fingerprinted with 72 DQL
queries covering every predicate type, index tokenizer, reverse edge, count
index, language tag, a 40-entity full dump, and star-delete verification:

| config | binary | budget | result |
|---|---|---|---|
| `stock30` | pre-change (`ab0a49e77`) | 30 | reference |
| `new0` | all changes | 0 (legacy path) | **identical** |
| `new30` | all changes | 30 (prod default) | **identical** |
| `newauto` | all changes | -1 (auto) | **identical** |

Zero mutation errors in all four. The corpus exercises `ProcessSingle`,
`ProcessList`, `ProcessReverse` (hot AND spread targets), `ProcessCount`,
`InsertTokenizerIndexes` (exact/hash/term/fulltext/trigram/int/float/bool/day/geo),
`@upsert`, `@noconflict`, `@lang`, `[string]` lists, SET, DEL, and star-delete —
i.e. every branch the pipeline work touched, not just the benchmark shape.

Harness: `/data/harness/verify.sh`, corpus generator `gen_mixed.py`.

### Pre-existing test failures — NOT caused by this work

Verified by running the identical suite on `ab0a49e77` (pre-change tree) on the
same box:

- **`worker` package fails `-race` on BOTH trees.** Root cause is structural:
  `worker.Init()` (`worker/worker.go:48-52`) overwrites the package-global
  `limiter` and starts `go limiter.bleed()`, a goroutine that is never stopped.
  Every test calls `Init()`, so each new call races the previous test's still-live
  `bleed()`. The race detector then attributes the failure to whichever test
  happens to be running, which is why the failing set differs every run.
  Measured over 5 runs per tree: baseline and HEAD both fail, with overlapping
  and fluctuating sets (`TestLimiterDeadlock` fails 5/5 on both).
- **`types`: `TestParseTimeWithTZ`** writes the process-global `time.Local`
  (`types/scalar_types_test.go:121`) while an OpenCensus worker reads it via
  `time.Now()`. ~6/8 failures on a clean tree.
- **`posting` — the package this work actually changes — passes cleanly on both
  trees** (280s baseline, 286s HEAD, `-race`).

Fixing either would be a worthwhile separate change; neither blocks this work.

### Harness trap discovered here

**Dgraph's `/admin` export is ASYNCHRONOUS** — it returns
`"Export queued with ID ..."` immediately. Sleeping and then reading the export
directory samples partial files, which silently produces four "different"
databases that are actually just different flush points. Poll for completion, or
use queries (synchronous) as done here.

### Integration suite (`t/` runner, Docker) — 39 packages, zero failures

Run on the 32-core box against a `dgraph/dgraph:local` image built from this
tree and **verified by symbol inspection** to contain the changes
(`flushConflicts`=2, `maphash`=18) before trusting any result.

**39 packages `ok`, 0 package failures, 0 test failures**, including the ones
that matter most here:

| package | time | note |
|---|---|---|
| `systest/mutations-and-queries` | 50s | the mutation-heavy systest |
| `systest/vector` | 1484s | HNSW path, a known pipeline bug source |
| `worker` | 213s | passes cleanly under the integration harness |
| `systest/multi-tenancy`, `systest/plugin`, `acl`, … | — | ok |
| `posting` (rerun, `--pkg=posting`) | — | `T_EXIT=0` |

Setup needed on a clean box: Docker (`t/` demands it even for unit tests),
`gotestsum` (`AUTO_INSTALL=true make check-deps`), a git repo root, and
`make local-image`. `ack` is unavailable on RHEL 9 and blocks `check-deps`, but
the runner itself does not need it — invoke `go run .` in `t/` directly.

**Unexplained:** the first full-suite invocation exited 1 with no failing test
and a log ending mid-`posting`. Re-running that package alone gave `T_EXIT=0`.
The four `panic:` entries in the log are intentional — they come from
`TestRunAll_WithDgraphDirectives/panic_catcher`, whose own output says the panic
is expected.

Caveat worth stating: `TestRebuildTokIndex` appears to stall if you filter test
output with `grep | head` — badger emits enough INFO lines between `=== RUN` and
`--- PASS` to be truncated. It passes in 0.06s.
