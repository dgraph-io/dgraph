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
	mrand "math/rand"
	"time"

	"github.com/ChainSafe/gossamer/lib/common/optional"

	log "github.com/ChainSafe/log15"
	"github.com/libp2p/go-libp2p-core/peer"
	"golang.org/x/exp/rand"
)

// sendBlockRequestMessage sends a block request message if peer best block number is greater than host best block number
func (s *Service) sendBlockRequestMessage(peer peer.ID, statusMessage *StatusMessage) {
	// check if peer status confirmed
	if s.status.confirmed(peer) {

		// get latest block header from block state
		latestHeader, err := s.cfg.BlockState.BestBlockHeader()
		if err != nil {
			log.Error("[network] Failed to get best block header from block state", "error", err)
			return
		}

		bestBlockNum := big.NewInt(int64(statusMessage.BestBlockNumber))

		// check if peer block number is greater than host block number
		if latestHeader.Number.Cmp(bestBlockNum) == -1 {

			//generate random ID
			s1 := rand.NewSource(uint64(time.Now().UnixNano()))
			seed := rand.New(s1).Uint64()
			randomID := mrand.New(mrand.NewSource(int64(seed))).Uint64()

			// keep track of the IDs in network state
			s.requestedBlockIDs[randomID] = true

			currentHash := latestHeader.Hash()

			blockRequest := &BlockRequestMessage{
				ID:            randomID, // random
				RequestedData: 3,        // block body
				StartingBlock: append([]byte{0}, currentHash[:]...),
				EndBlockHash:  optional.NewHash(true, latestHeader.Hash()),
				Direction:     1,
				Max:           optional.NewUint32(false, 0),
			}

			// send block request message
			err := s.host.send(peer, blockRequest)
			if err != nil {
				log.Error("[network] Failed to send block request message to peer", "error", err)
			} else {
				log.Trace("[network] Sent block message to peer", "peer", peer, "type", blockRequest.GetType())
			}
		}
	}
}
