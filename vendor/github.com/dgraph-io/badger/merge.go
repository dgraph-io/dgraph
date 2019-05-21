/*
 * Copyright 2017 Dgraph Labs, Inc. and Contributors
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

package badger

import (
	"sync"
	"time"

	"github.com/dgraph-io/badger/y"
	"github.com/pkg/errors"
)

// MergeOperator represents a Badger merge operator.
type MergeOperator struct {
	sync.RWMutex
	f      MergeFunc
	db     *DB
	key    []byte
	closer *y.Closer
}

// MergeFunc accepts two byte slices, one representing an existing value, and
// another representing a new value that needs to be ‘merged’ into it. MergeFunc
// contains the logic to perform the ‘merge’ and return an updated value.
// MergeFunc could perform operations like integer addition, list appends etc.
// Note that the ordering of the operands is unspecified, so the merge func
// should either be agnostic to ordering or do additional handling if ordering
// is required.
type MergeFunc func(existing, val []byte) []byte

// GetMergeOperator creates a new MergeOperator for a given key and returns a
// pointer to it. It also fires off a goroutine that performs a compaction using
// the merge function that runs periodically, as specified by dur.
func (db *DB) GetMergeOperator(key []byte,
	f MergeFunc, dur time.Duration) *MergeOperator {
	op := &MergeOperator{
		f:      f,
		db:     db,
		key:    key,
		closer: y.NewCloser(1),
	}

	go op.runCompactions(dur)
	return op
}

var errNoMerge = errors.New("No need for merge")

func (op *MergeOperator) iterateAndMerge(txn *Txn) (val []byte, err error) {
	opt := DefaultIteratorOptions
	opt.AllVersions = true
	it := txn.NewKeyIterator(op.key, opt)
	defer it.Close()

	var numVersions int
	for it.Rewind(); it.Valid(); it.Next() {
		item := it.Item()
		numVersions++
		if numVersions == 1 {
			val, err = item.ValueCopy(val)
			if err != nil {
				return nil, err
			}
		} else {
			if err := item.Value(func(newVal []byte) error {
				val = op.f(val, newVal)
				return nil
			}); err != nil {
				return nil, err
			}
		}
		if item.DiscardEarlierVersions() {
			break
		}
	}
	if numVersions == 0 {
		return nil, ErrKeyNotFound
	} else if numVersions == 1 {
		return val, errNoMerge
	}
	return val, nil
}

func (op *MergeOperator) compact() error {
	op.Lock()
	defer op.Unlock()
	err := op.db.Update(func(txn *Txn) error {
		var (
			val []byte
			err error
		)
		val, err = op.iterateAndMerge(txn)
		if err != nil {
			return err
		}
		// Write value back to the DB. It is important that we do not set the bitMergeEntry bit
		// here. When compaction happens, all the older merged entries will be removed.
		return txn.SetWithDiscard(op.key, val, 0)
	})

	if err == ErrKeyNotFound || err == errNoMerge {
		// pass.
	} else if err != nil {
		return err
	}
	return nil
}

func (op *MergeOperator) runCompactions(dur time.Duration) {
	ticker := time.NewTicker(dur)
	defer op.closer.Done()
	var stop bool
	for {
		select {
		case <-op.closer.HasBeenClosed():
			stop = true
		case <-ticker.C: // wait for tick
		}
		if err := op.compact(); err != nil {
			op.db.opt.Errorf("failure while running merge operation: %s", err)
		}
		if stop {
			ticker.Stop()
			break
		}
	}
}

// Add records a value in Badger which will eventually be merged by a background
// routine into the values that were recorded by previous invocations to Add().
func (op *MergeOperator) Add(val []byte) error {
	return op.db.Update(func(txn *Txn) error {
		return txn.setMergeEntry(op.key, val)
	})
}

// Get returns the latest value for the merge operator, which is derived by
// applying the merge function to all the values added so far.
//
// If Add has not been called even once, Get will return ErrKeyNotFound.
func (op *MergeOperator) Get() ([]byte, error) {
	op.RLock()
	defer op.RUnlock()
	var existing []byte
	err := op.db.View(func(txn *Txn) (err error) {
		existing, err = op.iterateAndMerge(txn)
		return err
	})
	if err == errNoMerge {
		return existing, nil
	}
	return existing, err
}

// Stop waits for any pending merge to complete and then stops the background
// goroutine.
func (op *MergeOperator) Stop() {
	op.closer.SignalAndWait()
}
