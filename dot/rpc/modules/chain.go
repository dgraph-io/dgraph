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
	"math/big"
	"net/http"

	"github.com/ChainSafe/gossamer/lib/common"
)

// ChainHashRequest Hash
type ChainHashRequest common.Hash

// ChainBlockNumberRequest Int
type ChainBlockNumberRequest *big.Int

// ChainBlockResponse struct
// TODO: Waiting on Block type defined here https://github.com/ChainSafe/gossamer/pull/233
type ChainBlockResponse struct{}

// ChainBlockHeaderResponse struct
type ChainBlockHeaderResponse struct{}

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

// GetBlock assigns the ChainModule api to nothing
func (cm *ChainModule) GetBlock(r *http.Request, req *ChainHashRequest, res *ChainBlockResponse) {
	_ = cm.blockAPI
}

// GetBlockHash isn't implemented properly yet.
func (cm *ChainModule) GetBlockHash(r *http.Request, req *ChainBlockNumberRequest, res *ChainHashResponse) {
}

// GetFinalizedHead isn't implemented properly yet.
func (cm *ChainModule) GetFinalizedHead(r *http.Request, req *EmptyRequest, res *ChainHashResponse) {
}

//GetHeader DB isn't implemented properly yet. Doesn't return block headers
func (cm *ChainModule) GetHeader(r *http.Request, req *ChainHashRequest, res *ChainBlockHeaderResponse) {
}

// SubscribeFinalizedHeads isn't implemented properly yet.
func (cm *ChainModule) SubscribeFinalizedHeads(r *http.Request, req *EmptyRequest, res *ChainBlockHeaderResponse) {
}

// SubscribeNewHead isn't implemented properly yet.
func (cm *ChainModule) SubscribeNewHead(r *http.Request, req *EmptyRequest, res *ChainBlockHeaderResponse) {
}
