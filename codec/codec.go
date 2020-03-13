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
	"sort"

	"github.com/RoaringBitmap/roaring"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
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
	currentBase uint64
	pack        *pb.UidPack
	uids        []uint32
}

// Decoder is used to read a pb.UidPack object back into a list of UIDs.
type Decoder struct {
	Pack          *pb.UidPack
	blockIdx      int
	uids          []uint64
	RoaringBitmap *roaring.Bitmap
}

// UIDSet stores UIDs which are split into two parts. The most significant bits are used as keys in
// the map, and the least significant bits are stored in the values of the map -- roaring bitmaps.
type UIDSet struct {
	bitmaps map[uint64]*roaring.Bitmap
}

// NewUIDSet returns a UIDSet given a UidPack
func NewUIDSet(pack *pb.UidPack) *UIDSet {
	uidSet := &UIDSet{
		bitmaps: make(map[uint64]*roaring.Bitmap),
	}
	if pack != nil {
		for _, block := range pack.Blocks {
			bitmap := roaring.New()
			x.Check2(bitmap.FromBuffer(block.Deltas))
			uidSet.bitmaps[block.Base] = bitmap
			x.AssertTrue(block.Base&lsbBitMask == 0)
		}
	}
	return uidSet
}

// ToUids returns an array of contained UIDs
func (uidSet *UIDSet) ToUids() []uint64 {
	uidSetIt := uidSet.NewIterator()
	uids := make([]uint64, 64)
	var result []uint64

	sz := uidSetIt.Next(uids)
	for ; sz > 0; sz = uidSetIt.Next(uids) {
		result = append(result, uids[:sz]...)
	}
	return result
}

// UIDSetFromList returns a UidSet given a List
func UIDSetFromList(list *pb.List) *UIDSet {
	uidSet := NewUIDSet(nil)
	uidSet.AddMany(list.Uids)
	return uidSet
}

// IsEmpty returns whether the UidSet is empty
func (uidSet *UIDSet) IsEmpty() bool {
	for _, bitmap := range uidSet.bitmaps {
		if !bitmap.IsEmpty() {
			return false
		}
	}
	return true
}

// NumUids
func (uidSet *UIDSet) NumUids() uint64 {
	var result uint64
	for _, bitmap := range uidSet.bitmaps {
		result += bitmap.GetCardinality()
	}
	return result
}

type UidSetIterator struct {
	bases       []uint64
	bitmaps     *map[uint64]*roaring.Bitmap
	curIdx      int
	roaringIter roaring.ManyIntIterable
	uidsLsbs    []uint32
}

// NewIterator
func (uidSet *UIDSet) NewIterator() *UidSetIterator {
	uidSetIt := &UidSetIterator{}
	for base := range uidSet.bitmaps {
		uidSetIt.bases = append(uidSetIt.bases, base)
	}
	sort.Slice(uidSetIt.bases, func(i, j int) bool {
		return uidSetIt.bases[i] < uidSetIt.bases[j]
	})
	if len(uidSetIt.bases) == 0 {
		return nil
	}
	base := uidSetIt.bases[0]
	uidSetIt.bitmaps = &uidSet.bitmaps
	if bitmap, ok := (*uidSetIt.bitmaps)[base]; ok {
		uidSetIt.roaringIter = bitmap.ManyIterator()
	}
	return uidSetIt
}

// Seek jumps to the first UID at or after the given UID
// func (uidSetIt *UidSetIterator) Seek(uids []uint64, uid uint64, inclusive bool) int {}

// Next
func (uidSetIter *UidSetIterator) Next(uids []uint64) int {
	if uidSetIter == nil || uidSetIter.curIdx >= len(uidSetIter.bases) {
		return 0
	}

	// Adjust size of uidSetIt.many.
	if len(uids) > cap(uidSetIter.uidsLsbs) {
		uidSetIter.uidsLsbs = make([]uint32, 0, len(uids))
	}
	uidSetIter.uidsLsbs = uidSetIter.uidsLsbs[:len(uids)]

	fill := func(base uint64) int {
		if uidSetIter.roaringIter == nil {
			return 0
		}
		out := uidSetIter.roaringIter.NextMany(uidSetIter.uidsLsbs)
		for i := 0; i < out; i++ {
			// NOTE that we can not set the uids slice via append, etc. That would not get reflected
			// back to the caller. All we can do is to set the internal elements of the given slice.
			uids[i] = base | uint64(uidSetIter.uidsLsbs[i])
		}
		return out
	}

	base := uidSetIter.bases[uidSetIter.curIdx]
	for uidSetIter.curIdx < len(uidSetIter.bases) {
		sz := fill(base)
		if sz > 0 {
			return sz
		}
		uidSetIter.roaringIter = nil
		uidSetIter.curIdx++
		if uidSetIter.curIdx < len(uidSetIter.bases) {
			base = uidSetIter.bases[uidSetIter.curIdx]
			if bitmap, ok := (*uidSetIter.bitmaps)[base]; ok {
				uidSetIter.roaringIter = bitmap.ManyIterator()
			}
		}
	}
	uids = nil
	return 0
}

func (uidSet *UIDSet) ToPack() *pb.UidPack {
	pack := &pb.UidPack{
		NumUids: uidSet.NumUids(),
	}
	for base, bitmap := range uidSet.bitmaps {
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

func (uidSet *UIDSet) AddUID(uid uint64) {
	base := uid & msbBitMask
	bitmap, ok := uidSet.bitmaps[base]
	if !ok {
		bitmap = roaring.New()
		uidSet.bitmaps[base] = bitmap
	}
	bitmap.Add(uint32(uid & lsbBitMask))
}

func (uidSet *UIDSet) RemoveOne(uid uint64) {
	base := uid & msbBitMask
	if bitmap, ok := uidSet.bitmaps[base]; ok {
		bitmap.Remove(uint32(uid & lsbBitMask))
	}
}

func (uidSet *UIDSet) AddMany(uids []uint64) {
	for _, uid := range uids {
		uidSet.AddUID(uid)
	}

}

func (uidSet *UIDSet) Intersect(a2 *UIDSet) {
	if a2 == nil || len(a2.bitmaps) == 0 {
		// a2 might be empty. In that case, just ignore.
		return
	}
	for base, bitmap := range uidSet.bitmaps {
		if a2Map, ok := a2.bitmaps[base]; !ok {
			// a2 does not have this base. So, remove.
			delete(uidSet.bitmaps, base)
		} else {
			bitmap.And(a2Map)
		}
	}
}

func (uidSet *UIDSet) Merge(a2 *UIDSet) {
	if a2 == nil || len(a2.bitmaps) == 0 {
		// a2 might be empty. In that case, just ignore.
		return
	}
	for a2base, a2map := range a2.bitmaps {
		if bitmap, ok := uidSet.bitmaps[a2base]; ok {
			bitmap.Or(a2map)
		} else {
			// uidSet does not have this bitmap. So, add.
			uidSet.bitmaps[a2base] = a2map
		}
	}
}

// RemoveBefore
func (uidSet *UIDSet) RemoveBefore(uid uint64) {
	if uid == 0 {
		return
	}
	uidBase := uid & msbBitMask
	// Iteration is not in serial order. So, can't break early.
	for base, bitmap := range uidSet.bitmaps {
		if base < uidBase {
			delete(uidSet.bitmaps, base)
		} else if base == uidBase {
			bitmap.RemoveRange(0, uid&lsbBitMask)
		}
	}
}

// PackOfOne
func PackOfOne(uid uint64) *pb.UidPack {
	uidSet := NewUIDSet(nil)
	uidSet.AddUID(uid)
	return uidSet.ToPack()
}

// AddBlock
func (uidSet *UIDSet) AddBlock(block *pb.UidBlock) error {
	bitmap, ok := uidSet.bitmaps[block.Base]
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

// Msb returns the most significant bits of a UID used for the base of a UidBlock
func Msb(uid uint64) uint64 {
	return uid & msbBitMask
}

func NewEncoder() *Encoder {
	encoder := &Encoder{
		pack: &pb.UidPack{},
	}
	return encoder
}

// Add takes a UID and adds it to the list of UIDs to be encoded.
func (e *Encoder) Add(uid uint64) {
	base := uid & msbBitMask
	if e.pack == nil {
		e.pack = &pb.UidPack{}
	}
	lenUids := len(e.uids)
	if lenUids > 0 && e.currentBase != base {
		e.packBlock()
		e.uids = e.uids[:0]
	}
	e.currentBase = base
	e.uids = append(e.uids, uint32(uid & ^msbBitMask))
}

// AddBlock appends the given block to the Encoder pack
func (e *Encoder) AddBlock(block *pb.UidBlock) {
	if e.pack == nil {
		e.pack = &pb.UidPack{}
	}
	e.pack.Blocks = append(e.pack.Blocks, block)
}

// AddBlockFromBitmap appends a new block given a roaring bitmap and base
func (e *Encoder) AddBlockFromBitmap(base uint64, rb *roaring.Bitmap, size uint32) {
	if e.pack == nil {
		e.pack = &pb.UidPack{}
	}
	encData, err := rb.ToBytes()
	x.Check(err)
	block := &pb.UidBlock{
		Base:    base,
		Deltas:  encData,
		NumUids: size,
	}
	e.pack.Blocks = append(e.pack.Blocks, block)
}

// Done returns the final output of the encoder.
func (e *Encoder) Done() *pb.UidPack {
	e.packBlock()
	return e.pack
}

func (e *Encoder) packBlock() {
	if len(e.uids) == 0 {
		return
	}
	roaringBitmap := roaring.New()
	roaringBitmap.AddMany(e.uids)
	e.AddBlockFromBitmap(e.currentBase, roaringBitmap, uint32(len(e.uids)))
	e.uids = e.uids[:0]
}

// NewDecoder returns a decoder for the given UidPack and properly initializes it.
func NewDecoder(pack *pb.UidPack) *Decoder {
	decoder := &Decoder{
		Pack:          pack,
		RoaringBitmap: roaring.New(),
	}
	decoder.Seek(0, SeekStart)
	return decoder
}

// CurrentBase returns the base of the current block
func (d *Decoder) CurrentBase() uint64 {
	return d.Pack.Blocks[d.blockIdx].Base
}

// CurrentBlock returns the current block
func (d *Decoder) CurrentBlock() *pb.UidBlock {
	return d.Pack.Blocks[d.blockIdx]
}

// UnpackBlockRoaringBitmap returns roaring bitmap for block
func (d *Decoder) UnpackBlockRoaringBitmap() *roaring.Bitmap {
	block := d.Pack.Blocks[d.blockIdx]
	x.Check2(d.RoaringBitmap.FromBuffer(block.Deltas))
	return d.RoaringBitmap
}

// RoaringBitmapForBlock returns roaring bitmap for block at given index
func (d *Decoder) RoaringBitmapForBlock(blockIdx int) *roaring.Bitmap {
	block := d.Pack.Blocks[blockIdx]
	x.Check2(d.RoaringBitmap.FromBuffer(block.Deltas))
	return d.RoaringBitmap
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

	d.UnpackBlockRoaringBitmap()
	d.uids = make([]uint64, d.RoaringBitmap.GetCardinality())
	for i, lsb := range d.RoaringBitmap.ToArray() {
		d.uids[i] = d.CurrentBase() + uint64(lsb)
	}

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
	idx := sort.Search(len(pack.Blocks), func(i int) bool {
		return pack.Blocks[i].Base >= (uid & msbBitMask)
	})

	if idx == len(pack.Blocks) {
		return []uint64{}
	}

	d.blockIdx = idx
	d.UnpackBlock()

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

// Encode takes in a list of uids and packs them into blocks. Within each block, all UIDs share the
// same most significant bits (of length `numMsb`, stored as `base` in the block), and the remaining
// least significant bits of each UID is stored in a roaring bitmap.
func Encode(uids []uint64) *pb.UidPack {
	enc := NewEncoder()
	for _, uid := range uids {
		enc.Add(uid)
	}
	return enc.Done()
}

// ApproxLen returns the approximate number of UIDs in the pack. Can be used for int slice
// allocations.
func ApproxLen(pack *pb.UidPack) int {
	if pack == nil {
		return 0
	}
	return len(pack.Blocks) * int(pack.BlockSize)
}

// ExactLen returns the total number of UIDs. Instead of using a UidPack, it accepts blocks,
// so we can calculate the number of uids after a seek.
// TODO: Can we store exact len in uidpack?
func ExactLen(pack *pb.UidPack) int {
	if pack == nil || len(pack.Blocks) == 0 {
		return 0
	}
	num := 0
	for _, b := range pack.Blocks {
		num += int(b.NumUids)
	}
	return num
}

// Decode decodes the UidPack back into the list of uids. This is a stop-gap function, Decode would
// need to do more specific things than just return the list back.
func Decode(pack *pb.UidPack, seek uint64) []uint64 {
	if pack == nil {
		return []uint64{}
	}
	uids := make([]uint64, 0, ExactLen(pack))
	dec := NewDecoder(pack)
	for block := dec.Seek(seek, SeekStart); dec.Valid(); block = dec.Next() {
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
