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
	"fmt"

	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/babe"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"

	log "github.com/ChainSafe/log15"
)

// initializeBabeSession creates a new BABE session
func (s *Service) initializeBabeSession() (*babe.Session, error) {
	log.Debug(
		"[core] initializing BABE session...",
		"epoch", s.syncer.verifier.EpochNumber(),
	)

	// TODO: AuthorityData comes from NextEpochDescriptor within the ConsensusDigest
	// of the block Digest, which is included in the first block of each epoch
	authData := s.bs.AuthorityData()
	if len(authData) == 0 {
		return nil, fmt.Errorf("authority data not set")
	}

	newBlocks := make(chan types.Block)
	s.blkRec = newBlocks

	epochDone := make(chan struct{})
	s.epochDone = epochDone

	babeKill := make(chan struct{})
	s.babeKill = babeKill

	keys := s.keys.Sr25519Keypairs()

	// get best slot to determine next start slot
	bestHash := s.blockState.BestBlockHash()
	bestSlot, err := s.blockState.GetSlotForBlock(bestHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get slot for latest block: %s", err)
	}

	// BABE session configuration
	bsConfig := &babe.SessionConfig{
		Keypair:          keys[0].(*sr25519.Keypair),
		Runtime:          s.rt,
		NewBlocks:        newBlocks, // becomes block send channel in BABE session
		BlockState:       s.blockState,
		StorageState:     s.storageState,
		TransactionQueue: s.transactionQueue,
		AuthData:         authData,
		Done:             epochDone,
		Kill:             babeKill,
		StartSlot:        bestSlot + 1,
		SyncLock:         s.syncLock,
	}

	// create new BABE session
	bs, err := babe.NewSession(bsConfig)
	if err != nil {
		log.Error("[core] failed to initialize BABE session", "error", err)
		return nil, err
	}

	log.Debug(
		"[core] BABE session initialized",
		"epoch", s.syncer.verifier.EpochNumber(),
	)

	return bs, nil
}
