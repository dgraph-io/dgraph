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

package modules

import (
	"fmt"
	"math/big"
	"net/http"

	"github.com/ChainSafe/gossamer/lib/common"
)

// ChainHashRequest Hash
//type ChainHashRequest common.Hash
type ChainHashRequest string

// ChainBlockNumberRequest Int
type ChainBlockNumberRequest interface{}

// ChainBlockResponse struct
type ChainBlockResponse struct {
	Block ChainBlock `json:"block"`
}

// ChainBlock struct to hold json instance of a block
type ChainBlock struct {
	Header ChainBlockHeaderResponse `json:"header"`
	Body   []string                 `json:"extrinsics"`
}

// ChainBlockHeaderResponse struct
type ChainBlockHeaderResponse struct {
	ParentHash     string   `json:"parentHash"`
	Number         *big.Int `json:"number"`
	StateRoot      string   `json:"stateRoot"`
	ExtrinsicsRoot string   `json:"extrinsicsRoot"`
	Digest         [][]byte `json:"digest"`
}

// ChainHashResponse struct
type ChainHashResponse struct {
	ChainHash common.Hash `json:"chainHash"`
}

// ChainModule is an RPC module providing access to storage API points.
type ChainModule struct {
	blockAPI BlockAPI
}

// NewChainModule creates a new State module.
func NewChainModule(api BlockAPI) *ChainModule {
	return &ChainModule{
		blockAPI: api,
	}
}

// GetBlock Get header and body of a relay chain block. If no block hash is provided,
//  the latest block body will be returned.
func (cm *ChainModule) GetBlock(r *http.Request, req *ChainHashRequest, res *ChainBlockResponse) error {
	hash, err := cm.hashLookup(req)
	if err != nil {
		return err
	}

	block, err := cm.blockAPI.GetBlockByHash(hash)
	if err != nil {
		return err
	}

	res.Block.Header.ParentHash = block.Header.ParentHash.String()
	res.Block.Header.Number = block.Header.Number
	res.Block.Header.StateRoot = block.Header.StateRoot.String()
	res.Block.Header.ExtrinsicsRoot = block.Header.ExtrinsicsRoot.String()
	res.Block.Header.Digest = block.Header.Digest // TODO: figure out how to get Digest to be a json object (Issue #744)
	if *block.Body != nil {
		ext, err := block.Body.AsExtrinsics()
		if err != nil {
			return err
		}
		for _, e := range ext {
			res.Block.Body = append(res.Block.Body, string(e))
		}
	}
	return nil
}

// GetBlockHash isn't implemented properly yet.
// TODO finish this
func (cm *ChainModule) GetBlockHash(r *http.Request, req *ChainBlockNumberRequest, res *ChainHashResponse) error {
	// TODO get values from req
	return fmt.Errorf("not implemented yet")
}

// GetFinalizedHead isn't implemented properly yet.
func (cm *ChainModule) GetFinalizedHead(r *http.Request, req *EmptyRequest, res *ChainHashResponse) {
}

//GetHeader Get header of a relay chain block. If no block hash is provided, the latest block header will be returned.
func (cm *ChainModule) GetHeader(r *http.Request, req *ChainHashRequest, res *ChainBlockHeaderResponse) error {
	hash, err := cm.hashLookup(req)
	if err != nil {
		return err
	}

	header, err := cm.blockAPI.GetHeader(hash)
	if err != nil {
		return err
	}

	res.ParentHash = header.ParentHash.String()
	res.Number = header.Number
	res.StateRoot = header.StateRoot.String()
	res.ExtrinsicsRoot = header.ExtrinsicsRoot.String()
	res.Digest = header.Digest // TODO: figure out how to get Digest to be a json object (Issue #744)

	return nil
}

// SubscribeFinalizedHeads isn't implemented properly yet.
func (cm *ChainModule) SubscribeFinalizedHeads(r *http.Request, req *EmptyRequest, res *ChainBlockHeaderResponse) {
}

// SubscribeNewHead isn't implemented properly yet.
func (cm *ChainModule) SubscribeNewHead(r *http.Request, req *EmptyRequest, res *ChainBlockHeaderResponse) {
}

func (cm *ChainModule) hashLookup(req *ChainHashRequest) (common.Hash, error) {
	if len(*req) == 0 {
		hash := cm.blockAPI.HighestBlockHash()
		return hash, nil
	}
	return common.HexToHash(string(*req))
}
