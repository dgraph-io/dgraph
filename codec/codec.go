/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
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
	"bytes"
	"math"
	"sort"

	"github.com/RoaringBitmap/roaring"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgryski/go-groupvarint"
)

type seekPos int

const (
	// SeekStart is used with Seek() to search relative to the Uid, returning it in the results.
	SeekStart seekPos = iota
	// SeekCurrent to Seek() a Uid using it as offset, not as part of the results.
	SeekCurrent
)

var (
	numMsb     uint8  = 48 // Number of most significant bits that are used as bases for UidBlocks
	msbBitMask uint64 = ((1 << numMsb) - 1) << (64 - numMsb)
	lsbBitMask uint64 = ^msbBitMask
)

// Encoder is used to convert a list of UIDs into a pb.UidPack object.
type Encoder struct {
	BlockSize int
	pack      *pb.UidPack
	uids      []uint64
}

type ListMap struct {
	bitmaps map[uint64]*roaring.Bitmap
}

func NewListMap(pack *pb.UidPack) *ListMap {
	lm := &ListMap{
		bitmaps: make(map[uint64]*roaring.Bitmap),
	}
	if pack != nil {
		for _, block := range pack.Blocks {
			bitmap := roaring.New()
			x.Check2(bitmap.FromBuffer(block.Deltas))
			lm.bitmaps[block.Base] = bitmap
			x.AssertTrue(block.Base&lsbBitMask == 0)
		}
	}
	return lm
}

func FromListXXX(list *pb.List) *ListMap {
	lm := NewListMap(nil)
	lm.AddMany(list.Uids)
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

func (lm *ListMap) ToPack() *pb.UidPack {
	pack := &pb.UidPack{}
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

// Add takes an uid and adds it to the list of UIDs to be encoded.
func (e *Encoder) Add(uid uint64) {
	if e.pack == nil {
		e.pack = &pb.UidPack{BlockSize: uint32(e.BlockSize)}
	}
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

// // TODO: This might have an issue. If the intersection is done with a UidPack of fewer blocks, then
// // the remaining blocks need to be removed from listmap.
// func (lm *ListMap) Intersect(block *pb.UidBlock) error {
// 	bitmap, ok := lm.bitmaps[block.Base]
// 	if !ok {
// 		return nil
// 	}
// 	dst := roaring.New()
// 	if _, err := dst.FromBuffer(block.Deltas); err != nil {
// 		return err
// 	}
// 	bitmap.And(dst)
// 	return nil
// }

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

// Done returns the final output of the encoder.
func (e *Encoder) Done() *pb.UidPack {
	return e.pack
}

// Decoder is used to read a pb.UidPack object back into a list of UIDs.
type Decoder struct {
	Pack     *pb.UidPack
	blockIdx int
	uids     []uint64
}

// NewDecoder returns a decoder for the given UidPack and properly initializes it.
func NewDecoder(pack *pb.UidPack) *Decoder {
	decoder := &Decoder{
		Pack: pack,
	}
	decoder.Seek(0, SeekStart)
	return decoder
}

func (d *Decoder) UnpackBlock() []uint64 {
	if len(d.uids) > 0 {
		// We were previously preallocating the d.uids slice to block size. This caused slowdown
		// because many blocks are small and only contain a few ints, causing wastage while still
		// paying cost of allocation.
		d.uids = d.uids[:0]
	}

	if d.blockIdx >= len(d.Pack.Blocks) {
		return d.uids
	}
	block := d.Pack.Blocks[d.blockIdx]

	last := block.Base
	d.uids = append(d.uids, last)

	tmpUids := make([]uint32, 4)
	var sum uint64
	encData := block.Deltas

	for uint32(len(d.uids)) < block.NumUids {
		if len(encData) < 17 {
			// Decode4 decodes 4 uids from encData. It moves slice(encData) forward while
			// decoding and expects it to be of length >= 4 at all the stages.
			// The SSE code tries to read 16 bytes past the header(1 byte).
			// So we are padding encData to increase its length to 17 bytes.
			// This is a workaround for https://github.com/dgryski/go-groupvarint/issues/1
			encData = append(encData, bytes.Repeat([]byte{0}, 17-len(encData))...)
		}

		groupvarint.Decode4(tmpUids, encData)
		encData = encData[groupvarint.BytesUsed[encData[0]]:]
		for i := 0; i < 4; i++ {
			sum = last + uint64(tmpUids[i])
			d.uids = append(d.uids, sum)
			last = sum
		}
	}

	d.uids = d.uids[:block.NumUids]
	return d.uids
}

// ApproxLen returns the approximate number of UIDs in the pb.UidPack object.
func (d *Decoder) ApproxLen() int {
	return int(d.Pack.BlockSize) * (len(d.Pack.Blocks) - d.blockIdx)
}

type searchFunc func(int) bool

// Seek will search for uid in a packed block using the specified whence position.
// The value of whence must be one of the predefined values SeekStart or SeekCurrent.
// SeekStart searches uid and includes it as part of the results.
// SeekCurrent searches uid but only as offset, it won't be included with results.
//
// Returns a slice of all uids whence the position, or an empty slice if none found.
func (d *Decoder) Seek(uid uint64, whence seekPos) []uint64 {
	if d.Pack == nil {
		return []uint64{}
	}
	d.blockIdx = 0
	if uid == 0 {
		return d.UnpackBlock()
	}

	pack := d.Pack
	blocksFunc := func() searchFunc {
		var f searchFunc
		switch whence {
		case SeekStart:
			f = func(i int) bool { return pack.Blocks[i].Base >= uid }
		case SeekCurrent:
			f = func(i int) bool { return pack.Blocks[i].Base > uid }
		}
		return f
	}

	idx := sort.Search(len(pack.Blocks), blocksFunc())
	// The first block.Base >= uid.
	if idx == 0 {
		return d.UnpackBlock()
	}
	// The uid is the first entry in the block.
	if idx < len(pack.Blocks) && pack.Blocks[idx].Base == uid {
		d.blockIdx = idx
		return d.UnpackBlock()
	}

	// Either the idx = len(pack.Blocks) that means it wasn't found in any of the block's base. Or,
	// we found the first block index whose base is greater than uid. In these cases, go to the
	// previous block and search there.
	d.blockIdx = idx - 1 // Move to the previous block. If blockIdx<0, unpack will deal with it.
	d.UnpackBlock()      // And get all their uids.

	uidsFunc := func() searchFunc {
		var f searchFunc
		switch whence {
		case SeekStart:
			f = func(i int) bool { return d.uids[i] >= uid }
		case SeekCurrent:
			f = func(i int) bool { return d.uids[i] > uid }
		}
		return f
	}

	// uidx points to the first uid in the uid list, which is >= uid.
	uidx := sort.Search(len(d.uids), uidsFunc())
	if uidx < len(d.uids) { // Found an entry in uids, which >= uid.
		d.uids = d.uids[uidx:]
		return d.uids
	}
	// Could not find any uid in the block, which is >= uid. The next block might still have valid
	// entries > uid.
	return d.Next()
}

// Uids returns all the uids in the pb.UidPack object as an array of integers.
// uids are owned by the Decoder, and the slice contents would be changed on the next call. They
// should be copied if passed around.
func (d *Decoder) Uids() []uint64 {
	return d.uids
}

// LinearSeek returns uids of the last block whose base is less than seek.
// If there are no such blocks i.e. seek < base of first block, it returns uids of first
// block. LinearSeek is used to get closest uids which are >= seek.
func (d *Decoder) LinearSeek(seek uint64) []uint64 {
	for {
		v := d.PeekNextBase()
		if seek < v {
			break
		}
		d.blockIdx++
	}

	return d.UnpackBlock()
}

// PeekNextBase returns the base of the next block without advancing the decoder.
func (d *Decoder) PeekNextBase() uint64 {
	bidx := d.blockIdx + 1
	if bidx < len(d.Pack.Blocks) {
		return d.Pack.Blocks[bidx].Base
	}
	return math.MaxUint64
}

// Valid returns true if the decoder has not reached the end of the packed data.
func (d *Decoder) Valid() bool {
	return d.blockIdx < len(d.Pack.Blocks)
}

// Next moves the decoder on to the next block.
func (d *Decoder) Next() []uint64 {
	d.blockIdx++
	return d.UnpackBlock()
}

// BlockIdx returns the index of the block that is currently being decoded.
func (d *Decoder) BlockIdx() int {
	return d.blockIdx
}

func Encode(uids []uint64, blockSize int) *pb.UidPack {
	return nil
}

// ApproxLen would indicate the total number of UIDs in the pack. Can be used for int slice
// allocations.
func ApproxLen(pack *pb.UidPack) int {
	if pack == nil {
		return 0
	}
	return len(pack.Blocks) * int(pack.BlockSize)
}

// ExactLen would calculate the total number of UIDs. Instead of using a UidPack, it accepts blocks,
// so we can calculate the number of uids after a seek.
func ExactLen(pack *pb.UidPack) int {
	if pack == nil {
		return 0
	}
	sz := len(pack.Blocks)
	if sz == 0 {
		return 0
	}
	num := 0
	for _, b := range pack.Blocks {
		num += int(b.NumUids) // NumUids includes the base UID.
	}
	return num
}

// Decode decodes the UidPack back into the list of uids. This is a stop-gap function, Decode would
// need to do more specific things than just return the list back.
func Decode(pack *pb.UidPack, seek uint64) []uint64 {
	uids := make([]uint64, 0, ApproxLen(pack))
	dec := Decoder{Pack: pack}

	for block := dec.Seek(seek, SeekStart); len(block) > 0; block = dec.Next() {
		uids = append(uids, block...)
	}
	return uids
}

// CopyUidPack creates a copy of the given UidPack.
func CopyUidPack(pack *pb.UidPack) *pb.UidPack {
	if pack == nil {
		return nil
	}

	packCopy := new(pb.UidPack)
	packCopy.BlockSize = pack.BlockSize
	packCopy.Blocks = make([]*pb.UidBlock, len(pack.Blocks))

	for i, block := range pack.Blocks {
		packCopy.Blocks[i] = new(pb.UidBlock)
		packCopy.Blocks[i].Base = block.Base
		packCopy.Blocks[i].NumUids = block.NumUids
		packCopy.Blocks[i].Deltas = make([]byte, len(block.Deltas))
		copy(packCopy.Blocks[i].Deltas, block.Deltas)
	}

	return packCopy
}
