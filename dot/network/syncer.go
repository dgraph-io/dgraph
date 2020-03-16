// Copyright 2019 ChainSafe Systems (ON) Corp.
// This file is part of gossamer.
//
// The gossamer library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The gossamer library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the gossamer library. If not, see <http://www.gnu.org/licenses/>.

package network

import (
	"math/big"

	"github.com/ChainSafe/gossamer/lib/common/optional"

	log "github.com/ChainSafe/log15"
	"github.com/libp2p/go-libp2p-core/peer"
)

// syncer submodule
type syncer struct {
	host              *host
	blockState        BlockState
	requestedBlockIDs map[uint64]bool // track requested block id messages
}

// newSyncer creates a new syncer instance from the host
func newSyncer(host *host, blockState BlockState) *syncer {
	return &syncer{
		host:              host,
		blockState:        blockState,
		requestedBlockIDs: make(map[uint64]bool),
	}
}

// addRequestedBlockID adds a requested block id to non-persistent state
func (s *syncer) addRequestedBlockID(blockID uint64) {
	log.Trace("[network] Adding block to network syncer...", "block", blockID)
	s.requestedBlockIDs[blockID] = true
}

// hasRequestedBlockID returns true if the block id has been requested
func (s *syncer) hasRequestedBlockID(blockID uint64) bool {
	requested := s.requestedBlockIDs[blockID]
	log.Trace("[network] Checking block in network syncer...", "block", blockID, "requested", requested)
	return requested
}

// removeRequestedBlockID removes a requested block id from non-persistent state
func (s *syncer) removeRequestedBlockID(blockID uint64) {
	log.Trace("[network] Removing block from network syncer...", "block", blockID)
	delete(s.requestedBlockIDs, blockID)
}

// handleStatusMesssage sends a block request message if peer best block
// number is greater than host best block number
func (s *syncer) handleStatusMesssage(peer peer.ID, statusMessage *StatusMessage) {

	// get latest block header from block state
	latestHeader, err := s.blockState.BestBlockHeader()
	if err != nil {
		log.Error("[network] Failed to get best block header from block state", "error", err)
		return
	}

	bestBlockNum := big.NewInt(int64(statusMessage.BestBlockNumber))

	// check if peer block number is greater than host block number
	if latestHeader.Number.Cmp(bestBlockNum) == -1 {

		// store requested block ids in syncer submodule (non-persistent state)
		s.addRequestedBlockID(latestHeader.Number.Uint64())

		currentHash := latestHeader.Hash()

		blockRequestMessage := &BlockRequestMessage{
			ID:            latestHeader.Number.Uint64(), // block id
			RequestedData: 3,                            // block body
			StartingBlock: append([]byte{0}, currentHash[:]...),
			EndBlockHash:  optional.NewHash(true, latestHeader.Hash()),
			Direction:     1,
			Max:           optional.NewUint32(false, 0),
		}

		// send block request message
		err := s.host.send(peer, blockRequestMessage)
		if err != nil {
			log.Error("[network] Failed to send block request message to peer", "error", err)
		}
	}
}
