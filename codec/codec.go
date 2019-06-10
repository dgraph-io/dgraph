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

	"github.com/dgraph-io/dgraph/protos/pb"
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
	bitMask uint64 = 0xffffffff00000000
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
	block := &pb.UidBlock{Base: e.uids[0], UidNum: uint32(len(e.uids))}
	last := e.uids[0]
	e.uids = e.uids[1:]

	var out bytes.Buffer
	buf := make([]byte, 17)
	tmpUids := make([]uint32, 4)
	for {
		i := 0
		for ; i < 4; i++ {
			if i >= len(e.uids) {
				// Padding with '0' because Encode4 encodes only in batch of 4.
				tmpUids[i] = 0
			} else {
				tmpUids[i] = uint32(e.uids[i] - last)
				last = e.uids[i]
			}
		}

		data := groupvarint.Encode4(buf, tmpUids)
		out.Write(data)

		// e.uids has ended and we have padded tmpUids with 0s
		if len(e.uids) <= 4 {
			e.uids = e.uids[:0]
			break
		}
		e.uids = e.uids[4:]
	}

	block.Deltas = out.Bytes()
	e.pack.Blocks = append(e.pack.Blocks, block)
}

func (e *Encoder) Add(uid uint64) {
	if e.pack == nil {
		e.pack = &pb.UidPack{BlockSize: uint32(e.BlockSize)}
	}

	size := len(e.uids)
	if size > 0 && !match32MSB(e.uids[size-1], uid) {
		e.packBlock()
		e.uids = e.uids[:0]
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

	var tmpUids [4]uint32
	deltas := block.Deltas

	// Decoding always expects the encoded byte array to be of
	// length >= 4. Padding doesn't affect the decoded values.
	deltas = append(deltas, 0, 0, 0)

	// Read back the encoded varints.
	// Due to padding of 3 '0's, it might be the case that we don't
	// completely consume the byte array. We are encoding uids in group
	// of 4, that requires atleast 5 bytes. So if we are left with deltas
	// of length < 5, those are probably the padded 0s.
	for len(deltas) >= 5 {
		groupvarint.Decode4(tmpUids[:], deltas)
		deltas = deltas[groupvarint.BytesUsed[deltas[0]]:]
		for i := 0; i < 4; i++ {
			d.uids = append(d.uids, uint64(tmpUids[i])+last)
			last += uint64(tmpUids[i])
		}
	}

	d.uids = d.uids[:block.UidNum]
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
		num += int(b.UidNum) // UidNum includes the base UID.
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

func match32MSB(num1, num2 uint64) bool {
	return (num1 & bitMask) == (num2 & bitMask)
}
