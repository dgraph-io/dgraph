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

package sync

import (
	"math/big"

	"github.com/ChainSafe/gossamer/dot/network"
	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/common/optional"
)

var maxResponseSize int64 = 128 // maximum number of block datas to reply with in a BlockResponse message.

// CreateBlockResponse creates a block response message from a block request message
func (s *Service) CreateBlockResponse(blockRequest *network.BlockRequestMessage) (*network.BlockResponseMessage, error) {
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

	startHeader, _ := s.blockState.GetHeader(startHash)
	endHeader, _ := s.blockState.GetHeader(endHash)
	s.logger.Debug("BlockRequestMessage", "start", startHeader.Number, "end", endHeader.Number, "startHash", startHash, "endHash", endHash)

	// get sub-chain of block hashes
	subchain, err := s.blockState.SubChain(startHash, endHash)
	if err != nil {
		return nil, err
	}

	if len(subchain) > int(maxResponseSize) {
		subchain = subchain[:maxResponseSize]
	}

	s.logger.Trace("subchain", "start", subchain[0], "end", subchain[len(subchain)-1])

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
		if (blockRequest.RequestedData & network.RequestedDataHeader) == 1 {
			retData, err := s.blockState.GetHeader(hash)
			if err == nil && retData != nil {
				blockData.Header = retData.AsOptional()
			}
		}

		// body
		if (blockRequest.RequestedData&network.RequestedDataBody)>>1 == 1 {
			retData, err := s.blockState.GetBlockBody(hash)
			if err == nil && retData != nil {
				blockData.Body = retData.AsOptional()
			}
		}

		// receipt
		if (blockRequest.RequestedData&network.RequestedDataReceipt)>>2 == 1 {
			retData, err := s.blockState.GetReceipt(hash)
			if err == nil && retData != nil {
				blockData.Receipt = optional.NewBytes(true, retData)
			}
		}

		// message queue
		if (blockRequest.RequestedData&network.RequestedDataMessageQueue)>>3 == 1 {
			retData, err := s.blockState.GetMessageQueue(hash)
			if err == nil && retData != nil {
				blockData.MessageQueue = optional.NewBytes(true, retData)
			}
		}

		// justification
		if (blockRequest.RequestedData&network.RequestedDataJustification)>>4 == 1 {
			retData, err := s.blockState.GetJustification(hash)
			if err == nil && retData != nil {
				blockData.Justification = optional.NewBytes(true, retData)
			}
		}

		responseData = append(responseData, blockData)
	}

	s.logger.Debug("sending BlockResponseMessage", "start", startHeader.Number, "end", endHeader.Number)
	return &network.BlockResponseMessage{
		ID:        blockRequest.ID,
		BlockData: responseData,
	}, nil
}
