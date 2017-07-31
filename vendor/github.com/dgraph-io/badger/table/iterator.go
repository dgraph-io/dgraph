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

package table

import (
	"bytes"
	"io"
	"math"
	"sort"

	"github.com/dgraph-io/badger/y"
	"github.com/pkg/errors"
)

type BlockIterator struct {
	data    []byte
	pos     uint32
	err     error
	baseKey []byte

	key  []byte
	val  []byte
	init bool

	last header // The last header we saw.
}

func (itr *BlockIterator) Reset() {
	itr.pos = 0
	itr.err = nil
	itr.baseKey = []byte{}
	itr.key = []byte{}
	itr.val = []byte{}
	itr.init = false
	itr.last = header{}
}

func (itr *BlockIterator) Init() {
	if !itr.init {
		itr.Next()
	}
}

func (itr *BlockIterator) Valid() bool {
	return itr != nil && itr.err == nil
}

func (itr *BlockIterator) Error() error {
	return itr.err
}

func (itr *BlockIterator) Close() {}

var (
	ORIGIN  = 0
	CURRENT = 1
)

// Seek brings us to the first block element that is >= input key.
func (itr *BlockIterator) Seek(key []byte, whence int) {
	itr.err = nil

	switch whence {
	case ORIGIN:
		itr.Reset()
	case CURRENT:
	}

	var done bool
	for itr.Init(); itr.Valid(); itr.Next() {
		k := itr.Key()
		if bytes.Compare(k, key) >= 0 {
			// We are done as k is >= key.
			done = true
			break
		}
	}
	if !done {
		itr.err = io.EOF
	}
}

func (itr *BlockIterator) SeekToFirst() {
	itr.err = nil
	itr.Init()
}

// SeekToLast brings us to the last element. Valid should return true.
func (itr *BlockIterator) SeekToLast() {
	itr.err = nil
	for itr.Init(); itr.Valid(); itr.Next() {
	}
	itr.Prev()
}

// parseKV would allocate a new byte slice for key and for value.
func (itr *BlockIterator) parseKV(h header) {
	if cap(itr.key) < int(h.plen+h.klen) {
		itr.key = make([]byte, 2*(h.plen+h.klen))
	}
	itr.key = itr.key[:h.plen+h.klen]
	copy(itr.key, itr.baseKey[:h.plen])
	copy(itr.key[h.plen:], itr.data[itr.pos:itr.pos+uint32(h.klen)])
	itr.pos += uint32(h.klen)

	if itr.pos+uint32(h.vlen) > uint32(len(itr.data)) {
		itr.err = errors.Errorf("Value exceeded size of block: %d %d %d %d %v",
			itr.pos, h.klen, h.vlen, len(itr.data), h)
		return
	}
	itr.val = y.Safecopy(itr.val, itr.data[itr.pos:itr.pos+uint32(h.vlen)])
	itr.pos += uint32(h.vlen)
}

func (itr *BlockIterator) Next() {
	itr.init = true
	itr.err = nil
	if itr.pos >= uint32(len(itr.data)) {
		itr.err = io.EOF
		return
	}

	var h header
	itr.pos += uint32(h.Decode(itr.data[itr.pos:]))
	itr.last = h // Store the last header.

	if h.klen == 0 && h.plen == 0 {
		// Last entry in the table.
		itr.err = io.EOF
		return
	}

	// Populate baseKey if it isn't set yet. This would only happen for the first Next.
	if len(itr.baseKey) == 0 {
		// This should be the first Next() for this block. Hence, prefix length should be zero.
		y.AssertTrue(h.plen == 0)
		itr.baseKey = itr.data[itr.pos : itr.pos+uint32(h.klen)]
	}
	itr.parseKV(h)
}

func (itr *BlockIterator) Prev() {
	if !itr.init {
		return
	}
	itr.err = nil
	if itr.last.prev == math.MaxUint32 {
		// This is the first element of the block!
		itr.err = io.EOF
		itr.pos = 0
		return
	}

	// Move back using current header's prev.
	itr.pos = itr.last.prev

	var h header
	y.AssertTruef(itr.pos >= 0 && itr.pos < uint32(len(itr.data)), "%d %d", itr.pos, len(itr.data))
	itr.pos += uint32(h.Decode(itr.data[itr.pos:]))
	itr.parseKV(h)
	itr.last = h
}

func (itr *BlockIterator) Key() []byte {
	if itr.err != nil {
		return nil
	}
	return itr.key
}

func (itr *BlockIterator) Value() []byte {
	if itr.err != nil {
		return nil
	}
	return itr.val
}

type TableIterator struct {
	t    *Table
	bpos int
	bi   *BlockIterator
	err  error
	init bool

	// Internally, TableIterator is bidirectional. However, we only expose the
	// unidirectional functionality for now.
	reversed bool
}

func (t *Table) NewIterator(reversed bool) *TableIterator {
	t.IncrRef() // Important.
	ti := &TableIterator{t: t, reversed: reversed}
	ti.Init()
	return ti
}

func (itr *TableIterator) Close() error {
	return itr.t.DecrRef()
}

func (itr *TableIterator) reset() {
	itr.bpos = 0
	itr.err = nil
}

func (itr *TableIterator) Valid() bool {
	return itr != nil && itr.err == nil
}

func (itr *TableIterator) Error() error {
	return itr.err
}

func (itr *TableIterator) Init() {
	if !itr.init {
		itr.next()
	}
}

func (itr *TableIterator) seekToFirst() {
	numBlocks := len(itr.t.blockIndex)
	if numBlocks == 0 {
		itr.err = io.EOF
		return
	}
	itr.bpos = 0
	block, err := itr.t.block(itr.bpos)
	if err != nil {
		itr.err = err
		return
	}
	itr.bi = block.NewIterator()
	itr.bi.SeekToFirst()
	itr.err = itr.bi.Error()
}

func (itr *TableIterator) seekToLast() {
	numBlocks := len(itr.t.blockIndex)
	if numBlocks == 0 {
		itr.err = io.EOF
		return
	}
	itr.bpos = numBlocks - 1
	block, err := itr.t.block(itr.bpos)
	if err != nil {
		itr.err = err
		return
	}
	itr.bi = block.NewIterator()
	itr.bi.SeekToLast()
	itr.err = itr.bi.Error()
}

func (itr *TableIterator) seekHelper(blockIdx int, key []byte) {
	itr.bpos = blockIdx
	block, err := itr.t.block(blockIdx)
	if err != nil {
		itr.err = err
		return
	}
	itr.bi = block.NewIterator()
	itr.bi.Seek(key, ORIGIN)
	itr.err = itr.bi.Error()
}

// seekFrom brings us to a key that is >= input key.
func (itr *TableIterator) seekFrom(key []byte, whence int) {
	itr.err = nil
	switch whence {
	case ORIGIN:
		itr.reset()
	case CURRENT:
	}

	idx := sort.Search(len(itr.t.blockIndex), func(idx int) bool {
		ko := itr.t.blockIndex[idx]
		return bytes.Compare(ko.key, key) > 0
	})
	if idx == 0 {
		// The smallest key in our table is already strictly > key. We can return that.
		// This is like a SeekToFirst.
		itr.seekHelper(0, key)
		return
	}

	// block[idx].smallest is > key.
	// Since idx>0, we know block[idx-1].smallest is <= key.
	// There are two cases.
	// 1) Everything in block[idx-1] is strictly < key. In this case, we should go to the first
	//    element of block[idx].
	// 2) Some element in block[idx-1] is >= key. We should go to that element.
	itr.seekHelper(idx-1, key)
	if itr.err == io.EOF {
		// Case 1. Need to visit block[idx].
		if idx == len(itr.t.blockIndex) {
			// If idx == len(itr.t.blockIndex), then input key is greater than ANY element of table.
			// There's nothing we can do. Valid() should return false as we seek to end of table.
			return
		}
		// Since block[idx].smallest is > key. This is essentially a block[idx].SeekToFirst.
		itr.seekHelper(idx, key)
	}
	// Case 2: No need to do anything. We already did the seek in block[idx-1].
}

// seek will reset iterator and seek to >= key.
func (itr *TableIterator) seek(key []byte) {
	itr.seekFrom(key, ORIGIN)
}

// seekForPrev will reset iterator and seek to <= key.
func (itr *TableIterator) seekForPrev(key []byte) {
	// TODO: Optimize this. We shouldn't have to take a Prev step.
	itr.seekFrom(key, ORIGIN)
	if !bytes.Equal(itr.Key(), key) {
		itr.prev()
	}
}

func (itr *TableIterator) next() {
	itr.err = nil

	if itr.bpos >= len(itr.t.blockIndex) {
		itr.err = io.EOF
		return
	}

	if itr.bi == nil {
		block, err := itr.t.block(itr.bpos)
		if err != nil {
			itr.err = err
			return
		}
		itr.bi = block.NewIterator()
		itr.bi.SeekToFirst()
		return
	}

	itr.bi.Next()
	if !itr.bi.Valid() {
		itr.bpos++
		itr.bi = nil
		itr.next()
		return
	}
}

func (itr *TableIterator) prev() {
	itr.err = nil
	if itr.bpos < 0 {
		itr.err = io.EOF
		return
	}

	if itr.bi == nil {
		block, err := itr.t.block(itr.bpos)
		if err != nil {
			itr.err = err
			return
		}
		itr.bi = block.NewIterator()
		itr.bi.SeekToLast()
		return
	}

	itr.bi.Prev()
	if !itr.bi.Valid() {
		itr.bpos--
		itr.bi = nil
		itr.prev()
		return
	}
}

func (itr *TableIterator) Key() []byte {
	return itr.bi.Key()
}

func (itr *TableIterator) Value() (ret y.ValueStruct) {
	ret.DecodeEntireSlice(itr.bi.Value())
	return
}

func (s *TableIterator) Next() {
	if !s.reversed {
		s.next()
	} else {
		s.prev()
	}
}

func (s *TableIterator) Rewind() {
	if !s.reversed {
		s.seekToFirst()
	} else {
		s.seekToLast()
	}
}

func (s *TableIterator) Seek(key []byte) {
	if !s.reversed {
		s.seek(key)
	} else {
		s.seekForPrev(key)
	}
}

func (s *TableIterator) Name() string { return "TableIterator" }

type ConcatIterator struct {
	idx      int // Which iterator is active now.
	cur      *TableIterator
	iters    []*TableIterator // Corresponds to tables.
	tables   []*Table         // Disregarding reversed, this is in ascending order.
	reversed bool
}

func NewConcatIterator(tbls []*Table, reversed bool) *ConcatIterator {
	iters := make([]*TableIterator, len(tbls))
	for i := 0; i < len(tbls); i++ {
		iters[i] = tbls[i].NewIterator(reversed)
	}
	return &ConcatIterator{
		reversed: reversed,
		iters:    iters,
		tables:   tbls,
		idx:      -1, // Not really necessary because s.it.Valid()=false, but good to have.
	}
}

func (s *ConcatIterator) setIdx(idx int) {
	s.idx = idx
	if idx < 0 || idx >= len(s.iters) {
		s.cur = nil
	} else {
		s.cur = s.iters[s.idx]
	}
}

func (s *ConcatIterator) Rewind() {
	if len(s.iters) == 0 {
		return
	}
	if !s.reversed {
		s.setIdx(0)
	} else {
		s.setIdx(len(s.iters) - 1)
	}
	s.cur.Rewind()
}

func (s *ConcatIterator) Valid() bool {
	return s.cur.Valid()
}

func (s *ConcatIterator) Name() string { return "ConcatIterator" }

func (s *ConcatIterator) Key() []byte {
	return s.cur.Key()
}

func (s *ConcatIterator) Value() y.ValueStruct {
	return s.cur.Value()
}

// Seek brings us to element >= key if reversed is false. Otherwise, <= key.
func (s *ConcatIterator) Seek(key []byte) {
	var idx int
	if !s.reversed {
		idx = sort.Search(len(s.tables), func(i int) bool {
			return bytes.Compare(s.tables[i].Biggest(), key) >= 0
		})
	} else {
		n := len(s.tables)
		idx = n - 1 - sort.Search(n, func(i int) bool {
			return bytes.Compare(s.tables[n-1-i].Smallest(), key) <= 0
		})
	}
	if idx >= len(s.tables) || idx < 0 {
		s.setIdx(-1)
		return
	}
	// For reversed=false, we know s.tables[i-1].Biggest() < key. Thus, the
	// previous table cannot possibly contain key.
	s.setIdx(idx)
	s.cur.Seek(key)
}

// Next advances our concat iterator.
func (s *ConcatIterator) Next() {
	s.cur.Next()
	if s.cur.Valid() {
		// Nothing to do. Just stay with the current table.
		return
	}
	for { // In case there are empty tables.
		if !s.reversed {
			s.setIdx(s.idx + 1)
		} else {
			s.setIdx(s.idx - 1)
		}
		if s.cur == nil {
			// End of list. Valid will become false.
			return
		}
		s.cur.Rewind()
		if s.cur.Valid() {
			break
		}
	}
}

func (s *ConcatIterator) Close() error {
	for _, it := range s.iters {
		if err := it.Close(); err != nil {
			return errors.Wrap(err, "ConcatIterator")
		}
	}
	return nil
}
