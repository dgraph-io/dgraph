/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
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
	"github.com/dgraph-io/dgraph/protos/pb"
)

// UidPackIterator is a Wrapper around Decoder to allow simplified iteration over
// a UidPack.
type UidPackIterator struct {
	pack      *pb.UidPack
	blockSize int
	decoder   *Decoder
	uidIdx    int
}

// NewUidPackIterator returns a new iterator from the beginning of the UidPack.
func NewUidPackIterator(pack *pb.UidPack) *UidPackIterator {
	it := &UidPackIterator{
		pack:    pack,
		decoder: &Decoder{Pack: pack},
		blockSize: int(pack.GetBlockSize()),
	}
	it.decoder.Seek(0, SeekStart)
	return it
}

// Get retrieves the uid at the current location.
func (it *UidPackIterator) Get() uint64 {
	return it.decoder.uids[it.uidIdx]
}

// Next advances the iterator by one step.
func (it *UidPackIterator) Next() {
	it.uidIdx++
}

// ValidBlock returns false if the uid counter is past the end of the current block.
func (it *UidPackIterator) ValidBlock() bool {
	return it.uidIdx < it.blockSize
}

// NextBlock decodes the next block and resets the uid counter.
func (it *UidPackIterator) NextBlock() {
	it.decoder.Next()
	it.uidIdx = 0
}

// Valid returns whether the iterator is at a valid position.
func (it *UidPackIterator) Valid() bool {
	if !it.decoder.Valid() {
		return false
	}

	if it.decoder.blockIdx == len(it.pack.Blocks) &&
		it.uidIdx == len(it.decoder.uids) {
		return false
	}

	return true
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
