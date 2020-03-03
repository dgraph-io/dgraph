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
	"github.com/ChainSafe/gossamer/lib/common/optional"
	log "github.com/ChainSafe/log15"
	"github.com/libp2p/go-libp2p-core/peer"

	"math/big"
	mrand "math/rand"
	"time"

	"golang.org/x/exp/rand"
)

// sendBlockRequestMessage will send a BlockRequestMessage if peer block is greater than host block number
func (s *Service) sendBlockRequestMessage(peer peer.ID, msg Message) {
	// check if peer status confirmed
	if s.status.confirmed(peer) {

		// get latest block header from block state
		latestHeader, err := s.cfg.BlockState.BestBlockHeader()
		if err != nil {
			log.Error("(*Service).sendBlockRequestMessage, failed to BestBlockHeader")
			return
		}

		statusMessage, ok := msg.(*StatusMessage)
		if !ok {
			log.Error("(*Service).sendBlockRequestMessage, failed to cast blockAnnounceMessage from msg.(*BlockAnnounceMessage)")
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
				log.Error("(*Service).sendBlockRequestMessage, failed to send blockRequest message to peer", "err", err)
			} else {
				log.Trace("(*Service).sendBlockRequestMessage, sent blockRequest message to peer")
			}
		}
	}
}
