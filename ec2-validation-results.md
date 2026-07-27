# EC2 validation — parallel reverse write (32 physical cores)

Validation of the `ProcessReverse` parallel-write change against a real Alpha
under sustained multi-threaded ingest, on hardware large enough to exercise the
parallelism (the prior laptop measurements had only 4 physical cores).

## Setup

| | |
|---|---|
| instance | `i4i.16xlarge` — 64 vCPU / **32 physical cores**, 495 GB RAM, local NVMe |
| CPU | Intel Xeon Platinum 8375C @ 2.90 GHz |
| OS / Go | RHEL 9.8, go1.26.3 |
| schema | 25 × `hcN: [uid] @reverse .` |
| batch | 20,000 triples/txn spanning **all 25 predicates**, 800 distinct reverse targets each |
| corpus | 600 distinct batches (12M unique triples), each thread owns a disjoint block |
| load | 64 threads, `commitNow=true`, 300 s per run |
| metric | `badger_write_bytes_user` delta / elapsed (the production "Badger Write Throughput") |

Binaries built on the instance from the same tree and verified to differ by
disassembly: stock `ProcessReverse` references only `(*Txn).AddDelta`; patched
references both `AddDeltaConcurrent` and the legacy `AddDelta`.

> **Flag names below are the pre-simplification ones.** Every run recorded here
> predates the `intra-mutation-*` rename, and the tables are left verbatim so
> they still describe what was actually executed. To reproduce:
>
> | as run | today |
> |---|---|
> | `mutations-pipeline-threshold=1` | `intra-mutation-min-edges=1` |
> | `mutations-pipeline-goroutines=30` | `intra-mutation-parallelism=30` |
> | `mutations-pipeline-goroutines=0` | `intra-mutation-parallelism=off` |
> | `mutations-pipeline-goroutines=-1` (auto) | `intra-mutation-parallelism=auto` |
> | `mutations-pipeline-goroutines-fraction=F` | `intra-mutation-parallelism=Fx` |
> | `mutations-pipeline-min-edges-per-worker=N` | `intra-mutation-edges-per-worker=N` |
>
> One behavioral caveat when re-running: the edges-per-worker cap now applies to
> fixed counts too, not only to auto. At the 20k-edge batch used here the cap is
> 78 and the fixed budget was 30, so the cap never binds and these numbers stand
> unchanged.

## Results

### budget = 30 (the shipped production default), 2 paired runs each

| build | triples/s | MB/s |
|---|---|---|
| stock | 97,030 / 96,833 | 9.64 / 9.61 |
| patched | 105,272 / 106,313 | 10.58 / 10.57 |
| **delta** | **+9.1%** | **+9.9%** |

Spread within each arm is ≤1%, and the arms do not overlap.

### budget = -1 (auto; resolves to 64 on this box)

| build | triples/s | MB/s |
|---|---|---|
| stock | 97,457 | 9.68 |
| patched | 113,081 | 11.29 |
| **delta** | **+16.0%** | **+16.6%** |

**Best config vs production default** (patched+auto vs stock+30): **+16.7%
triples/s, +17.2% MB/s**.

Note: auto does **not** help stock (97,457 vs 96,932 = +0.5%). It only pays once
the reverse write can actually consume the extra workers. The "just raise the
budget" free win hypothesised earlier is **not** supported.

### Adding the forward-write gate fix (`612ae8cd5`)

`ProcessList`/`ProcessSingle` had the identical gate coupling. Fixing it too:

| build | budget=30 | auto |
|---|---|---|
| stock | 9.63 | 9.68 |
| + reverse (`4a02d8eeb`) | 10.58 | 11.29 |
| **+ forward (`612ae8cd5`)** | **11.41 / 11.47** | **11.45 / 11.31** |

**+18.8% over stock at the production default.** Note the budget stops mattering
once the lock-free store is reachable at any grant — auto and 30 now agree to
within noise. Tuning the worker count is no longer load-bearing for this
workload, which removes a whole class of misconfiguration — and is part of why
the four tuning flags were later collapsed into `intra-mutation-parallelism`.

### Adding conflict-key batching (`5b69ac2b4`)

| build | budget=30 MB/s | triples/s |
|---|---|---|
| stock | 9.63 | — |
| + reverse (`4a02d8eeb`) | 10.58 | — |
| + forward (`612ae8cd5`) | 11.44 | 113,001 |
| **+ conflict batching (`5b69ac2b4`)** | **12.45** | **123,732** |

**+29.3% cumulative over stock.** The conflict-key path vanishes from the
goroutine profile (68.5% → 0.0% of pipeline goroutines; blocked 23.0% → 0.0%),
and total pipeline goroutine observations fall 518 → 216. New top blocker:
`Deltas.AddToDeltas` (57.6%) — shard contention on the delta map, `NumShards = 30`.
See `conflict-key-batching-plan.md`.

## Where the time goes

Goroutine dumps (100 samples/run, `?debug=2`), bucketed by pipeline pass and
blocking site, budget=30:

| | stock | + reverse | + forward |
|---|---|---|---|
| parked pipeline goroutines | 701 | 566 | 643 |
| in `ProcessReverse` | **61.2%** | 35.7–39.9% | 58.8% |
| in `ProcessList/Single` | 34.2% | **56.2–60.1%** | 37.6% |
| blocked on global `cache.Lock` | **86.4%** | 41.4–52.5% | **0% (absent)** |
| blocked on `txn.Lock` (`addConflictKeyWithUid`) | **0.4%** | 30.7–31.4% | **71.2%** |
| on sharded `AddDeltaConcurrent` | 0.7% | 3.2–10.0% | 6.7% |
| **runnable** (doing work, not blocked) | ~13% | ~17% | **57.9%** |

The global cache lock is **eliminated** from the blocking profile. The
conflict-key lock goes 0.4% → 31% → **71.2%**: a textbook bottleneck migration,
where each fix exposes the next. Goroutines actually running rise from ~13% to
**58%**.

Two conclusions:

1. **The reverse-write convoy is real and is removed.** Stock parks 61% of
   pipeline goroutines in `ProcessReverse`, 86% of them on one global mutex —
   this reproduces the production goroutine dump that motivated the work. After
   the change reverse drops to ~37% and is no longer the top site.
2. **The bottleneck moves twice.** `ProcessList/Single` (which still gates its
   lock-free store on `workers > 1`) becomes #1 at ~58%, and
   `addConflictKeyWithUid` goes from 0.4% → **31%**.

### Core-count sensitivity — why laptop numbers misled

`addConflictKeyWithUid` measured **2.7%** on a 4-physical-core laptop and
**31%** here. Contention on a single mutex scales with contender count; small
boxes cannot see it. Any ranking of this work must be done on real hardware.

### CPU utilization

Mean CPU across runs: **6–8% of 64 vCPU** (~4–5 cores busy), patched slightly
higher than stock. The box is nowhere near CPU-bound. This is the architectural
ceiling described in `CLAUDE.md`: `processApplyCh` applies one Raft entry at a
time, so a single Alpha cannot use 64 cores regardless of intra-transaction
parallelism. Top CPU consumers (pprof, patched): `sync/atomic.(*Int32).Add`
7.2%, `runtime.memmove` 5.7%, `worker.(*groupi).Tablet` 6.3% cum,
`x.(*SafeMutex).RUnlock` 3.0%.

## Honest caveats

- Synthetic schema is **100% `[uid] @reverse`** with no indexes or counts, which
  overstates the reverse share versus a real mixed schema. The real-workload
  gain will be smaller in proportion to the `@reverse` fraction.
- 300 s runs, not the 30-minute production cycles.
- 32 physical cores, not 148.
- Each thread re-writes its own block of batches, so later writes hit existing
  posting lists — realistic for upserts, not for pure first-time ingest.

## Runs that were discarded (and why)

- **First A/B (laptop):** a stale Alpha held `:8080`; the "patched" run profiled
  the *stock* process. Detected because the patched binary's `AddDelta` is at
  `index.go:684` but stacks showed `608`. Harness now asserts the serving pid.
- **First EC2 matrix:** the load generator strided through batch files
  (`i += threads`), letting threads drift onto the same file and write identical
  `(src,tgt)` pairs concurrently → 1.47M transaction aborts. Fixed by giving each
  thread a disjoint contiguous block.
- **One `stock_b30` run at 54,324 triples/s** was an outlier (61 aborts); two
  clean re-runs gave 97,030 and 96,833. The headline numbers use the re-runs.
