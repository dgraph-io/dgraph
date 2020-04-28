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
	"math/big"

	"github.com/ChainSafe/gossamer/dot/network"
	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/common/optional"
	"github.com/ChainSafe/gossamer/lib/transaction"

	log "github.com/ChainSafe/log15"
)

// BlockRequestMessage 1

// ProcessBlockRequestMessage processes a block request message, returning a block response message
func (s *Service) ProcessBlockRequestMessage(msg *network.BlockRequestMessage) error {
	log.Debug("[core] received BlockRequestMessage")

	res, err := s.createBlockResponse(msg)
	if err != nil {
		return err
	}

	return s.safeMsgSend(res)
}

// createBlockResponse create a block response message from a block request message
func (s *Service) createBlockResponse(blockRequest *network.BlockRequestMessage) (*network.BlockResponseMessage, error) {
	var startHash common.Hash
	var endHash common.Hash

	switch startBlock := blockRequest.StartingBlock.Value().(type) {
	case uint64:
		if startBlock == 0 {
			startBlock = 1
		}
		block, err := s.blockState.GetBlockByNumber(big.NewInt(0).SetUint64(startBlock))
		if err != nil {
			return nil, err
		}

		startHash = block.Header.Hash()
	case common.Hash:
		startHash = startBlock
	}

	if blockRequest.EndBlockHash.Exists() {
		endHash = blockRequest.EndBlockHash.Value()
	} else {
		endHash = s.blockState.BestBlockHash()
	}

	log.Debug("[core] BlockRequestMessage", "startHash", startHash, "endHash", endHash)

	// get sub-chain of block hashes
	subchain, err := s.blockState.SubChain(startHash, endHash)
	if err != nil {
		return nil, err
	}

	if len(subchain) > int(maxResponseSize) {
		subchain = subchain[:maxResponseSize]
	}

	log.Trace("[core] subchain", "start", subchain[0], "end", subchain[len(subchain)-1])

	responseData := []*types.BlockData{}

	for _, hash := range subchain {

		blockData := new(types.BlockData)
		blockData.Hash = hash

		// set defaults
		blockData.Header = optional.NewHeader(false, nil)
		blockData.Body = optional.NewBody(false, nil)
		blockData.Receipt = optional.NewBytes(false, nil)
		blockData.MessageQueue = optional.NewBytes(false, nil)
		blockData.Justification = optional.NewBytes(false, nil)

		// header
		if (blockRequest.RequestedData & 1) == 1 {
			retData, err := s.blockState.GetHeader(hash)
			if err == nil && retData != nil {
				blockData.Header = retData.AsOptional()
			}
		}
		// body
		if (blockRequest.RequestedData&2)>>1 == 1 {
			retData, err := s.blockState.GetBlockBody(hash)
			if err == nil && retData != nil {
				blockData.Body = retData.AsOptional()
			}
		}
		// receipt
		if (blockRequest.RequestedData&4)>>2 == 1 {
			retData, err := s.blockState.GetReceipt(hash)
			if err == nil && retData != nil {
				blockData.Receipt = optional.NewBytes(true, retData)
			}
		}
		// message queue
		if (blockRequest.RequestedData&8)>>3 == 1 {
			retData, err := s.blockState.GetMessageQueue(hash)
			if err == nil && retData != nil {
				blockData.MessageQueue = optional.NewBytes(true, retData)
			}
		}
		// justification
		if (blockRequest.RequestedData&16)>>4 == 1 {
			retData, err := s.blockState.GetJustification(hash)
			if err == nil && retData != nil {
				blockData.Justification = optional.NewBytes(true, retData)
			}
		}

		responseData = append(responseData, blockData)
	}

	return &network.BlockResponseMessage{
		ID:        blockRequest.ID,
		BlockData: responseData,
	}, nil
}

// BlockResponseMessage 2

// ProcessBlockResponseMessage attempts to validate and add the block to the
// chain by calling `core_execute_block`. Valid blocks are stored in the block
// database to become part of the canonical chain.
func (s *Service) ProcessBlockResponseMessage(msg *network.BlockResponseMessage) error {
	log.Debug("[core] received BlockResponseMessage")

	// send block response message to syncer
	s.respOut <- msg

	// check if we need to update the runtime
	err := s.checkForRuntimeChanges()
	if err != nil {
		return err
	}

	return nil
}

// BlockAnnounceMessage 3

// ProcessBlockAnnounceMessage creates a block request message from the block
// announce messages (block announce messages include the header but the full
// block is required to execute `core_execute_block`).
func (s *Service) ProcessBlockAnnounceMessage(msg *network.BlockAnnounceMessage) error {
	log.Debug("[core] received BlockAnnounceMessage")

	// create header from message
	header, err := types.NewHeader(
		msg.ParentHash,
		msg.Number,
		msg.StateRoot,
		msg.ExtrinsicsRoot,
		msg.Digest,
	)
	if err != nil {
		return err
	}

	// check if block header is stored in block state and save block header
	_, err = s.blockState.GetHeader(header.Hash())
	if err != nil && err.Error() == "Key not found" {
		err = s.blockState.SetHeader(header)
		if err != nil {
			return err
		}
		log.Debug(
			"[core] saved block header to block state",
			"number", header.Number,
			"hash", header.Hash(),
		)
	} else {
		return err
	}

	// check if block body is stored in block state (if we should send to syncer)
	_, err = s.blockState.GetBlockBody(header.Hash())
	if err != nil && err.Error() == "Key not found" {
		log.Debug(
			"[core] sending block number to syncer",
			"number", msg.Number,
		)
		s.blockNumOut <- msg.Number
	} else if err != nil {
		return err
	}

	return nil
}

// TransactionMessage 4

// ProcessTransactionMessage validates each transaction in the message and
// adds valid transactions to the transaction queue of the BABE session
func (s *Service) ProcessTransactionMessage(msg *network.TransactionMessage) error {
	log.Debug("[core] received TransactionMessage")

	// get transactions from message extrinsics
	txs := msg.Extrinsics

	for _, tx := range txs {
		tx := tx // pin

		// validate each transaction
		val, err := s.rt.ValidateTransaction(tx)
		if err != nil {
			log.Error("[core] failed to validate transaction", "err", err)
			return err // exit
		}

		// create new valid transaction
		vtx := transaction.NewValidTransaction(tx, val)

		if s.isBabeAuthority {
			// push to the transaction queue of BABE session
			hash, err := s.transactionQueue.Push(vtx)
			if err != nil {
				log.Trace("[core] Failed to push transaction to queue", "error", err)
			} else {
				log.Trace("[core] Added transaction to queue", "hash", hash)
			}
		}
	}

	return nil
}
