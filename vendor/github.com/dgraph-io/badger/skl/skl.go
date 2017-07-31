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

/*
Adapted from RocksDB inline skiplist.

Key differences:
- No optimization for sequential inserts (no "prev").
- No custom comparator.
- Support overwrites. This requires care when we see the same key when inserting.
  For RocksDB or LevelDB, overwrites are implemented as a newer sequence number in the key, so
	there is no need for values. We don't intend to support versioning. In-place updates of values
	would be more efficient.
- We discard all non-concurrent code.
- We do not support Splices. This simplifies the code a lot.
- No AllocateNode or other pointer arithmetic.
- We combine the findLessThan, findGreaterOrEqual, etc into one function.
*/

package skl

import (
	"bytes"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/dgraph-io/badger/y"
)

const (
	kMaxHeight      = 20
	kHeightIncrease = math.MaxUint32 / 3
)

type node struct {
	// ONLY guards valueOffset, valueSize. To do without lock, we need to encode
	// valueSize in arena. Consider this next time.
	// RWMutex takes 24 bytes whereas Mutex takes 8 bytes.
	sync.Mutex

	// A byte slice is 24 bytes. We are trying to save space here.
	keyOffset uint32 // Immutable. No need to lock to access key.
	keySize   uint16 // Immutable. No need to lock to access key.

	valueOffset uint32
	valueSize   uint16 // Assume values not too big. There's value threshold.

	// []*node. Size is <=kMaxNumLevels. Usually a very small array. CAS.
	// No need to lock.
	next []unsafe.Pointer
}

type Skiplist struct {
	height int32 // Current height. 1 <= height <= kMaxHeight. CAS.
	head   *node
	ref    int32
	arena  *Arena
}

var (
	nodePools []sync.Pool
)

func init() {
	nodePools = make([]sync.Pool, kMaxHeight+1)
	for i := 1; i <= kMaxHeight; i++ {
		func(i int) { // Need a function here in order to capture variable i.
			nodePools[i].New = func() interface{} {
				return &node{
					next: make([]unsafe.Pointer, i),
				}
			}
		}(i)
	}
}

func (s *Skiplist) IncrRef() {
	atomic.AddInt32(&s.ref, 1)
}

func (s *Skiplist) DecrRef() {
	newRef := atomic.AddInt32(&s.ref, -1)
	if newRef > 0 {
		return
	}
	// Deallocate all nodes.
	x := s.head
	for x != nil {
		next := x.getNext(0)
		nodePools[len(x.next)].Put(x)
		x = next
	}
	s.arena.Reset()
	// Indicate we are closed. Good for testing.  Also, lets GC reclaim memory. Race condition
	// here would suggest we are accessing skiplist when we are supposed to have no reference!
	s.arena = nil
}

func (s *Skiplist) Valid() bool { return s.arena != nil }

func newNode(arena *Arena, key []byte, v y.ValueStruct, height int) *node {
	keyOffset := arena.PutKey(key)
	valOffset := arena.PutVal(v)
	return &node{
		keyOffset:   keyOffset,
		keySize:     uint16(len(key)),
		valueOffset: valOffset,
		valueSize:   uint16(len(v.Value)),
		next:        make([]unsafe.Pointer, height),
	}
}

func NewSkiplist(arenaSize int64) *Skiplist {
	arena := NewArena(arenaSize)
	head := newNode(arena, nil, y.ValueStruct{}, kMaxHeight)
	return &Skiplist{
		height: 1,
		head:   head,
		arena:  arena,
		ref:    1,
	}
}

func (s *node) getValueOffset() (uint32, uint16) {
	s.Lock()
	defer s.Unlock()
	return s.valueOffset, s.valueSize
}

func (s *node) key(arena *Arena) []byte {
	return arena.GetKey(s.keyOffset, s.keySize)
}

func (s *node) setValue(arena *Arena, v y.ValueStruct) {
	valOffset := arena.PutVal(v)
	s.Lock()
	defer s.Unlock()
	s.valueOffset = valOffset
	s.valueSize = uint16(len(v.Value))
}

func (s *node) getNext(h int) *node {
	return (*node)(atomic.LoadPointer(&s.next[h]))
}

func (s *node) casNext(h int, old, val *node) bool {
	return atomic.CompareAndSwapPointer(&s.next[h], unsafe.Pointer(old), unsafe.Pointer(val))
}

// Returns true if key is strictly > n.key.
// If n is nil, this is an "end" marker and we return false.
//func (s *Skiplist) keyIsAfterNode(key []byte, n *node) bool {
//	y.AssertTrue(n != s.head)
//	return n != nil && bytes.Compare(key, n.key) > 0
//}

func randomHeight() int {
	h := 1
	for h < kMaxHeight && rand.Uint32() <= kHeightIncrease {
		h++
	}
	return h
}

// findNear finds the node near to key.
// If less=true, it finds rightmost node such that node.key < key (if allowEqual=false) or
// node.key <= key (if allowEqual=true).
// If less=false, it finds leftmost node such that node.key > key (if allowEqual=false) or
// node.key >= key (if allowEqual=true).
// Returns the node found. The bool returned is true if the node has key equal to given key.
func (s *Skiplist) findNear(key []byte, less bool, allowEqual bool) (*node, bool) {
	x := s.head
	level := int(s.Height() - 1)
	for {
		// Assume x.key < key.
		next := x.getNext(level)
		if next == nil {
			// x.key < key < END OF LIST
			if level > 0 {
				// Can descend further to iterate closer to the end.
				level--
				continue
			}
			// Level=0. Cannot descend further. Let's return something that makes sense.
			if !less {
				return nil, false
			}
			// Try to return x. Make sure it is not a head node.
			if x == s.head {
				return nil, false
			}
			return x, false
		}

		nextKey := next.key(s.arena)
		cmp := bytes.Compare(key, nextKey)
		if cmp > 0 {
			// x.key < next.key < key. We can continue to move right.
			x = next
			continue
		}
		if cmp == 0 {
			// x.key < key == next.key.
			if allowEqual {
				return next, true
			}
			if !less {
				// We want >, so go to base level to grab the next bigger note.
				return next.getNext(0), false
			}
			// We want <. If not base level, we should go closer in the next level.
			if level > 0 {
				level--
				continue
			}
			// On base level. Return x.
			if x == s.head {
				return nil, false
			}
			return x, false
		}
		// cmp < 0. In other words, x.key < key < next.
		if level > 0 {
			level--
			continue
		}
		// At base level. Need to return something.
		if !less {
			return next, false
		}
		// Try to return x. Make sure it is not a head node.
		if x == s.head {
			return nil, false
		}
		return x, false
	}
}

// findSpliceForLevel returns (outBefore, outAfter) with outBefore.key <= key <= outAfter.key.
// The input "before" tells us where to start looking.
// If we found a node with the same key, then we return outBefore = outAfter.
// Otherwise, outBefore.key < key < outAfter.key.
func (s *Skiplist) findSpliceForLevel(key []byte, before *node, level int) (*node, *node) {
	for {
		// Assume before.key < key.
		next := before.getNext(level)
		if next == nil {
			return before, next
		}
		nextKey := next.key(s.arena)
		cmp := bytes.Compare(key, nextKey)
		if cmp == 0 {
			// Equality case.
			return next, next
		}
		if cmp < 0 {
			// before.key < key < next.key. We are done for this level.
			return before, next
		}
		before = next // Keep moving right on this level.
	}
}

func (s *Skiplist) Height() int32 {
	return atomic.LoadInt32(&s.height)
}

// Put inserts the key-value pair.
func (s *Skiplist) Put(key []byte, v y.ValueStruct) {
	// Since we allow overwrite, we may not need to create a new node. We might not even need to
	// increase the height. Let's defer these actions.

	listHeight := s.Height()
	var prev [kMaxHeight + 1]*node
	var next [kMaxHeight + 1]*node
	prev[listHeight] = s.head
	next[listHeight] = nil
	for i := int(listHeight) - 1; i >= 0; i-- {
		// Use higher level to speed up for current level.
		prev[i], next[i] = s.findSpliceForLevel(key, prev[i+1], i)
		if prev[i] == next[i] {
			prev[i].setValue(s.arena, v)
			return
		}
	}

	// We do need to create a new node.
	height := randomHeight()
	x := newNode(s.arena, key, v, height)

	// Try to increase s.height via CAS.
	listHeight = s.Height()
	for height > int(listHeight) {
		if atomic.CompareAndSwapInt32(&s.height, listHeight, int32(height)) {
			// Successfully increased skiplist.height.
			break
		}
		listHeight = s.Height()
	}

	// We always insert from the base level and up. After you add a node in base level, we cannot
	// create a node in the level above because it would have discovered the node in the base level.
	for i := 0; i < height; i++ {
		for {
			if prev[i] == nil {
				y.AssertTrue(i > 1) // This cannot happen in base level.
				// We haven't computed prev, next for this level because height exceeds old listHeight.
				// For these levels, we expect the lists to be sparse, so we can just search from head.
				prev[i], next[i] = s.findSpliceForLevel(key, s.head, i)
				// Someone adds the exact same key before we are able to do so. This can only happen on
				// the base level. But we know we are not on the base level.
				y.AssertTrue(prev[i] != next[i])
			}
			x.next[i] = unsafe.Pointer(next[i])
			if prev[i].casNext(i, next[i], x) {
				// Managed to insert x between prev[i] and next[i]. Go to the next level.
				break
			}
			// CAS failed. We need to recompute prev and next.
			// It is unlikely to be helpful to try to use a different level as we redo the search,
			// because it is unlikely that lots of nodes are inserted between prev[i] and next[i].
			prev[i], next[i] = s.findSpliceForLevel(key, prev[i], i)
			if prev[i] == next[i] {
				y.AssertTruef(i == 0, "Equality can happen only on base level: %d", i)
				prev[i].setValue(s.arena, v)
				return
			}
		}
	}
}

// findLast returns the last element. If head (empty list), we return nil. All the find functions
// will NEVER return the head nodes.
func (s *Skiplist) findLast() *node {
	n := s.head
	level := int(s.Height()) - 1
	for {
		next := n.getNext(level)
		if next != nil {
			n = next
			continue
		}
		if level == 0 {
			if n == s.head {
				return nil
			}
			return n
		}
		level--
	}
}

func (s *Skiplist) Get(key []byte) y.ValueStruct {
	n, found := s.findNear(key, false, true) // findGreaterOrEqual.
	if !found {
		return y.ValueStruct{}
	}
	valOffset, valSize := n.getValueOffset()
	return s.arena.GetVal(valOffset, valSize)
}

func (s *Skiplist) NewIterator() *Iterator {
	s.IncrRef()
	return &Iterator{list: s}
}

func (s *Skiplist) Size() int64 { return s.arena.Size() }

// Iterator is an iterator over skiplist object. For new objects, you just
// need to initialize Iterator.list.
type Iterator struct {
	list *Skiplist
	n    *node
}

func (s *Iterator) Close() error {
	s.list.DecrRef()
	return nil
}

// Valid returns true iff the iterator is positioned at a valid node.
func (s *Iterator) Valid() bool { return s.n != nil }

// Key returns the key at the current position.
func (s *Iterator) Key() []byte {
	return s.list.arena.GetKey(s.n.keyOffset, s.n.keySize)
}

// Value returns value.
func (s *Iterator) Value() y.ValueStruct {
	valOffset, valSize := s.n.getValueOffset()
	return s.list.arena.GetVal(valOffset, valSize)
}

// Next advances to the next position.
func (s *Iterator) Next() {
	y.AssertTrue(s.Valid())
	s.n = s.n.getNext(0)
}

// Prev advances to the previous position.
func (s *Iterator) Prev() {
	y.AssertTrue(s.Valid())
	s.n, _ = s.list.findNear(s.Key(), true, false) // find <. No equality allowed.
}

// Seek advances to the first entry with a key >= target.
func (s *Iterator) Seek(target []byte) {
	s.n, _ = s.list.findNear(target, false, true) // find >=.
}

// SeekForPrev finds an entry with key <= target.
func (s *Iterator) SeekForPrev(target []byte) {
	s.n, _ = s.list.findNear(target, true, true) // find <=.
}

// SeekToFirst seeks position at the first entry in list.
// Final state of iterator is Valid() iff list is not empty.
func (s *Iterator) SeekToFirst() {
	s.n = s.list.head.getNext(0)
}

// SeekToLast seeks position at the last entry in list.
// Final state of iterator is Valid() iff list is not empty.
func (s *Iterator) SeekToLast() {
	s.n = s.list.findLast()
}

func (s *Iterator) Name() string { return "SkiplistIterator" }

// UniIterator is a unidirectional memtable iterator. It is a thin wrapper around
// Iterator. We like to keep Iterator as before, because it is more powerful and
// we might support bidirectional iterators in the future.
type UniIterator struct {
	iter     *Iterator
	reversed bool
}

func (s *Skiplist) NewUniIterator(reversed bool) *UniIterator {
	return &UniIterator{
		iter:     s.NewIterator(),
		reversed: reversed,
	}
}

func (s *UniIterator) Next() {
	if !s.reversed {
		s.iter.Next()
	} else {
		s.iter.Prev()
	}
}

func (s *UniIterator) Rewind() {
	if !s.reversed {
		s.iter.SeekToFirst()
	} else {
		s.iter.SeekToLast()
	}
}

func (s *UniIterator) Seek(key []byte) {
	if !s.reversed {
		s.iter.Seek(key)
	} else {
		s.iter.SeekForPrev(key)
	}
}

func (s *UniIterator) Key() []byte          { return s.iter.Key() }
func (s *UniIterator) Value() y.ValueStruct { return s.iter.Value() }
func (s *UniIterator) Valid() bool          { return s.iter.Valid() }
func (s *UniIterator) Name() string         { return "UniMemtableIterator" }
func (s *UniIterator) Close() error         { return s.iter.Close() }
