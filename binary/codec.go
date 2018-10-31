package main

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

func Encode(uids []uint64, blockSize int) pb.UidPack {
	pack := pb.UidPack{BlockSize: uint32(blockSize)}
	for {
		if len(uids) <= blockSize {
			block := packBlock(uids)
			pack.Blocks = append(pack.Blocks, block)
			return pack
		} else {
			block := packBlock(uids[:blockSize])
			pack.Blocks = append(pack.Blocks, block)
			uids = uids[blockSize:] // Advance.
		}
	}
}

func NumUids(pack pb.UidPack) int {
	sz := len(pack.Blocks)
	lastBlock := pack.Blocks[sz-1]
	return (sz-1)*int(pack.BlockSize) + len(lastBlock.Deltas) + 1 // We don't store base in deltas.
}

// Decode would need to do more specific things than just return the array back.
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
