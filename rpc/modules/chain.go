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

	"github.com/ChainSafe/gossamer/common"
	"github.com/ChainSafe/gossamer/internal/api"
)

type ChainHashRequest common.Hash

type ChainBlockNumberRequest *big.Int

// TODO: Waiting on Block type defined here https://github.com/ChainSafe/gossamer/pull/233
type ChainBlockResponse struct{}

type ChainBlockHeaderResponse struct{}

type ChainHashResponse struct {
	ChainHash common.Hash `json:"chainHash"`
}

// ChainModule is an RPC module providing access to storage API points.
type ChainModule struct {
	api *api.Api
}

// NewChainModule creates a new State module.
func NewChainModule(api *api.Api) *ChainModule {
	return &ChainModule{
		api: api,
	}
}

func (cm *ChainModule) GetBlock(r *http.Request, req *ChainHashRequest, res *ChainBlockResponse) {
	_ = cm.api
}

func (cm *ChainModule) GetBlockHash(r *http.Request, req *ChainBlockNumberRequest, res *ChainHashResponse) {
}

func (cm *ChainModule) GetFinalizedHead(r *http.Request, req *EmptyRequest, res *ChainHashResponse) {
}

//DB isn't implemented properly yet. Doesn't return block headers
func (cm *ChainModule) GetHeader(r *http.Request, req *ChainHashRequest, res *ChainBlockHeaderResponse) {
}

func (cm *ChainModule) SubscribeFinalizedHeads(r *http.Request, req *EmptyRequest, res *ChainBlockHeaderResponse) {
}

func (cm *ChainModule) SubscribeNewHead(r *http.Request, req *EmptyRequest, res *ChainBlockHeaderResponse) {
}
