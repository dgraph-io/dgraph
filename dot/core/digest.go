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

package core

import (
	"errors"
	"fmt"

	"github.com/ChainSafe/gossamer/dot/core/types"
	"github.com/ChainSafe/gossamer/lib/babe"

	log "github.com/ChainSafe/log15"
)

// handleBlockDigest checks if the provided header is the block header for
// the first block of the current epoch, finds and decodes the consensus digest
// item from the block digest, and then sets the epoch data for the next epoch
func (s *Service) handleBlockDigest(header *types.Header) (err error) {

	// check if block header digest items exist
	if header.Digest == nil || len(header.Digest) == 0 {
		return fmt.Errorf("header digest is not set")
	}

	// declare digest item
	var item types.DigestItem

	// decode each digest item and check its type
	for _, digest := range header.Digest {
		item, err = types.DecodeDigestItem(digest)
		if err != nil {
			return err
		}

		// check if digest item is consensus digest type
		if item.Type() == types.ConsensusDigestType {

			// cast decoded consensus digest item to conensus digest type
			consensusDigest, ok := item.(*types.ConsensusDigest)
			if !ok {
				return errors.New("failed to cast DigestItem to ConsensusDigest")
			}

			err = s.handleConsensusDigest(header, consensusDigest)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// handleConsensusDigest checks if the first block has been set for the current
// epoch, if first block has not been set or the block header has a lower block
// number, sets epoch data for the next epoch using data in the ConsensusDigest
func (s *Service) handleConsensusDigest(header *types.Header, digest *types.ConsensusDigest) error {

	// check if block header is from current epoch
	currentEpoch, err := s.blockFromCurrentEpoch(header.Hash())
	if err != nil {
		return fmt.Errorf("failed to check if block header is from current epoch: %s", err)
	} else if !currentEpoch {
		return fmt.Errorf("block header is not from current epoch")
	}

	// check if first block has been set for current epoch
	if s.firstBlock != nil {

		// check if block header has lower block number than current first block
		if header.Number.Cmp(s.firstBlock) >= 0 {

			// error if block header does not have lower block number
			return fmt.Errorf("first block already set for current epoch")
		}

		// either BABE produced two first blocks or we received invalid first block from connected peer
		log.Warn("[core] received first block header with lower block number than current first block")
	}

	err = s.setNextEpochDescriptor(digest.Data)
	if err != nil {
		return fmt.Errorf("failed to set next epoch descriptor: %s", err)
	}

	// set first block in current epoch
	s.firstBlock = header.Number

	return nil
}

// setNextEpochDescriptor sets the epoch data for the next epoch from the data
// included in the ConsensusDigest of the first block of each epoch
func (s *Service) setNextEpochDescriptor(data []byte) error {

	// initialize epoch data interface for next epoch
	nextEpochData := new(babe.NextEpochDescriptor)

	// decode consensus digest data for next epoch
	err := nextEpochData.Decode(data)
	if err != nil {
		return err
	}

	// set epoch data for next epoch
	return s.bs.SetEpochData(nextEpochData)
}
