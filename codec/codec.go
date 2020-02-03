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

// Msb returns the most significant bits of a UID used for the base of a UidBlock
func Msb(uid uint64) uint64 {
	return uid & msbBitMask
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

func (e *Encoder) packBlock() {
	if len(e.uids) == 0 {
		return
	}
	roaringBitmap := roaring.New()
	roaringBitmap.AddMany(e.uids)
	e.AddBlockFromBitmap(e.currentBase, roaringBitmap, uint32(len(e.uids)))
	e.uids = e.uids[:0]
}

// Add takes a UID and adds it to the list of UIDs to be encoded.
func (e *Encoder) Add(uid uint64) {
	base := uid & msbBitMask
	if e.pack == nil {
		e.pack = &pb.UidPack{}
		e.currentBase = base
	}
	lenUids := len(e.uids)
	if lenUids > 0 && e.currentBase != base {
		e.packBlock()
		e.currentBase = base
		e.uids = e.uids[:0]
	}
	e.uids = append(e.uids, uint32(uid & ^msbBitMask))
}

// Done returns the final output of the encoder.
func (e *Encoder) Done() *pb.UidPack {
	e.packBlock()
	return e.pack
}

// NewDecoder returns a decoder for the given UidPack and properly initializes it.
func NewDecoder(pack *pb.UidPack) *Decoder {
	decoder := &Decoder{
		Pack:          pack,
		RoaringBitmap: roaring.New(),
	}
	return decoder
}

// RoaringBitmapForBlock returns roaring bitmap for block at given index
func (d *Decoder) RoaringBitmapForBlock(blockIdx int) *roaring.Bitmap {
	block := d.Pack.Blocks[blockIdx]
	rb := roaring.New()
	x.Check2(rb.FromBuffer(block.Deltas))
	return rb
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
	x.Check2(d.RoaringBitmap.FromBuffer(block.Deltas))

	d.uids = make([]uint64, d.RoaringBitmap.GetCardinality())
	for i, lsb := range d.RoaringBitmap.ToArray() {
		d.uids[i] = block.Base + uint64(lsb)
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
	idx := sort.Search(len(pack.Blocks), func(i int) bool { return pack.Blocks[i].Base >= (uid & msbBitMask) })

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

/*
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
*/

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

// Encode takes in a list of uids and a block size. It packs these uids into blocks of the given
// size, with the last block having fewer uids. Within each block, it stores the first uid as base.
// For each next uid, a delta = uids[i] - uids[i-1] is stored. Protobuf uses Varint encoding,
// as mentioned here: https://developers.google.com/protocol-buffers/docs/encoding . This ensures
// that the deltas, being considerably smaller than the original uids, are packed in fewer bytes.
// Our benchmarks on artificial data show the compressed size to be 13% of the original. This
// mechanism is a LOT simpler to understand and if needed, debug.
func Encode(uids []uint64) *pb.UidPack {
	enc := Encoder{}
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
func ExactLen(pack *pb.UidPack) int {
	if pack == nil || len(pack.Blocks) == 0 {
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
	uids := make([]uint64, 0, ApproxLen(pack)) // TODO: Can we use exact length quickly?
	dec := NewDecoder(pack)

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
