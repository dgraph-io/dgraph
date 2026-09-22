# Predicate move tests

Integration tests for Zero-driven predicate moves on a real 1-zero, 2-alpha cluster (one group per
alpha) brought up via `dgraphtest`. Two tests share this package and its helpers
(`helpers_test.go`), selected by build tag:

| Test                      | Build tag      | Runs in CI | Duration                 |
| ------------------------- | -------------- | ---------- | ------------------------ |
| `TestCancelPredicateMove` | `integration2` | yes        | a few minutes            |
| `TestLargePredicateMove`  | `largemove`    | no         | tens of minutes to hours |

## Cancelling a move (`TestCancelPredicateMove`)

Exercises Zero's `/cancelMove` endpoint. The destination Alpha is paused (`docker pause`: the
process is frozen with its sockets intact) so the move stalls mid-stream and stays cancellable for
as long as the test needs, independent of data volume or host speed.

1. Loads a small payload into one predicate, pauses the destination Alpha, and starts a move.
2. Cancels it. The move must fail reporting the operator cancellation, the tablet must stay on the
   source group, and commits on the predicate must flow again (Zero aborts every commit touching a
   predicate while its move is in progress).
3. Unpauses the destination and retries. The move must complete with the data intact, proving the
   aborted attempt left nothing behind that a retry cannot clean up.

```bash
make install       # builds the binary dgraphtest mounts into containers
make image-local   # dgraphtest boots containers from dgraph/dgraph:local; build it if absent
go test -v --tags=integration2 -run TestCancelPredicateMove ./systest/predicate-move/
```

## Large move timeout and backoff (`TestLargePredicateMove`)

A long-running test for the size-aware predicate move timeout and rebalancer backoff
(dgraph-io/dgraph#9792, fixes #9784). It is excluded from CI via the `largemove` build tag, which no
workflow compiles: the test loads several GiB of data and can run from tens of minutes to hours
depending on the data size and host.

1. Loads `MOVE_TEST_GB` GiB (default 8) of incompressible 64KiB string values into one predicate.
2. Waits until Zero reports the tablet size, then triggers a move and kills the destination Alpha
   mid-stream. Asserts the move fails and Zero records the rebalancer backoff ("Skipping automatic
   rebalancing of this tablet").
3. Restarts the destination and retries. Asserts the move completes, every move announcement in
   Zero's log carried a size-scaled timeout above the 2h floor, and the row count is intact on the
   destination group.

The auto-rebalancer cannot interfere: with a single dominant tablet, `chooseTablet` only picks
tablets no larger than half the group size difference, which the payload tablet always exceeds.

```bash
make install
make image-local
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
