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
	"math"
	"sync"
	"time"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	ostats "go.opencensus.io/stats"
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
	StartTs uint64

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
}

// NewTxn returns a new Txn instance.
func NewTxn(startTs uint64) *Txn {
	return &Txn{
		StartTs:    startTs,
		cache:      NewLocalCache(startTs),
		lastUpdate: time.Now(),
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

// Update calls UpdateDeltasAndDiscardLists on the local cache.
func (txn *Txn) Update() {
	txn.cache.UpdateDeltasAndDiscardLists()
}

// Store is used by tests.
func (txn *Txn) Store(pl *List) *List {
	return txn.cache.SetIfAbsent(string(pl.key), pl)
}

type oracle struct {
	x.SafeMutex

	// max start ts given out by Zero. Do not use mutex on this, only use atomics.
	maxAssigned map[string]uint64

	// Keeps track of all the startTs we have seen so far, based on the mutations. Then as
	// transactions are committed or aborted, we delete entries from the startTs map. When taking a
	// snapshot, we need to know the minimum start ts present in the map, which represents a
	// mutation which has not yet been committed or aborted.  As we iterate over entries, we should
	// only discard those whose StartTs is below this minimum pending start ts.
	pendingTxns map[string]map[uint64]*Txn

	// Used for waiting logic for transactions with startTs > maxpending so that we don't read an
	// uncommitted transaction.
	waiters map[string]map[uint64][]chan struct{}
}

func (o *oracle) init() {
	o.waiters = make(map[string]map[uint64][]chan struct{})
	o.pendingTxns = make(map[string]map[uint64]*Txn)
}

func (o *oracle) RegisterStartTs(namespace string, ts uint64) *Txn {
	o.Lock()
	defer o.Unlock()
	txns, ok := o.pendingTxns[namespace]
	if !ok {
		txns = make(map[uint64]*Txn)
	}
	txn, ok := txns[ts]
	if ok {
		txn.lastUpdate = time.Now()
	} else {
		txn = NewTxn(ts)
		txns[ts] = txn
	}
	o.pendingTxns[namespace] = txns
	return txn
}

func (o *oracle) CacheAt(namespces string, ts uint64) *LocalCache {
	o.RLock()
	defer o.RUnlock()
	txns, ok := o.pendingTxns[namespces]
	if !ok {
		return nil
	}
	txn, ok := txns[ts]
	if !ok {
		return nil
	}
	return txn.cache
}

// MinPendingStartTs returns the min start ts which is currently pending a commit or abort decision
// and it's namespace.
func (o *oracle) MinPendingStartTs() (uint64, string) {
	o.RLock()
	defer o.RUnlock()
	min := uint64(math.MaxUint64)
	minNamespace := ""
	for namespace, txn := range o.pendingTxns {
		for ts := range txn {
			if ts < min {
				min = ts
				minNamespace = namespace
			}
		}

	}
	return min, minNamespace
}

func (o *oracle) NumPendingTxns() int {
	o.RLock()
	defer o.RUnlock()
	return len(o.pendingTxns)
}

// OlderTxn contain all the pending start timestamp of a namespace
type OlderTxn struct {
	Namespace string
	StartTs   []uint64
}

// TxnOlderThan will give older txn of all the namespace
func (o *oracle) TxnOlderThan(dur time.Duration) (oldTxns []*OlderTxn) {
	o.RLock()
	defer o.RUnlock()
	cutoff := time.Now().Add(-dur)
	for namespace, txns := range o.pendingTxns {
		oldTxn := &OlderTxn{
			Namespace: namespace,
		}
		for startTs, txn := range txns {
			if txn.lastUpdate.Before(cutoff) {
				oldTxn.StartTs = append(oldTxn.StartTs, startTs)
			}
		}
		oldTxns = append(oldTxns, oldTxn)
	}
	return oldTxns
}

func (o *oracle) addToWaiters(namespace string, startTs uint64) (chan struct{}, bool) {
	// Check again after acquiring lock, because o.waiters is being processed serially. So, if we
	// don't check here, then it's possible that we add to waiters here, but MaxAssigned has already
	// moved past startTs. Caller should take care of thread safety.
	if startTs <= o.MaxAssigned(namespace) {
		return nil, false
	}
	waiters, ok := o.waiters[namespace]
	if !ok {
		waiters = make(map[uint64][]chan struct{})
	}
	ch := make(chan struct{})
	waiters[startTs] = append(waiters[startTs], ch)
	o.waiters[namespace] = waiters
	return ch, true
}

func (o *oracle) MaxAssigned(namespace string) uint64 {
	// caller should take care of lock
	return o.maxAssigned[namespace]
}

func (o *oracle) WaitForTs(ctx context.Context, namespace string, startTs uint64) error {
	o.Lock()
	defer o.Unlock()
	ch, ok := o.addToWaiters(namespace, startTs)
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

func (o *oracle) WaitForAllNamespace(ctx context.Context, startTs uint64) error {
	o.Lock()
	defer o.Unlock()
	for namespace := range o.waiters {
		ch, ok := o.addToWaiters(namespace, startTs)
		if !ok {
			continue
		}
		select {
		case <-ch:
			continue
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
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
	for _, txn := range delta.Txns {
		delete(o.pendingTxns[delta.Namespace], txn.StartTs)
	}
	curMax := o.MaxAssigned(delta.GetNamespace())
	if delta.MaxAssigned < curMax {
		return
	}

	// Notify the waiting cattle.
	for startTs, toNotify := range o.waiters[delta.GetNamespace()] {
		if startTs > delta.MaxAssigned {
			continue
		}
		for _, ch := range toNotify {
			close(ch)
		}
		delete(o.waiters[delta.GetNamespace()], startTs)
	}
	o.maxAssigned[delta.GetNamespace()] = delta.GetMaxAssigned()
	ostats.Record(context.Background(),
		x.MaxAssignedTs.M(int64(delta.MaxAssigned))) // Can't access o.MaxAssigned without atomics.
}

func (o *oracle) ResetTxns() {
	o.Lock()
	defer o.Unlock()
	o.pendingTxns = make(map[string]map[uint64]*Txn)
}

func (o *oracle) GetTxn(namespace string, startTs uint64) *Txn {
	o.RLock()
	defer o.RUnlock()
	return o.pendingTxns[namespace][startTs]
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
func (o *oracle) IterateTxns(namespace string, ok func(key []byte) bool) []uint64 {
	o.RLock()
	defer o.RUnlock()
	var timestamps []uint64
	for startTs, txn := range o.pendingTxns[namespace] {
		if txn.matchesDelta(ok) {
			timestamps = append(timestamps, startTs)
		}
	}
	return timestamps
}
