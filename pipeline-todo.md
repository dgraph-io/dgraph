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

## 1. Simplify the flags — DONE

Four knobs became **three**, all live at every setting, renamed to say what they
scope: `intra-mutation-*`.

| was | now | default |
|---|---|---|
| `mutations-pipeline-threshold` | `intra-mutation-min-edges` | 1 |
| `mutations-pipeline-goroutines` + `-goroutines-fraction` | `intra-mutation-parallelism` (`off｜auto｜N｜Fx`) | `auto` |
| `mutations-pipeline-min-edges-per-worker` | `intra-mutation-edges-per-worker` | 256 |

What the fix actually was, beyond the count:

- **The two "inert unless `goroutines == -1`" flags were not deleted — the
  conditional was.** `edges-per-worker` now caps every sizing mode, so "do not
  spin N workers for a handful of edges" holds for a fixed count as much as a
  derived one. That is what removed the inertness; deleting the flags would have
  hidden the problem instead.
- **`goroutines` and `-fraction` were the same axis in two notations**, not two
  settings. One was a worker count, the other a multiple of GOMAXPROCS — and the
  first doubled as the tag selecting which of the two was read. They merged into
  one value taking `N` or `Fx`. The `-1` sentinel is gone; `auto` == `1x`.
- **`auto` is now the default**, so the pool tracks the box rather than a magic
  30 that was wrong on both an 8-core and a 148-core machine.
- **`edges-per-worker` deliberately stayed a flag.** 256 was adopted by analogy
  to `DivideAndRule` and never measured, and it is the *binding* term on a large
  box: at 256 a 20k-edge mutation caps at **78 workers however many cores
  exist**. Freezing an unmeasured number that governs large-box behavior was the
  wrong thing to make permanent. Revisit after the 148-core sweep in §3.
- **The proposed fifth flag was not added.** `reverseParallelMinTargets` stays a
  `const` until §3 calibrates it — shipping an uncalibrated knob works against
  this cleanup.

### Two things the trace turned up that were not in the original write-up

1. **`goroutines=1` was silently identical to `goroutines=0`**
   (`allocateWorkers` returned nil for `budget < 2`), while **`goroutines=2`
   with 25 predicates gave every predicate a lock-free 1-worker grant** via the
   `P >= budget` short-circuit. The cheapest good setting looked inert.
   Confirmed against the code: at 25 equal predicates, budget 2 → `{1:25}`,
   30 → `{1:20 2:5}`, 50 (2×P) → `{2:25}`, 64 → `{2:11 3:14}`, 78 → `{3:22 4:3}`.
2. **`-fraction` saturated.** Because the result was `min(machineCap, workCap)`,
   raising fraction stopped changing anything above ≈1.22 on the 64-vCPU /
   20k-edge shape. The help text's advice to "raise above 1.0 to oversubscribe,
   peak near 2-3x cores" was therefore **unfollowable at that batch size** —
   reaching 128 needs ≥32,768 edges. That advice came from the 8-core box, where
   `workCap` was nowhere near binding. Corollary: on the 148-core box earmarked
   for the §3 sweep, the old auto would derive **78, not 148**.

   Both are pinned by `TestResolveWorkers` / `TestParallelismSizing` so they
   cannot silently regress.

### 1a. `WorkerOptions.String()` hid the pipeline config — DONE
`x/config.go` hand-formats the struct and stopped at `Audit:%v}`, so the startup
`glog.Infof("x.WorkerConfig: %+v")` never showed the effective settings. This
cost real time during live debugging — the log could not confirm whether the
budget was 30 or 0. The three `IntraMutation*` fields are now printed.

### 1b. Observability — DONE
- `glog.V(2)` line after `allocateWorkers` reporting resolved workers, **which
  term bound** (`boundBy=parallelism` vs `edges-per-worker` — the least visible
  part, given the saturation above), predicate count, edges, and the grant
  histogram.
- `intra_mutation_no_fanout_total` counts batches whose grant was all-1s, i.e.
  the silent-degradation case. Explicitly *not* incremented when parallelism is
  `off`, which is a deliberate operator choice rather than a surprise.

---

## 1c. NEW FINDING — the pipeline and the legacy path disagree, and always have

Surfaced while decoupling the lock-free store from the worker grant. That change
deleted the in-pipeline `workers == 0` branches, which were an in-pipeline
replica of legacy semantics and the baseline `TestSchemaMatrixByteIdentical`
compared against. Replacing that baseline with the **real** legacy path (a serial
`runMutation` loop — what `intra-mutation-min-edges=0` actually selects) made the
matrix compare pipeline-vs-legacy for the first time, and it does not match.

**All three divergences were measured on the pre-change tree** (`308ea6b35`, in a
clean worktree, pipeline at the old disabled budget) and are therefore unrelated
to the flag work:

| schema | measurement | direction |
|---|---|---|
| `uid @reverse` | legacy 600 conflict keys, pipeline **1200** | pipeline superset (safe: extra aborts) |
| `string @index(fulltext) @lang` | legacy 3000, pipeline 4800, **600 dropped that legacy emits** | pipeline drops keys — **the unsafe direction** |
| `[uid] @reverse @count` | legacy 726 state keys, pipeline 607 | 119 legacy-only keys, **all empty**, 0 differing values |

Committed state otherwise matches exactly across every schema and tokenizer in
the matrix; the count difference is emptied buckets legacy leaves behind and the
pipeline never creates, so the pipeline's output is the cleaner one.

**The `@lang` case deserves a look on its own.** Dropping a conflict key that the
legacy path emits is the direction that risks a lost update, and the pipeline has
been the production default (`threshold=1`) all along. It is out of scope for the
flag simplification — it is neither caused nor fixed by it — but it is the most
significant thing this exercise turned up. Whether the 600 dropped keys are
genuinely redundant (superseded by the 2400 extra) or a real gap needs the
conflict-key derivation for `@lang` postings traced against `GetConflictKey`.

The matrix test now asserts what is actually true and says why in-line: non-empty
committed state identical to legacy, and conflict keys identical **across
parallelism settings within the pipeline** — the guarantee this branch's work
must preserve, and what the production abort-count parity rests on.

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

### Production-replica corpus benchmark (ext-pointer-tech-collab data generator)

The synthetic corpus used above is 100% `[uid] @reverse` with no upserts, which
overstates the reverse share. Re-ran against the three-stage generator from
`ext-pointer-tech-collab/data-generator`, which replicates the NiFi/dgraph4j
production pipeline: **30 `@reverse` and 20 `@upsert` predicates**, 1.67M seeded
nodes, hot-node reverse fan-out (Zipf `--hot-skew`), `@upsert` re-asserts and
hours-later update waves. 532 transaction-sized files (20k nquads each, 600 MB).

Both arms restore the **same seeded snapshot**, so they differ only in binary.
Budget=30 (shipped default), 16 client threads, 2 runs per arm:

| run | MB/s | elapsed | aborts | nquads committed | nquads/s |
|---|---|---|---|---|---|
| stock r1 | 1.47 | 129.2s | 1855 | 5,692,627 | 44,900 |
| stock r2 | 1.48 | 129.5s | 1871 | 5,746,044 | 44,498 |
| **shard r1** | **1.74** | 110.8s | 1850 | 5,709,947 | **52,612** |
| **shard r2** | **1.73** | 110.1s | 1857 | 5,625,354 | **52,101** |

**+17.6% MB/s, +17.1% nquads/s, 14.6% faster wall clock**, reproducible to ~1%.

**Abort counts are identical (~1,860 both arms)** — the conflict-key semantics
are unchanged under real `@upsert` contention, which is the strongest available
correctness signal for the conflict-key batching work.

Two things this reveals that the synthetic corpus hid:

1. **Absolute throughput is far lower (1.5 MB/s vs 13 MB/s)** because this
   workload is *abort*-dominated: only ~283 of 532 files commit, the rest give up
   after 6 retries. The ceiling here is Zero's conflict arbitration, not the
   apply path — which is exactly the contention the production client's
   Caffeine lock layer exists to avoid.
2. The relative gain (+17.6%) is **half** the synthetic figure (+36.8%), which is
   the expected dilution from a realistic predicate mix. Quote the 17.6% number
   for production expectations, not the 36.8%.

Reproduce: `/data/harness/prodbench.sh <binary> <tag> <budget> <threads>` with
`/data/gen` (generator) and `/data/seed_snapshot` (seeded state) on the EC2 box.
Generator needs Python 3.10+ (`dataclass(slots=True)`) — RHEL 9 ships 3.9.

#### Client-thread sweep — the abort storm dominates, and fewer threads is faster

Same production-replica corpus, budget=30, varying client threads (emulating a
lock-cache-protected client, which serialises conflicting writes rather than
letting them abort):

| threads | stock MB/s | shard MB/s | speedup | stock nquads/s | shard nquads/s | files committed | aborts |
|---:|---:|---:|---:|---:|---:|---:|---:|
| 2 | 5.30 | **5.96** | 1.12x | 143,143 | **160,825** | 502-520 / 532 | ~380 |
| 4 | 3.62 | **4.09** | 1.13x | 100,348 | **113,459** | 454 / 532 | ~940 |
| 8 | 2.31 | **2.67** | 1.16x | 65,918 | **76,008** | ~368 / 532 | ~1,470 |
| 16 | 1.48 | **1.74** | 1.18x | 44,699 | 52,357 | ~283 / 532 | ~1,860 |

**Throughput more than triples as client threads DROP from 16 to 2** (1.48 ->
5.30 MB/s on stock; 44.7k -> 143k nquads/s). Aborts fall 1,860 -> 380 and
committed files rise 283 -> 502 of 532. On this workload the Alpha is not the
constraint at all — Zero's conflict arbitration over `@upsert` re-asserts is,
and every extra client thread buys more aborted work than committed work.

Two consequences worth acting on:

1. **A production ingest tuned for more threads is likely losing throughput to
   aborts, not gaining it.** This is precisely what the client-side node-UID lock
   cache exists to prevent — these numbers quantify what it is worth. Anyone
   without it should test *lower* concurrency before adding more.
2. **The apply-path gain is consistent everywhere (1.12x-1.18x)** and grows
   mildly with contention. It is not an artifact of the abort storm — at 2
   threads, where 502/532 files commit and aborts are minimal, it is still
   **+12%** (and +12.4% on nquads/s).

Abort counts track each other across every thread count (380/379, 945/931,
1465/1479, 1855/1857) — conflict semantics unchanged, as intended.
