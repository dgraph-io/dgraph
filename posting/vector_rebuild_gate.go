/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package posting

import (
	"context"
	"encoding/json"
	"math"
	"sync"
	"time"

	"github.com/golang/glog"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/tok"
	"github.com/dgraph-io/dgraph/v25/tok/hnsw"
	tokIndex "github.com/dgraph-io/dgraph/v25/tok/index"
	"github.com/dgraph-io/dgraph/v25/types"
	"github.com/dgraph-io/dgraph/v25/x"
)

// A vector index rebuild runs in the background while mutations keep applying
// (worker's runSchemaMutation launches it with `go buildIndexes`). The builder
// commits every graph key at the alter's startTs, so a concurrent mutation —
// which commits above startTs — wins last-writer-wins over the freshly built
// graph. Worst case it applies while the graph is empty and installs itself as
// the sole entry point (createEntryAndStartNodes' data == nil branch),
// orphaning everything the builder writes.
//
// The capture gate below makes the builder the sole writer to a predicate's
// vector keyspace for the duration of its build: while capture is active,
// live index writes for that predicate are suppressed and only (uid, op) is
// recorded; the build drains the captured uids into the finished index before
// deactivating. Base-data writes are untouched — transactions succeed exactly
// as before, only the index maintenance is deferred.
//
// The map holds uids, not payloads: replay re-reads the authoritative value
// from the data posting list, so repeated updates to one uid dedupe for free
// and memory is bounded by the predicate's cardinality. The map is not
// persisted: a crash mid-build either abandons the alter (the new schema is
// only written to disk after a successful build) or replays the alter from
// the raft WAL together with the same mutations, which then re-capture.

type vectorPendingOp byte

const (
	vectorPendingSet vectorPendingOp = iota
	vectorPendingDel
)

// vectorPendingMutation records the last captured op for a uid together
// with the startTs of the transaction that carried it. The startTs lets the
// drain distinguish "base value not committed yet" (wait and retry) from
// "transaction resolved without leaving a value" (aborted or deleted: skip):
// capture happens at apply time, but the commit delta from zero arrives
// later, and a drain that outruns the commit would silently drop the uid.
type vectorPendingMutation struct {
	op      vectorPendingOp
	startTs uint64
}

type vectorCaptureState struct {
	mu sync.Mutex
	// pending maps uid -> last captured mutation. nil means the final drain
	// has completed and the gate is closed even if the registry entry has
	// not been removed yet.
	pending map[uint64]vectorPendingMutation
}

var vectorCapture = struct {
	sync.RWMutex
	m map[string]*vectorCaptureState
}{m: make(map[string]*vectorCaptureState)}

// StartVectorRebuildCapture activates the capture gate for attr. It must run
// in the raft apply path, before the alter's proposal application returns, so
// that no later log entry can apply before the gate is up. Idempotent.
func StartVectorRebuildCapture(attr string) {
	vectorCapture.Lock()
	defer vectorCapture.Unlock()
	if _, ok := vectorCapture.m[attr]; !ok {
		vectorCapture.m[attr] = &vectorCaptureState{
			pending: make(map[uint64]vectorPendingMutation),
		}
	}
}

// FinishVectorRebuildCapture removes attr's capture gate. Idempotent. The
// successful build path calls it after draining; the worker also calls it on
// build failure, where the captured map is discarded together with the
// aborted alter (the rolled-back schema has no index to replay into).
func FinishVectorRebuildCapture(attr string) {
	vectorCapture.Lock()
	defer vectorCapture.Unlock()
	delete(vectorCapture.m, attr)
}

// VectorRebuildCaptureActive reports whether a capture gate exists for attr.
func VectorRebuildCaptureActive(attr string) bool {
	vectorCapture.RLock()
	defer vectorCapture.RUnlock()
	_, ok := vectorCapture.m[attr]
	return ok
}

// VectorRebuildPendingCount returns the number of uids waiting for replay on
// attr's gate. Exposed for tests and metrics.
func VectorRebuildPendingCount(attr string) int {
	st := vectorCaptureStateFor(attr)
	if st == nil {
		return 0
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	return len(st.pending)
}

func vectorCaptureStateFor(attr string) *vectorCaptureState {
	vectorCapture.RLock()
	defer vectorCapture.RUnlock()
	return vectorCapture.m[attr]
}

// captureVectorMutation records a live index write against an in-flight
// rebuild of attr and reports true when the caller must suppress the write.
// False means no build is in flight (or its final drain has completed) and
// the caller proceeds with a normal live index write.
func captureVectorMutation(attr string, uid uint64, op pb.DirectedEdge_Op, startTs uint64) bool {
	st := vectorCaptureStateFor(attr)
	if st == nil {
		return false
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.pending == nil {
		return false
	}
	pm := vectorPendingMutation{op: vectorPendingSet, startTs: startTs}
	if op == pb.DirectedEdge_DEL {
		pm.op = vectorPendingDel
	}
	st.pending[uid] = pm
	return true
}

// swapVectorPending atomically hands the current pending map to the drain
// loop, leaving a fresh map so mutations that land during replay are captured
// for the next round. Returns nil when no gate is active.
func swapVectorPending(attr string) map[uint64]vectorPendingMutation {
	st := vectorCaptureStateFor(attr)
	if st == nil {
		return nil
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.pending == nil {
		return nil
	}
	out := st.pending
	st.pending = make(map[uint64]vectorPendingMutation)
	return out
}

// The rebuild's own writes must pass the gate: the builder and the drain are
// exactly the writers the gate exists to protect. They mark their contexts so
// the capture check can tell them apart from live mutations — an explicit
// signal, deliberately not the accident that today's builder happens to use
// BuildInsert instead of Insert.

type vectorRebuildCtxKey struct{}

// WithVectorRebuildContext marks ctx as belonging to an index rebuild.
func WithVectorRebuildContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, vectorRebuildCtxKey{}, true)
}

func isVectorRebuildContext(ctx context.Context) bool {
	v, _ := ctx.Value(vectorRebuildCtxKey{}).(bool)
	return v
}

// drainVectorRebuildCapture replays the uids captured while attr's build ran
// into the finished index, then closes the gate. Runs at the end of a
// successful build, after the graph is durable (the builder flushed).
//
// Each captured uid replays in its own transaction at a strictly increasing
// timestamp (StartTs+1, StartTs+2, …), mirroring the live path's
// one-insert-per-txn shape. Both properties are load-bearing:
//
//   - Above StartTs: a replay write at StartTs itself would duplicate the
//     builder's (key, version) and badger resolves such duplicates
//     arbitrarily — neighbor-link updates can silently lose to the builder's
//     blobs, orphaning the replayed vector.
//   - Strictly increasing, one txn per uid: successive inserts into one
//     shared transaction do not reliably see each other's uncommitted
//     neighbor-row updates, so later inserts overwrite earlier ones' inbound
//     links (observed: two replayed mutual nearest neighbors, neither
//     linking the other). Distinct versions make every insert read its
//     predecessors' committed rows.
//
// The timestamp range cannot collide with live writes: each replayed uid
// stems from at least one captured mutation, and every captured mutation
// consumed two oracle ticks above StartTs (its own startTs and commitTs), so
// the oracle is already past StartTs+2N when the gate comes down — any
// post-drain live index write commits above that. While the gate is up, the
// drain is the sole writer above StartTs on these keys by construction.
//
// Replay goes through the registered long-lived index instance (the one live
// mutations use) — the builder's own instance is torn down by EndBuild and
// cannot serve live-style inserts.
func drainVectorRebuildCapture(ctx context.Context, rb *IndexRebuild,
	spec *tok.FactoryCreateSpec) error {
	attr := rb.Attr
	st := vectorCaptureStateFor(attr)
	if st == nil {
		return nil
	}

	nextReplayTs := rb.StartTs
	// replayOne indexes one captured uid. retry=true means the uid's base
	// transaction has not resolved yet (commit delta still in flight) and the
	// caller must try again later.
	replayOne := func(uid uint64, pm vectorPendingMutation) (retry bool, rerr error) {
		// For a SET, resolve the committed value BEFORE allocating a replay
		// version: retries must not consume versions, or the write versions
		// inflate past the collision-free StartTs+N budget (and past the
		// readers' snapshots).
		var inVec []float32
		if pm.op == vectorPendingSet {
			pl, err := GetNoStore(x.DataKey(attr, uid), math.MaxUint64)
			if err != nil {
				return false, err
			}
			val, err := pl.Value(math.MaxUint64)
			if err != nil || val.Value == nil || val.Tid != types.VFloatID {
				// No committed value yet. Capture happens at apply time,
				// before the commit delta arrives, so the transaction may
				// simply not have resolved: the caller retries until the
				// oracle moves past its startTs, then allows a short grace
				// (the delta resolution and the durable write are not
				// atomic, and the memory layer may still serve the empty
				// list cached by an earlier attempt — drop caches so the
				// next attempt reads fresh) before treating it as aborted.
				ResetCache()
				return true, nil
			}
			inVec = types.BytesAsFloatArray(val.Value.([]byte))
		}
		nextReplayTs++
		ts := nextReplayTs
		// Read view and write version are deliberately split. The captured
		// mutation's base value committed ABOVE StartTs (it arrived
		// mid-build), so the replay must READ at a timestamp that sees it —
		// otherwise every scoring read of this uid's vector inside the
		// insert returns "no value", the uid scores as garbage, and the
		// capped neighbor-row merges truncate it out of every row: zero
		// back-links, orphaned vector (observed on monolithic, whose rows
		// sit at the efConstruction cap; partitioned's small cluster rows
		// masked it). Writes still commit at StartTs+i, which stays
		// collision-free while the gate is up.
		readTs := Oracle().MaxAssigned()
		if readTs < ts {
			readTs = ts
		}
		txn := NewTxn(readTs)
		tc := hnsw.NewTxnCache(NewViTxn(txn), readTs)
		// Resolve the index instance per replay, exactly like the live
		// mutation path does per insert: a monolithic persistentHNSW caches
		// adjacency rows per instance, scoped to a single transaction's
		// view — reusing one instance across the per-uid replay
		// transactions serves stale rows and silently drops earlier
		// replays' back-links (observed: 66% of drained vectors orphaned).
		indexer, err := spec.FindOrCreateIndex(attr)
		if err != nil {
			return false, err
		}
		if pm.op == vectorPendingDel {
			// The builder indexed the value visible at StartTs (the
			// pre-delete snapshot); tombstone the uid in the dead list of
			// the cluster that value routes to. No vfloat value at StartTs
			// means the builder never indexed it: nothing to do.
			pl, err := GetNoStore(x.DataKey(attr, uid), rb.StartTs)
			if err != nil {
				return false, err
			}
			val, err := pl.Value(rb.StartTs)
			if err != nil || val.Value == nil || val.Tid != types.VFloatID {
				return false, nil
			}
			delVec := types.BytesAsFloatArray(val.Value.([]byte))
			deadAttr := hnsw.ConcatStrings(attr, hnsw.VecDead)
			if resolver, ok := indexer.(tokIndex.VectorDeadListResolver[float32]); ok {
				deadAttr, err = resolver.DeadAttrForVector(tc, delVec)
				if err != nil {
					return false, err
				}
			}
			if err := appendVectorDeadNode(ctx, txn, deadAttr, uid); err != nil {
				return false, err
			}
		} else {
			// The committed value was resolved above, before the version
			// was allocated; repeated updates collapsed into this one
			// replay, and a value deleted again by a later captured DEL is
			// handled by that entry instead.
			if _, err := indexer.Insert(ctx, tc, uid, inVec); err != nil {
				return false, err
			}
		}
		txn.Update()
		writer := NewTxnWriter(pstore)
		// MaxRetries can be zero (unset) outside a running alpha; retry at
		// least once or the commit is silently skipped (see RunWithoutTemp).
		if err := x.ExponentialRetry(max(1, int(x.Config.MaxRetries)),
			20*time.Millisecond, func() error {
				err := txn.CommitToDisk(writer, ts)
				if err == badger.ErrBannedKey {
					glog.Errorf("Error while writing to banned namespace.")
					return nil
				}
				return err
			}); err != nil {
			return false, err
		}
		if err := writer.Flush(); err != nil {
			return false, err
		}
		// The writer bypasses the memory layer; drop cached lists so the
		// next replay txn and post-drain readers see this write (the
		// builder does the same).
		ResetCache()
		return false, nil
	}

	// Converging rounds: mutations that land while a round replays are
	// captured into a fresh map and picked up next round; a captured uid
	// whose base transaction has not committed yet (capture happens at apply
	// time, the commit delta arrives later) is carried over until the oracle
	// resolves it. The gate stays up throughout, so no live index write can
	// interleave while replays are outstanding — which also means the gate
	// only closes when nothing is left to replay, removing any need to
	// replay under the capture lock.
	carry := map[uint64]vectorPendingMutation{}
	grace := map[uint64]int{}
	stalls := 0
	for {
		for uid, pm := range swapVectorPending(attr) {
			carry[uid] = pm // newer captures win over carried retries
		}
		if len(carry) == 0 {
			// Nothing left: close the gate, unless a capture slipped in
			// between the swap above and taking the lock.
			st.mu.Lock()
			if len(st.pending) == 0 {
				st.pending = nil
				st.mu.Unlock()
				break
			}
			st.mu.Unlock()
			continue
		}
		next := map[uint64]vectorPendingMutation{}
		progressed := false
		for uid, pm := range carry {
			retry, err := replayOne(uid, pm)
			if err != nil {
				return err
			}
			if !retry {
				progressed = true
				delete(grace, uid)
				continue
			}
			// Retry while the transaction is unresolved; once resolved,
			// allow a short grace for the commit to become durably readable,
			// then treat the uid as aborted (nothing to index).
			if Oracle().TxnPending(pm.startTs) {
				next[uid] = pm
				continue
			}
			if grace[uid] == 0 {
				grace[uid] = 40 // ~200ms at 5ms per stalled round
			}
			grace[uid]--
			if grace[uid] > 0 {
				next[uid] = pm
			} else {
				delete(grace, uid)
			}
		}
		carry = next
		if len(carry) > 0 && !progressed {
			// Only unresolved captures remain; wait for their deltas.
			stalls++
			if stalls%2000 == 0 {
				glog.Warningf("vector rebuild drain for %s waiting on %d "+
					"uncommitted captured mutations for %v", attr, len(carry),
					time.Duration(stalls)*5*time.Millisecond)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	FinishVectorRebuildCapture(attr)
	return nil
}

// appendVectorDeadNode appends uid to deadAttr's dead list (a single blob at
// entity 1, read-modify-write) — the replay-time equivalent of the live
// delete path in addIndexMutations.
func appendVectorDeadNode(ctx context.Context, txn *Txn, deadAttr string, uid uint64) error {
	deadKey := x.DataKey(deadAttr, 1)
	pl, err := txn.Get(deadKey)
	if err != nil {
		return err
	}
	var deadNodes []uint64
	deadData, _ := pl.Value(txn.StartTs)
	if deadData.Value != nil {
		deadNodes, err = hnsw.ParseEdges(string(deadData.Value.([]byte)))
		if err != nil {
			return err
		}
	}
	deadNodes = append(deadNodes, uid)
	deadNodesBytes, err := json.Marshal(deadNodes)
	if err != nil {
		return err
	}
	return pl.addMutation(ctx, txn, &pb.DirectedEdge{
		Entity:    1,
		Attr:      deadAttr,
		Value:     deadNodesBytes,
		ValueType: pb.Posting_ValType(0),
	})
}

// testingVectorRebuildNumGo, when positive, overrides the badger stream
// parallelism of index-rebuild scans (RunWithoutTemp). Tests pin it to 1 so
// graph construction is deterministic — concurrent build inserts produce a
// different graph shape every run, which tiny test graphs cannot absorb.
// Zero (the production value) keeps the adaptive default.
var testingVectorRebuildNumGo int

// TestingVectorRebuildStageHook, if non-nil, is invoked synchronously at
// named stage boundaries of a vector index rebuild ("training_done",
// "build_done"). Tests use it to interleave mutations at exact points without
// sleeps; it is nil in production and injects no behavior.
var TestingVectorRebuildStageHook func(stage string)

func vectorRebuildStage(stage string) {
	if TestingVectorRebuildStageHook != nil {
		TestingVectorRebuildStageHook(stage)
	}
}
