# Large predicate move test

A long-running integration test for the size-aware predicate move timeout and rebalancer backoff
(dgraph-io/dgraph#9792, fixes #9784). It is excluded from CI via the `largemove` build tag, which no
workflow compiles: the test loads several GiB of data and can run from tens of minutes to hours
depending on the data size and host.

## What it does

1. Brings up a 1-zero, 2-alpha cluster (one group per alpha) via `dgraphtest` and loads
   `MOVE_TEST_GB` GiB (default 8) of incompressible 64KiB string values into one predicate.
2. Waits until Zero reports the tablet size, then triggers a move and kills the destination Alpha
   mid-stream. Asserts the move fails and Zero records the rebalancer backoff ("Skipping automatic
   rebalancing of this tablet").
3. Restarts the destination and retries. Asserts the move completes, every move announcement in
   Zero's log carried a size-scaled timeout above the 2h floor, and the row count is intact on the
   destination group.

The auto-rebalancer cannot interfere: with a single dominant tablet, `chooseTablet` only picks
tablets no larger than half the group size difference, which the payload tablet always exceeds.

## Running

```bash
make install   # builds the Linux binary dgraphtest mounts into containers (required on macOS)
go test -v -timeout=8h --tags=largemove -run TestLargePredicateMove ./systest/predicate-move/
```

Do not run this through `make test TAGS=largemove`: the Makefile does not pass a `-timeout`, so the
Go default of 10m kills the test.

Knobs and requirements:

- `MOVE_TEST_GB` (default 8, minimum 3): value data loaded before the move. This is a floor, not the
  final size: the test measures the host's ingest rate and tops up automatically until the estimated
  move duration outlasts the 75s kill point (roughly 17 GiB on a fast laptop NVMe, less on slower
  disks).
- Disk: budget roughly 5x the final loaded size inside the Docker Desktop VM (source + destination
  copies, Raft WAL, and compaction headroom); on a fast host assume ~80-100 GB.
- Time: about 30 minutes at the default size on an Apple Silicon laptop, dominated by the load and
  the tablet-size reporting interval (Alphas recompute tablet sizes on a periodic ticker).
