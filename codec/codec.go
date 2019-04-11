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
	"encoding/binary"
	"math"
	"sort"

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

type Encoder struct {
	BlockSize int
	pack      *pb.UidPack
	uids      []uint64
}

func (e *Encoder) packBlock() {
	if len(e.uids) == 0 {
		return
	}
	block := &pb.UidBlock{Base: e.uids[0]}
	last := e.uids[0]

	count := 1
	var out bytes.Buffer
	var buf [binary.MaxVarintLen64]byte
	for _, uid := range e.uids[1:] {
		n := binary.PutUvarint(buf[:], uid-last)
		x.Check2(out.Write(buf[:n]))
		last = uid
		count++
	}
	block.Deltas = out.Bytes()
	e.pack.Blocks = append(e.pack.Blocks, block)
}

func (e *Encoder) Add(uid uint64) {
	if e.pack == nil {
		e.pack = &pb.UidPack{BlockSize: uint32(e.BlockSize)}
	}
	e.uids = append(e.uids, uid)
	if len(e.uids) >= e.BlockSize {
		e.packBlock()
		e.uids = e.uids[:0]
	}
}

func (e *Encoder) Done() *pb.UidPack {
	e.packBlock()
	return e.pack
}

type Decoder struct {
	Pack     *pb.UidPack
	blockIdx int
	uids     []uint64
}

func (d *Decoder) unpackBlock() []uint64 {
	if cap(d.uids) == 0 {
		d.uids = make([]uint64, 0, d.Pack.BlockSize)
	} else {
		d.uids = d.uids[:0]
	}

	if d.blockIdx >= len(d.Pack.Blocks) {
		return d.uids
	}
	block := d.Pack.Blocks[d.blockIdx]

	last := block.Base
	d.uids = append(d.uids, last)

	// Read back the encoded varints.
	var offset int
	for offset < len(block.Deltas) {
		delta, n := binary.Uvarint(block.Deltas[offset:])
		x.AssertTrue(n > 0)
		offset += n
		uid := last + delta
		d.uids = append(d.uids, uid)
		last = uid
	}
	return d.uids
}

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
		return d.unpackBlock()
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
		return d.unpackBlock()
	}
	// The uid is the first entry in the block.
	if idx < len(pack.Blocks) && pack.Blocks[idx].Base == uid {
		d.blockIdx = idx
		return d.unpackBlock()
	}

	// Either the idx = len(pack.Blocks) that means it wasn't found in any of the block's base. Or,
	// we found the first block index whose base is greater than uid. In these cases, go to the
	// previous block and search there.
	d.blockIdx = idx - 1 // Move to the previous block. If blockIdx<0, unpack will deal with it.
	d.unpackBlock()      // And get all their uids.

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

// Uids are owned by the Decoder, and the slice contents would be changed on the next call. They
// should be copied if passed around.
func (d *Decoder) Uids() []uint64 {
	return d.uids
}

func (d *Decoder) LinearSeek(seek uint64) []uint64 {
	prev := d.blockIdx
	for {
		v := d.PeekNextBase()
		if seek <= v {
			break
		}
		d.blockIdx++
	}
	if d.blockIdx == prev {
		// The seek id is <= base of next block. But, we have already searched this
		// block. So, let's move to the next block anyway.
		return d.Next()
	}
	return d.unpackBlock()
}

func (d *Decoder) PeekNextBase() uint64 {
	bidx := d.blockIdx + 1
	if bidx < len(d.Pack.Blocks) {
		return d.Pack.Blocks[bidx].Base
	}
	return math.MaxUint64
}

func (d *Decoder) Valid() bool {
	return d.blockIdx < len(d.Pack.Blocks)
}

func (d *Decoder) Next() []uint64 {
	d.blockIdx++
	return d.unpackBlock()
}

// Encode takes in a list of uids and a block size. It would pack these uids into blocks of the
// given size, with the last block having fewer uids. Within each block, it stores the first uid as
// base. For each next uid, a delta = uids[i] - uids[i-1] is stored. Protobuf uses Varint encoding,
// as mentioned here: https://developers.google.com/protocol-buffers/docs/encoding . This ensures
// that the deltas being considerably smaller than the original uids are nicely packed in fewer
// bytes. Our benchmarks on artificial data show compressed size to be 13% of the original. This
// mechanism is a LOT simpler to understand and if needed, debug.
func Encode(uids []uint64, blockSize int) *pb.UidPack {
	enc := Encoder{BlockSize: blockSize}
	for _, uid := range uids {
		enc.Add(uid)
	}
	return enc.Done()
}

// ApproxNum would indicate the total number of UIDs in the pack. Can be used for int slice
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
	block := pack.Blocks[sz-1]
	num := 1 // Count the base.
	for _, b := range block.Deltas {
		// If the MSB in varint encoding is zero, then it is the final byte, not a continuation of
		// the integer. Thus, we can count it as one delta.
		if b&0x80 == 0 {
			num++
		}
	}
	num += (sz - 1) * int(pack.BlockSize)
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
