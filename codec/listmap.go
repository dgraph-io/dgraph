/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
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

package codec

import (
	"sort"

	"github.com/RoaringBitmap/roaring"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
)

var (
	numMsb     uint8  = 48 // Number of most significant bits that are used as bases for UidBlocks
	msbBitMask uint64 = ((1 << numMsb) - 1) << (64 - numMsb)
	lsbBitMask uint64 = ^msbBitMask
)

type ListMap struct {
	bitmaps map[uint64]*roaring.Bitmap
}

func NewListMap(pack *pb.UidPack) *ListMap {
	lm := &ListMap{
		bitmaps: make(map[uint64]*roaring.Bitmap),
	}
	if pack == nil {
		return lm
	}
	for _, block := range pack.Blocks {
		bitmap := roaring.New()
		x.Check2(bitmap.FromBuffer(block.Deltas))
		x.AssertTrue(bitmap.GetCopyOnWrite())
		lm.bitmaps[block.Base] = bitmap
		x.AssertTrue(block.Base&lsbBitMask == 0)
	}
	return lm
}

func (lm *ListMap) ToUids() []uint64 {
	lmi := lm.NewIterator()
	uids := make([]uint64, 64)
	var result []uint64
	for {
		sz := lmi.Next(uids)
		result = append(result, uids[:sz]...)
		if sz < len(uids) {
			break
		}
	}
	return result
}

func FromListXXX(list *pb.List) *ListMap {
	lm := NewListMap(nil)
	lm.AddMany(list.GetUids())
	return lm
}

func (lm *ListMap) IsEmpty() bool {
	for _, bitmap := range lm.bitmaps {
		if !bitmap.IsEmpty() {
			return false
		}
	}
	return true
}

func (lm *ListMap) NumUids() uint64 {
	var result uint64
	for _, bitmap := range lm.bitmaps {
		result += bitmap.GetCardinality()
	}
	return result
}

type ListMapIterator struct {
	bases   []uint64
	bitmaps map[uint64]*roaring.Bitmap
	curIdx  int
	itr     roaring.ManyIntIterable
	many    []uint32
}

func (lm *ListMap) NewIterator() *ListMapIterator {
	lmi := &ListMapIterator{
		bases:   make([]uint64, 0, len(lm.bitmaps)),
		bitmaps: lm.bitmaps,
		many:    make([]uint32, 64),
	}
	if len(lm.bitmaps) == 0 {
		return lmi
	}
	for base := range lm.bitmaps {
		lmi.bases = append(lmi.bases, base)
	}
	sort.Slice(lmi.bases, func(i, j int) bool {
		return lmi.bases[i] < lmi.bases[j]
	})
	base := lmi.bases[0]
	if bitmap, ok := lmi.bitmaps[base]; ok {
		lmi.itr = bitmap.ManyIterator()
	}
	return lmi
}

func (lmi *ListMapIterator) Next(uids []uint64) int {
	if lmi == nil {
		return 0
	}
	if lmi.curIdx >= len(lmi.bases) {
		return 0
	}

	base := lmi.bases[lmi.curIdx]
	var idx int
	fill := func() int {
		if lmi.itr == nil {
			return 0
		}
		if len(uids[idx:]) < len(lmi.many) {
			lmi.many = lmi.many[:len(uids[idx:])]
		}
		out := lmi.itr.NextMany(lmi.many)
		for i := 0; i < out; i++ {
			// NOTE that we can not set the uids slice via append, etc. That would not get reflected
			// back to the caller. All we can do is to set the internal elements of the given slice.
			if idx >= len(uids) {
				break
			}
			uids[idx] = base | uint64(lmi.many[i])
			idx++
		}
		return out
	}

	for {
		sz := fill()
		if idx >= len(uids) {
			break
		}
		if sz == len(lmi.many) {
			continue
		}
		lmi.itr = nil
		lmi.curIdx++
		if lmi.curIdx >= len(lmi.bases) {
			break
		}
		base = lmi.bases[lmi.curIdx]
		if bitmap, ok := lmi.bitmaps[base]; ok {
			lmi.itr = bitmap.ManyIterator()
		} else {
			panic("This base should be present")
		}
	}
	return idx
}

func (lm *ListMap) ToPack() *pb.UidPack {
	pack := &pb.UidPack{
		// NumUids: lm.NumUids(),
	}
	for base, bitmap := range lm.bitmaps {
		data, err := bitmap.ToBytes()
		x.Check(err)
		block := &pb.UidBlock{
			Base:   base,
			Deltas: data,
		}
		pack.Blocks = append(pack.Blocks, block)
	}
	sort.Slice(pack.Blocks, func(i, j int) bool {
		return pack.Blocks[i].Base < pack.Blocks[j].Base
	})
	return pack
}

func (lm *ListMap) AddOne(uid uint64) {
	base := uid & msbBitMask
	bitmap, ok := lm.bitmaps[base]
	if !ok {
		bitmap = roaring.New()
		lm.bitmaps[base] = bitmap
	}
	bitmap.Add(uint32(uid & lsbBitMask))
}

func (lm *ListMap) RemoveOne(uid uint64) {
	base := uid & msbBitMask
	if bitmap, ok := lm.bitmaps[base]; ok {
		bitmap.Remove(uint32(uid & lsbBitMask))
	}
}

func (lm *ListMap) AddMany(uids []uint64) {
	for _, uid := range uids {
		lm.AddOne(uid)
	}
}

func PackOfOne(uid uint64) *pb.UidPack {
	lm := NewListMap(nil)
	lm.AddOne(uid)
	return lm.ToPack()
}

func (lm *ListMap) Add(block *pb.UidBlock) error {
	bitmap, ok := lm.bitmaps[block.Base]
	if !ok {
		return nil
	}
	dst := roaring.New()
	if _, err := dst.FromBuffer(block.Deltas); err != nil {
		return err
	}
	bitmap.Or(dst)
	return nil
}

func (lm *ListMap) Intersect(a2 *ListMap) {
	if a2 == nil || len(a2.bitmaps) == 0 {
		// a2 might be empty. In that case, just ignore.
		return
	}
	for base, bitmap := range lm.bitmaps {
		if a2Map, ok := a2.bitmaps[base]; !ok {
			// a2 does not have this base. So, remove.
			delete(lm.bitmaps, base)
		} else {
			bitmap.And(a2Map)
		}
	}
}

func (lm *ListMap) Merge(a2 *ListMap) {
	if a2 == nil || len(a2.bitmaps) == 0 {
		// a2 might be empty. In that case, just ignore.
		return
	}
	for a2base, a2map := range a2.bitmaps {
		if bitmap, ok := lm.bitmaps[a2base]; ok {
			bitmap.Or(a2map)
		} else {
			// lm does not have this bitmap. So, add.
			lm.bitmaps[a2base] = a2map
		}
	}
}

func (lm *ListMap) RemoveBefore(uid uint64) {
	if uid == 0 {
		return
	}
	uidBase := uid & msbBitMask
	// Iteration is not in serial order. So, can't break early.
	for base, bitmap := range lm.bitmaps {
		if base < uidBase {
			delete(lm.bitmaps, base)
		} else if base == uidBase {
			bitmap.RemoveRange(0, uid&lsbBitMask)
		}
	}
}
