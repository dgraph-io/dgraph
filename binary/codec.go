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
	"github.com/dgraph-io/dgraph/protos/pb"
)

func packBlock(uids []uint64) *pb.UidBlock {
	if len(uids) == 0 {
		return nil
	}
	block := &pb.UidBlock{Base: uids[0]}
	last := uids[0]
	for _, uid := range uids[1:] {
		block.Deltas = append(block.Deltas, uid-last)
		last = uid
	}
	return block
}

// Encode takes in a list of uids and a block size. It would pack these uids into blocks of the
// given size, with the last block having fewer uids. Within each block, it stores the first uid as
// base. For each next uid, a delta = uids[i] - uids[i-1] is stored. Protobuf uses Varint encoding,
// as mentioned here: https://developers.google.com/protocol-buffers/docs/encoding . This ensures
// that the deltas being considerably smaller than the original uids are nicely packed in fewer
// bytes. Our benchmarks on artificial data show compressed size to be 13% of the original. This
// mechanism is a LOT simpler to understand and if needed, debug.
func Encode(uids []uint64, blockSize int) pb.UidPack {
	pack := pb.UidPack{BlockSize: uint32(blockSize)}
	for {
		if len(uids) <= blockSize {
			block := packBlock(uids)
			pack.Blocks = append(pack.Blocks, block)
			return pack
		}
		block := packBlock(uids[:blockSize])
		pack.Blocks = append(pack.Blocks, block)
		uids = uids[blockSize:] // Advance.
	}
}

// NumUids returns the number of uids stored in a UidPack.
func NumUids(pack pb.UidPack) int {
	sz := len(pack.Blocks)
	if sz == 0 {
		return 0
	}
	lastBlock := pack.Blocks[sz-1]
	return (sz-1)*int(pack.BlockSize) + len(lastBlock.Deltas) + 1 // We don't store base in deltas.
}

// Decode decodes the UidPack back into the list of uids. This is a stop-gap function, Decode would
// need to do more specific things than just return the list back.
func Decode(pack pb.UidPack) []uint64 {
	uids := make([]uint64, NumUids(pack))
	uids = uids[:0]

	for _, block := range pack.Blocks {
		last := block.Base
		uids = append(uids, last)
		for _, delta := range block.Deltas {
			uid := last + delta
			uids = append(uids, uid)
			last = uid
		}
	}
	return uids
}
