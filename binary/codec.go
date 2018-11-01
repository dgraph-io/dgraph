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

package binary

import (
	"sort"

	"github.com/dgraph-io/dgraph/protos/pb"
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
	for _, uid := range e.uids[1:] {
		block.Deltas = append(block.Deltas, uid-last)
		last = uid
	}
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
}

func (d *Decoder) unpackBlock() []uint64 {
	if d.blockIdx >= len(d.Pack.Blocks) {
		return []uint64{}
	}

	uids := make([]uint64, 0, d.Pack.BlockSize)
	block := d.Pack.Blocks[d.blockIdx]

	last := block.Base
	uids = append(uids, last)
	for _, delta := range block.Deltas {
		uid := last + delta
		uids = append(uids, uid)
		last = uid
	}
	return uids
}

func (d *Decoder) Seek(uid uint64) []uint64 {
	if d.Pack == nil {
		return []uint64{}
	}
	d.blockIdx = 0
	if uid == 0 {
		return d.unpackBlock()
	}

	pack := d.Pack
	idx := sort.Search(len(pack.Blocks), func(i int) bool {
		return pack.Blocks[i].Base >= uid
	})
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
	d.blockIdx = idx - 1    // Move to the previous block. If blockIdx<0, unpack will deal with it.
	uids := d.unpackBlock() // And get all their uids.

	// uidx points to the first uid in the uid list, which is >= uid.
	uidx := sort.Search(len(uids), func(i int) bool {
		return uids[i] >= uid
	})
	if uidx < len(uids) { // Found an entry in uids, which >= uid.
		return uids[uidx:]
	}
	// Could not find any uid in the block, which is >= uid. The next block might still have valid
	// entries > uid.
	return d.Next()
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

// NumUids returns the number of uids stored in a UidPack.
func NumUids(pack *pb.UidPack) int {
	sz := len(pack.Blocks)
	if sz == 0 {
		return 0
	}
	lastBlock := pack.Blocks[sz-1]
	return (sz-1)*int(pack.BlockSize) + len(lastBlock.Deltas) + 1 // We don't store base in deltas.
}

// Decode decodes the UidPack back into the list of uids. This is a stop-gap function, Decode would
// need to do more specific things than just return the list back.
func Decode(pack *pb.UidPack, seek uint64) []uint64 {
	uids := make([]uint64, 0, NumUids(pack))
	dec := Decoder{Pack: pack}

	for block := dec.Seek(seek); len(block) > 0; block = dec.Next() {
		uids = append(uids, block...)
	}
	return uids
}
