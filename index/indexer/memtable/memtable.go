/*
 * Copyright 2016 Dgraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package memtable

import (
	"sort"
	"sync"

	"github.com/dgraph-io/dgraph/index/indexer"
	"github.com/dgraph-io/dgraph/x"
)

// Indexer implements indexer.Indexer. It does not talk to disk at all.
// Very simple implementation. Just lock the whole Indexer. We can lock just
// predIndex, but this is really just a temporary solution and for testing.
type Indexer struct {
	sync.RWMutex
	idx map[string]*predIndex
}

type predIndex struct {
	pred     string
	forward  map[string]string // Key -> Val.
	backward map[string]uidSet // Val -> Multiple keys.
}

type uidSet map[string]struct{}

type mutation struct {
	remove         bool // If false, this is a insert.
	pred, key, val string
}

type batch struct {
	sync.RWMutex
	m []*mutation
}

func init() {
	indexer.Register("memtable", New)
}

// New creates our memtable indexer.
func New() indexer.Indexer {
	return &Indexer{
		idx: make(map[string]*predIndex),
	}
}

// NewBatch creates our own batch object.
func (s *Indexer) NewBatch() (indexer.Batch, error) {
	return &batch{}, nil
}

// Open opens a directory and creates indexer from it.
func (s *Indexer) Open(dir string) error { return nil }

// Close closes the indexer.
func (s *Indexer) Close() error { return nil }

// Create populates an empty directory and initializes an Indexer.
func (s *Indexer) Create(dir string) error { return nil }

func (s *Indexer) getOrNewPred(pred string) *predIndex {
	idx := s.idx[pred]
	if idx == nil {
		idx = &predIndex{
			pred:     pred,
			forward:  make(map[string]string),
			backward: make(map[string]uidSet),
		}
		s.idx[pred] = idx
	}
	return idx
}

func (s *predIndex) delBackward(key, val string) {
	if us, found := s.backward[val]; found {
		delete(us, key)
	}
}

// Insert adds to the indexer a key, val pair. It can overwrite existing value.
func (s *Indexer) Insert(pred, key, val string) error {
	s.Lock()
	defer s.Unlock()

	idx := s.getOrNewPred(pred)
	// Check if key has an old value.
	oldVal, found := idx.forward[key]
	if found {
		if oldVal == val {
			// Old value equal to new value! Nothing to do.
			return nil
		}
		idx.delBackward(key, oldVal)
	}
	idx.forward[key] = val

	// Add to backward.
	us, found := idx.backward[val]
	if !found {
		us = make(map[string]struct{})
		idx.backward[val] = us
	}
	us[key] = struct{}{}
	return nil
}

// Remove removes from indexer a certain key. If missing, nothing is done.
func (s *Indexer) Remove(pred, key string) error {
	s.Lock()
	defer s.Unlock()

	idx := s.idx[pred]
	if idx == nil {
		return nil
	}

	val, found := idx.forward[key]
	if !found {
		// Key is not in forward map. Nothing to delete.
		// Assume backward is consistent with forward.
		return nil
	}

	// Do the actual updates.
	delete(idx.forward, key)
	idx.delBackward(key, val)
	return nil
}

// Query asks indexer for keys associated with a certain value. Output is sorted.
func (s *Indexer) Query(pred, val string) ([]string, error) {
	s.RLock()
	defer s.RUnlock()

	idx := s.idx[pred]
	if idx == nil {
		return nil, nil
	}

	us := idx.backward[val]
	if len(us) == 0 { // us can be nil.
		return nil, nil
	}

	// Return "us" as sorted keys.
	keys := make([]string, 0, len(us))
	for k := range us {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys, nil
}

// Size returns size of batch.
func (b *batch) Size() int {
	b.RLock()
	defer b.RUnlock()
	return len(b.m)
}

// Reset resets the batch to hold nothing.
func (b *batch) Reset() {
	b.Lock()
	defer b.Unlock()
	b.m = nil
}

// Insert adds an insert operation to the batch.
func (b *batch) Insert(pred, key, val string) error {
	b.Lock()
	defer b.Unlock()
	b.m = append(b.m, &mutation{
		remove: false,
		pred:   pred,
		key:    key,
		val:    val,
	})
	return nil
}

// Remove adds a remove operation to the batch.
func (b *batch) Remove(pred, key string) error {
	b.Lock()
	defer b.Unlock()
	b.m = append(b.m, &mutation{
		remove: true,
		pred:   pred,
		key:    key,
	})
	return nil
}

// Batch executes the batch of operations.
func (s *Indexer) Batch(b indexer.Batch) error {
	bb := b.(*batch)
	bb.RLock()
	defer bb.RUnlock()
	x.Assert(bb != nil)
	for _, m := range bb.m {
		if m.remove {
			s.Remove(m.pred, m.key)
		} else {
			s.Insert(m.pred, m.key, m.val)
		}
	}
	return nil
}
