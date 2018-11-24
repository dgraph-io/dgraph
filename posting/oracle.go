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
)

var o *oracle

// TODO: Oracle should probably be located in worker package, instead of posting
// package now that we don't run inSnapshot anymore.
func Oracle() *oracle {
	return o
}

func init() {
	o = new(oracle)
	o.init()
}

type Txn struct {
	StartTs uint64

	// atomic
	shouldAbort uint32
	// Fields which can changed after init
	sync.Mutex
	// Deltas keeps track of the posting list keys, and whether they should be considered for
	// conflict detection or not. When a txn is marked committed or aborted, we use the keys stored
	// here to determine which posting lists to get and update.
	deltas map[string]struct{}

	// Keeps track of conflict keys that should be used to determine if this
	// transaction conflicts with another.
	conflicts map[string]struct{}

	// Keeps track of last update wall clock. We use this fact later to
	// determine unhealthy, stale txns.
	lastUpdate time.Time

	// getList can be set for a txn, to isolate the retrieval and storage of
	// posting lists in a separate cache. If nil, global LRU cache is used.
	getList func(key []byte) (*List, error)
}

func (txn *Txn) Get(key []byte) (*List, error) {
	if txn.getList == nil {
		return Get(key)
	}
	return txn.getList(key)
}

type oracle struct {
	x.SafeMutex

	// max start ts given out by Zero.
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

func (o *oracle) RegisterStartTs(ts uint64) *Txn {
	o.Lock()
	defer o.Unlock()
	txn, ok := o.pendingTxns[ts]
	if ok {
		txn.lastUpdate = time.Now()
	} else {
		txn = &Txn{StartTs: ts, lastUpdate: time.Now()}
		o.pendingTxns[ts] = txn
	}
	return txn
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
	o.Lock()
	defer o.Unlock()
	if o.maxAssigned >= startTs {
		return nil, false
	}
	ch := make(chan struct{})
	o.waiters[startTs] = append(o.waiters[startTs], ch)
	return ch, true
}

func (o *oracle) MaxAssigned() uint64 {
	o.RLock()
	defer o.RUnlock()
	return o.maxAssigned
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

func (o *oracle) ProcessDelta(delta *pb.OracleDelta) {
	o.Lock()
	defer o.Unlock()
	for _, txn := range delta.Txns {
		delete(o.pendingTxns, txn.StartTs)
	}
	if delta.MaxAssigned < o.maxAssigned {
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
	o.maxAssigned = delta.MaxAssigned
}

func (o *oracle) ResetTxns() {
	o.Lock()
	defer o.Unlock()
	o.pendingTxns = make(map[uint64]*Txn)
}

func (o *oracle) GetTxn(startTs uint64) *Txn {
	o.RLock()
	defer o.RUnlock()
	return o.pendingTxns[startTs]
}

func (t *Txn) matchesDelta(ok func(key []byte) bool) bool {
	t.Lock()
	defer t.Unlock()
	for key := range t.deltas {
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
