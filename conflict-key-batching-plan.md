# Conflict-key batching — plan, decisions, and measured results

Durable record of the third change in the intra-transaction apply-path series.
Commit `5b69ac2b4`, on `rahst12/hybrid-pipeline-reverse-parallel`.

---

## Where this sits

| commit | change | budget=30 MB/s |
|---|---|---|
| (stock) | branch base | 9.63 |
| `4a02d8eeb` | parallel reverse write, lock-free store at any grant | 10.58 (+9.9%) |
| `612ae8cd5` | lock-free forward write at a one-worker grant | 11.44 (+18.8%) |
| **`5b69ac2b4`** | **batch conflict-key emission** | **12.45 (+29.3%)** |

All measured on `i4i.16xlarge` (32 physical cores / 64 vCPU, 495 GB, local NVMe),
25 `[uid] @reverse` predicates, 20k triples/txn, 64 writer threads, 300 s runs,
`badger_write_bytes_user` — the same metric used for the Preview1 vs
Preview1-Hybrid comparison in discussion #9727.

---

## The problem

After the first two commits removed the `txn.cache.Lock()` convoy, `Txn.conflicts`
(`posting/oracle.go:52`, a `map[uint64]struct{}` under `Txn`'s embedded
`sync.Mutex`) was the only remaining mutex contention: **25% of pipeline
goroutines blocked on it, 46% running inside `addConflictKeyWithUid`**.

The decisive evidence was the line-level CPU profile — the map insert was never
the problem:

| line | cum | |
|---|---|---|
| `txn.Lock()` | **8.75s** | 49% |
| deferred `Unlock` | **5.33s** | 31% |
| map insert | 2.71s | 15% |
| `Fingerprint64` | 0.27s | 1.5%, needlessly inside the lock |

**~80% acquire/release, ~15% real work.** The cost was the *number of
acquisitions* — one per key, from every worker in every predicate.

## Decision: batch, don't shard

Sharding `Txn.conflicts` was the obvious move and was **rejected on the
arithmetic**. Batching removes the 14.08s of acquire/release (79% of the
function). What is left for sharding to attack is the 2.71s of inserts — about
**1% of total CPU** — in exchange for changing `Txn.conflicts`' type, mechanically
editing nine test sites, rewriting `FillContext`, and either raising `NumShards`
from 30 (shared with `posting/lists.go` deltas and indexMap) or maintaining a
second striped container.

**Revisit trigger:** shard only if `flushConflicts` self-CPU exceeds 3% or
mutex-BLOCKED exceeds 5%. Post-change both are **0.0%**, so this stays closed.

### Why it is safe

- The embedded mutex on `Txn` protects `conflicts` and **nothing else**
  (`lastUpdate` is guarded by `oracle`'s `SafeMutex`).
- The only reader is `FillContext`, which runs `x.Unique` — sorting and deduping —
  so `ctx.Keys` was **already** order-independent. Zero's `hasConflict` is a pure
  existential over the set. No consumer is order-, index-, or length-sensitive.
- Writers join (L2 `wg.Wait()`, then L1 `eg.Wait()`) strictly before
  `FillContext` runs via `worker/mutation.go`.

## Three traps, and how each was handled

1. **Eager key expansion.** Buffers hold `uint64`, never `key []byte`. Every call
   site reuses one scratch key buffer across iterations, so deferring the
   fingerprint to flush time would hash the *last* uid's bytes for every entry —
   wrong conflict keys, no panic, invisible to `-race`.
2. **Defer ordering in `InsertTokenizerIndexes`.** It takes `txn.Lock()` while
   holding `txn.cache.Lock()`. The flush defer is registered **above**
   `cache.Lock()` so LIFO runs it after `cache.Unlock()`. Registering it below
   compiles, passes every byte-identical test, and silently preserves the nested
   lock this change exists to remove.
3. **`break`, not `return`, on worker error.** Workers now have post-loop work
   (storing their buffer). The pre-existing error path never rolled back emitted
   keys, so dropping them would *change* the set — and a subset risks a lost
   update while a superset only costs a spurious abort.

## Results

**Throughput** (2 runs per arm, non-overlapping):

| | triples/s | MB/s |
|---|---|---|
| before (`612ae8cd5`) | 113,916 / 112,086 | 11.40 / 11.47 |
| after (`5b69ac2b4`) | 123,399 / 124,065 | 12.41 / 12.49 |
| | **+9.5%** | **+8.8%** |

**Goroutine profile — the primary acceptance criterion:**

| | before | after |
|---|---|---|
| pipeline goroutine observations | 518 | **216** (−58%) |
| in the conflict-key path | **68.5%** | **0.0%** |
| blocked on the conflict mutex | **23.0%** | **0.0%** |
| CPU utilisation | 7.9% | 9.6% |

The conflict-key path disappears from the sampled profile entirely.

**Correctness signals:** `batches_fail = 0` on all four runs, both binaries —
no change in client-visible aborts. (`dgraph_txn_aborts_total` is not exposed by
this build, so the zero-failure parity plus the golden test carries that check.)

## Verification

- `TestFillContextKeysGolden` (new) — pins the exact 205-key `ctx.Keys` sent to
  Zero against a golden captured at `612ae8cd5`, and asserts the keys are
  `startTs`-independent. **This closed a real gap:** the six existing
  byte-identical tests only compare budget 0 vs N, so a regression changing the
  serial and parallel paths *the same way* would pass all of them, and nothing
  asserted `FillContext`'s output at all.
- Six existing byte-identical tests (conflict-key set equality across budgets).
- Full `posting` package green under `-race` (354 s).
- `TestPipelineReverseListCountMultiPred` is an intermittent pre-existing flake:
  0/6 failures on both this commit and `612ae8cd5`.

## What comes next

The new top blocker, named by the post-change profile: **`Deltas.AddToDeltas`**
at 57.6% of a much smaller blocked population — shard contention on the delta
map, where `NumShards = 30` (`types/sharded_map.go:18`) is thin for 64 vCPU.
Raising it, or improving `getShardIndex` (which also heap-allocates on `string`
keys — ~13% of batch allocations), is the cheap next step.

**The standing constraint:** CPU is still only **9.6% of 64 vCPU**.
`processApplyCh` applies one Raft entry at a time, so a single Alpha cannot
saturate a large box no matter how much intra-transaction contention is removed.
Further material gains require cross-transaction concurrency in the apply loop —
a much larger change than any of these three.
