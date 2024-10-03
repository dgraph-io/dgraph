/*
 * Copyright 2016-2024 Dgraph Labs, Inc. and Contributors
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
 *
 * Co-authored by: jairad26@gmail.com, sunil@hypermode.com, bill@hypdermode.com
 */

package hnsw

import (
	"context"
	"encoding/binary"
	"math"
	"strings"
	"sync"

	"github.com/dgraph-io/dgraph/v24/tok/index"
	"github.com/pkg/errors"
)

// holds an map in memory that is a string (which will be []bytes as string)
// as the key, with an index.Val as the value
type indexStorage struct {
	inMemTestDb map[string]index.Value

	//Two locks allow for lock promotion when writing, so we promote a read lock
	//between the start and finish times to a full lock on the finish time

	// readMu acquires read locks when accessing values
	readMu sync.RWMutex
	// writeMu acquires write locks on mutations
	writeMu sync.Mutex
}

// datastructure visualization of persistent db over 100 units of time
// within this, we will conduct all testing, i.e. reads at 1 Ts = tsDbs[1],
// writes at 4 Ts = tsDbs[4]
var tsDbs [100]indexStorage

func emptyTsDbs() {
	for i := range tsDbs {
		tsDbs[i] = indexStorage{inMemTestDb: make(map[string]index.Value)}
	}
}

type inMemList struct {
	key      string
	startTs  uint64
	finishTs uint64
}

// creates a new inMem list with the list's corresponding key,
// when it's action was started and when it will conclude.
// for mutations startTs will be txn.StartTs and finishTs will be txn.commitTs
// for reads, they both start and finish at c.ReadTs
// finishTs is unknown in real scenarios, this is for testing purposes
func newInMemList(key string, startTs, finishTs uint64) *inMemList {
	return &inMemList{
		key:      key,
		startTs:  startTs,
		finishTs: finishTs,
	}
}

// locks the posting list & invokes ValueWithLockHeld
func (l *inMemList) Value(readTs uint64) (rval index.Value, rerr error) {
	// reading should only lock the db at current instance in time
	tsDbs[readTs].readMu.RLock()
	defer tsDbs[readTs].readMu.RUnlock()
	return l.ValueWithLockHeld(readTs)
}

// reads value from the database at readTs corresponding to List's key
func (l *inMemList) ValueWithLockHeld(readTs uint64) (rval index.Value, rerr error) {
	val, ok := tsDbs[readTs].inMemTestDb[l.key]
	if !ok {
		return nil, errors.New("Could not find data with key " + l.key)
	}
	return val, nil
}

// locks the posting list and invokes AddMutationWithLockHeld
func (l *inMemList) AddMutation(ctx context.Context, txn index.Txn, t *index.KeyValue) error {
	// locks from the txn.StartTs up to txn.CommitTs
	l.Lock()
	defer l.Unlock()
	return l.AddMutationWithLockHeld(ctx, txn, t)
}

// adds mutation to the database at the txn's commitTs
func (l *inMemList) AddMutationWithLockHeld(ctx context.Context, txn index.Txn, t *index.KeyValue) error {
	// creates key from directedEdge
	//builds value
	val := t.Value
	// a mutation persists from the moment the txn gets committed until the "rest of time"
	for i := l.finishTs; i < uint64(len(tsDbs)); i++ {
		tsDbs[i].inMemTestDb[l.key] = val
	}
	return nil
}

// if youre locking at a certain point in time, the lock should be held for this moment
// and all future moments until your commitTs
func (l *inMemList) Lock() {
	if !strings.Contains(l.key, "entry") {
		for i := l.startTs; i <= l.finishTs; i++ {
			tsDbs[i].readMu.RLock()
		}
		for i := l.finishTs; i < uint64(len(tsDbs)); i++ {
			tsDbs[i].writeMu.Lock()
		}
	}
}

// undoes lock
func (l *inMemList) Unlock() {
	if !strings.Contains(l.key, "entry") {
		for i := l.startTs; i <= l.finishTs; i++ {
			tsDbs[i].readMu.RUnlock()
		}
		for i := l.finishTs; i < uint64(len(tsDbs)); i++ {
			tsDbs[i].writeMu.Unlock()
		}
	}
}

// a txn has a startTs (when the txn started) and commitTs (when the txn changes were committed)
type inMemTxn struct {
	startTs  uint64
	commitTs uint64
}

func (t *inMemTxn) Find(prefix []byte, filter func([]byte) bool) (uint64, error) {
	tsDbs[t.startTs].readMu.RLock()
	defer tsDbs[t.startTs].readMu.RUnlock()
	for _, b := range tsDbs[t.startTs].inMemTestDb {
		if filter(b.([]byte)) {
			return 1, nil
		}
	}
	return 0, nil
}

func (t *inMemTxn) StartTs() uint64 {
	return t.startTs
}

// locks the txn and invokes GetWithLockHeld
func (t *inMemTxn) Get(key []byte) (rval index.Value, rerr error) {
	tsDbs[t.startTs].readMu.RLock()
	defer tsDbs[t.startTs].readMu.RUnlock()
	return t.GetWithLockHeld(key)
}

// reads value from the database at txn's startTs
func (t *inMemTxn) GetWithLockHeld(key []byte) (rval index.Value, rerr error) {
	val, ok := tsDbs[t.startTs].inMemTestDb[string(key[:])]
	if !ok {
		return nil, errors.New("Could not find data with key " + string(key[:]))
	}
	return val, nil
}

// locks the txn and invokes AddMutationWithLockHeld
func (t *inMemTxn) AddMutation(ctx context.Context, key []byte, t1 *index.KeyValue) error {
	tsDbs[t.startTs].writeMu.Lock()
	defer tsDbs[t.startTs].writeMu.Unlock()
	return t.AddMutationWithLockHeld(ctx, key, t1)
}

// adds mutation to the database at the txn's commitTs
func (t *inMemTxn) AddMutationWithLockHeld(ctx context.Context, key []byte, t1 *index.KeyValue) error {
	val := t1.Value
	for i := t.commitTs; i < uint64(len(tsDbs)); i++ {
		tsDbs[i].inMemTestDb[string(key[:])] = val
	}
	return nil
}

// locks the txn
func (t *inMemTxn) LockKey(key []byte) {
	if !strings.Contains(string(key[:]), "entry") {
		// locks from the txn.StartTs up to txn.CommitTs
		for i := t.startTs; i <= t.commitTs; i++ {
			tsDbs[i].readMu.RLock()
		}
		for i := t.commitTs; i < uint64(len(tsDbs)); i++ {
			tsDbs[i].writeMu.Lock()
		}
	}
}

// undoes lock
func (t *inMemTxn) UnlockKey(key []byte) {
	if !strings.Contains(string(key[:]), "entry") {
		// locks from the txn.StartTs up to txn.CommitTs
		for i := t.startTs; i <= t.commitTs; i++ {
			tsDbs[i].readMu.RUnlock()
		}
		for i := t.commitTs; i < uint64(len(tsDbs)); i++ {
			tsDbs[i].writeMu.Unlock()
		}
	}
}

type inMemLocalCache struct {
	readTs uint64
}

// locks the local cache and invokes GetWithLockHeld
func (c *inMemLocalCache) Get(key []byte) (rval index.Value, rerr error) {
	tsDbs[c.readTs].readMu.RLock()
	defer tsDbs[c.readTs].readMu.RUnlock()
	return c.GetWithLockHeld(key)
}

func (c *inMemLocalCache) Find(prefix []byte, filter func([]byte) bool) (uint64, error) {
	tsDbs[c.readTs].readMu.RLock()
	defer tsDbs[c.readTs].readMu.RUnlock()
	for _, b := range tsDbs[c.readTs].inMemTestDb {
		if filter(b.([]byte)) {
			return 1, nil
		}
	}
	return 0, nil
}

// reads value from the database at c's readTs
func (c *inMemLocalCache) GetWithLockHeld(key []byte) (rval index.Value, rerr error) {
	val, ok := tsDbs[c.readTs].inMemTestDb[string(key[:])]
	if !ok {
		return nil, errors.New("Could not find data with key " + string(key[:]))
	}
	return val, nil
}

func equalFloat64Slice(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalUint64Slice(a, b []uint64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// floatArrayAsBytes(v) will create a byte array encoding
// v using LittleEndian format. This is sort of the inverse
// of BytesAsFloatArray, but note that we can always be successful
// converting to bytes, but the inverse is not feasible.
func floatArrayAsBytes(v []float64) []byte {
	retVal := make([]byte, 8*len(v))
	offset := retVal
	for i := range v {
		bits := math.Float64bits(v[i])
		binary.LittleEndian.PutUint64(offset, bits)
		offset = offset[8:]
	}
	return retVal
}
