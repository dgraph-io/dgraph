/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package posting

import (
	"context"
	"encoding/hex"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/badger/v3/skl"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	ostats "go.opencensus.io/stats"
	otrace "go.opencensus.io/trace"
)

var o *oracle

// Oracle returns the global oracle instance.
// TODO: Oracle should probably be located in worker package, instead of posting
// package now that we don't run inSnapshot anymore.
func Oracle() *oracle {
	return o
}

func init() {
	o = new(oracle)
	o.init()
}

// Txn represents a transaction.
type Txn struct {
	StartTs          uint64
	MaxAssignedSeen  uint64
	AppliedIndexSeen uint64

	// Runs keeps track of how many times this txn has been through applyCh.
	Runs int32

	// atomic
	shouldAbort uint32
	// Fields which can changed after init
	sync.Mutex

	// Keeps track of conflict keys that should be used to determine if this
	// transaction conflicts with another.
	conflicts map[uint64]struct{}

	// Keeps track of last update wall clock. We use this fact later to
	// determine unhealthy, stale txns.
	lastUpdate time.Time

	cache *LocalCache // This pointer does not get modified.
	ErrCh chan error

	slWait sync.WaitGroup
	sl     *skl.Skiplist
}

// NewTxn returns a new Txn instance.
func NewTxn(startTs uint64) *Txn {
	return &Txn{
		StartTs:    startTs,
		cache:      NewLocalCache(startTs),
		lastUpdate: time.Now(),
		ErrCh:      make(chan error, 1),
	}
}

// Get retrieves the posting list for the given list from the local cache.
func (txn *Txn) Get(key []byte) (*List, error) {
	return txn.cache.Get(key)
}

// GetFromDelta retrieves the posting list from delta cache, not from Badger.
func (txn *Txn) GetFromDelta(key []byte) (*List, error) {
	return txn.cache.GetFromDelta(key)
}

func (txn *Txn) Skiplist() *skl.Skiplist {
	txn.slWait.Wait()
	return txn.sl
}

// Update calls UpdateDeltasAndDiscardLists on the local cache.
func (txn *Txn) Update(ctx context.Context) {
	txn.Lock()
	defer txn.Unlock()
	txn.cache.UpdateDeltasAndDiscardLists()

	// If we already have a pending Update, then wait for it to be done first. So it does not end up
	// overwriting the skiplist that we generate here.
	txn.slWait.Wait()
	txn.slWait.Add(1)
	go func() {
		if err := txn.ToSkiplist(); err != nil {
			glog.Errorf("While creating skiplist: %v\n", err)
		}
		span := otrace.FromContext(ctx)
		span.Annotate(nil, "ToSkiplist done")
		txn.slWait.Done()
	}()
}

// Store is used by tests.
func (txn *Txn) Store(pl *List) *List {
	return txn.cache.SetIfAbsent(string(pl.key), pl)
}

type oracle struct {
	x.SafeMutex

	// max start ts given out by Zero. Do not use mutex on this, only use atomics.
	maxAssigned uint64

	// Keeps track of all the startTs we have seen so far, based on the mutations. Then as
	// transactions are committed or aborted, we delete entries from the startTs map. When taking a
	// snapshot, we need to know the minimum start ts present in the map, which represents a
	// mutation which has not yet been committed or aborted.  As we iterate over entries, we should
	// only discard those whose StartTs is below this minimum pending start ts.
	pendingTxns map[uint64]*Txn

	// Used for waiting logic for transactions with startTs > maxpending so that we don't read an
	// uncommitted transaction.
	waiters map[uint64][]chan struct{}
}

func (o *oracle) init() {
	o.waiters = make(map[uint64][]chan struct{})
	o.pendingTxns = make(map[uint64]*Txn)
}

// RegisterStartTs would return a txn and a bool.
// If the bool is true, the txn was already present. If false, it is new.
func (o *oracle) RegisterStartTs(ts uint64) (*Txn, bool) {
	o.Lock()
	defer o.Unlock()
	txn, ok := o.pendingTxns[ts]
	if ok {
		txn.lastUpdate = time.Now()
	} else {
		txn = NewTxn(ts)
		o.pendingTxns[ts] = txn
	}
	return txn, ok
}

func (o *oracle) ResetTxn(ts uint64) *Txn {
	o.Lock()
	defer o.Unlock()

	txn := NewTxn(ts)
	o.pendingTxns[ts] = txn
	return txn
}

func (o *oracle) CacheAt(ts uint64) *LocalCache {
	o.RLock()
	defer o.RUnlock()
	txn, ok := o.pendingTxns[ts]
	if !ok {
		return nil
	}
	return txn.cache
}

// MinPendingStartTs returns the min start ts which is currently pending a commit or abort decision.
func (o *oracle) MinPendingStartTs() uint64 {
	o.RLock()
	defer o.RUnlock()
	min := uint64(math.MaxUint64)
	for ts := range o.pendingTxns {
		if ts < min {
			min = ts
		}
	}
	return min
}

func (o *oracle) MinMaxAssignedSeenTs() uint64 {
	o.RLock()
	defer o.RUnlock()
	min := o.MaxAssigned()
	for _, txn := range o.pendingTxns {
		if txn.MaxAssignedSeen < min {
			min = txn.MaxAssignedSeen
		}
	}
	return min
}

func (o *oracle) NumPendingTxns() int {
	o.RLock()
	defer o.RUnlock()
	return len(o.pendingTxns)
}

func (o *oracle) TxnOlderThan(dur time.Duration) (res []uint64) {
	o.RLock()
	defer o.RUnlock()

	cutoff := time.Now().Add(-dur)
	for startTs, txn := range o.pendingTxns {
		if txn.lastUpdate.Before(cutoff) {
			res = append(res, startTs)
		}
	}
	return res
}

func (o *oracle) addToWaiters(startTs uint64) (chan struct{}, bool) {
	if startTs <= o.MaxAssigned() {
		return nil, false
	}
	o.Lock()
	defer o.Unlock()
	// Check again after acquiring lock, because o.waiters is being processed serially. So, if we
	// don't check here, then it's possible that we add to waiters here, but MaxAssigned has already
	// moved past startTs.
	if startTs <= o.MaxAssigned() {
		return nil, false
	}
	ch := make(chan struct{})
	o.waiters[startTs] = append(o.waiters[startTs], ch)
	return ch, true
}

func (o *oracle) MaxAssigned() uint64 {
	return atomic.LoadUint64(&o.maxAssigned)
}
func (o *oracle) SetMaxAssigned(m uint64) {
	cur := atomic.LoadUint64(&o.maxAssigned)
	glog.Infof("Current MaxAssigned: %d. SetMaxAssigned: %d.\n", cur, m)
	if m < cur {
		return
	}
	atomic.StoreUint64(&o.maxAssigned, m)
}

func (o *oracle) WaitForTs(ctx context.Context, startTs uint64) error {
	ch, ok := o.addToWaiters(startTs)
	if !ok {
		return nil
	}
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (o *oracle) DeleteTxns(delta *pb.OracleDelta) {
	o.Lock()
	for _, txn := range delta.Txns {
		delete(o.pendingTxns, txn.StartTs)
	}
	o.Unlock()
}

func (o *oracle) ProcessDelta(delta *pb.OracleDelta) {
	if glog.V(3) {
		glog.Infof("ProcessDelta: Max Assigned: %d", delta.MaxAssigned)
		glog.Infof("ProcessDelta: Group checksum: %v", delta.GroupChecksums)
		for _, txn := range delta.Txns {
			if txn.CommitTs == 0 {
				glog.Infof("ProcessDelta Aborted: %d", txn.StartTs)
			} else {
				glog.Infof("ProcessDelta Committed: %d -> %d", txn.StartTs, txn.CommitTs)
			}
		}
	}

	o.Lock()
	defer o.Unlock()
	curMax := o.MaxAssigned()
	if delta.MaxAssigned < curMax {
		return
	}

	// Notify the waiting cattle.
	for startTs, toNotify := range o.waiters {
		if startTs > delta.MaxAssigned {
			continue
		}
		for _, ch := range toNotify {
			close(ch)
		}
		delete(o.waiters, startTs)
	}
	x.AssertTrue(atomic.CompareAndSwapUint64(&o.maxAssigned, curMax, delta.MaxAssigned))
	ostats.Record(context.Background(),
		x.MaxAssignedTs.M(int64(delta.MaxAssigned))) // Can't access o.MaxAssigned without atomics.
}

func (o *oracle) ResetTxns() {
	o.Lock()
	defer o.Unlock()
	o.pendingTxns = make(map[uint64]*Txn)
}

// ResetTxnForNs deletes all the pending transactions for a given namespace.
func (o *oracle) ResetTxnsForNs(ns uint64) {
	txns := o.IterateTxns(func(key []byte) bool {
		pk, err := x.Parse(key)
		if err != nil {
			glog.Errorf("error %v while parsing key %v", err, hex.EncodeToString(key))
			return false
		}
		return x.ParseNamespace(pk.Attr) == ns
	})
	o.Lock()
	defer o.Unlock()
	for _, txn := range txns {
		delete(o.pendingTxns, txn)
	}
}

func (o *oracle) GetTxn(startTs uint64) *Txn {
	o.RLock()
	defer o.RUnlock()
	return o.pendingTxns[startTs]
}

func (txn *Txn) matchesDelta(ok func(key []byte) bool) bool {
	txn.Lock()
	defer txn.Unlock()
	for key := range txn.cache.deltas {
		if ok([]byte(key)) {
			return true
		}
	}
	return false
}

// IterateTxns returns a list of start timestamps for currently pending transactions, which match
// the provided function.
func (o *oracle) IterateTxns(ok func(key []byte) bool) []uint64 {
	o.RLock()
	defer o.RUnlock()
	var timestamps []uint64
	for startTs, txn := range o.pendingTxns {
		if txn.matchesDelta(ok) {
			timestamps = append(timestamps, startTs)
		}
	}
	return timestamps
}
