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
	"reflect"
	"regexp"

	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
)

// ChainHashRequest Hash as a string
type ChainHashRequest string

// ChainBlockNumberRequest interface can accept string, float64 or []
type ChainBlockNumberRequest interface{}

// ChainIntRequest represents an integer
type ChainIntRequest uint64

// ChainBlockHeaderResponse struct
type ChainBlockHeaderResponse struct {
	ParentHash     string                 `json:"parentHash"`
	Number         string                 `json:"number"`
	StateRoot      string                 `json:"stateRoot"`
	ExtrinsicsRoot string                 `json:"extrinsicsRoot"`
	Digest         ChainBlockHeaderDigest `json:"digest"`
}

// ChainBlockHeaderDigest struct to hold digest logs
type ChainBlockHeaderDigest struct {
	Logs []string `json:"logs"`
}

// ChainBlock struct to hold json instance of a block
type ChainBlock struct {
	Header ChainBlockHeaderResponse `json:"header"`
	Body   []string                 `json:"extrinsics"`
}

// ChainBlockResponse struct
type ChainBlockResponse struct {
	Block ChainBlock `json:"block"`
}

// ChainHashResponse interface to handle response
type ChainHashResponse interface{}

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

	res.Block.Header = HeaderToJSON(*block.Header)

	if *block.Body != nil {
		ext, err := block.Body.AsExtrinsics()
		if err != nil {
			return err
		}
		for _, e := range ext {
			res.Block.Body = append(res.Block.Body, fmt.Sprintf("0x%x", e))
		}
	}
	return nil
}

// GetBlockHash Get hash of the 'n-th' block in the canon chain. If no parameters are provided,
//  the latest block hash gets returned.
func (cm *ChainModule) GetBlockHash(r *http.Request, req *ChainBlockNumberRequest, res *ChainHashResponse) error {
	// if request is empty, return highest hash
	if *req == nil || reflect.ValueOf(*req).Len() == 0 {
		*res = cm.blockAPI.BestBlockHash().String()
		return nil
	}

	val, err := cm.unwindRequest(*req)
	// if result only returns 1 value, just use that (instead of array)
	if len(val) == 1 {
		*res = val[0]
	} else {
		*res = val
	}

	return err
}

// GetHead alias for GetBlockHash
func (cm *ChainModule) GetHead(r *http.Request, req *ChainBlockNumberRequest, res *ChainHashResponse) error {
	return cm.GetBlockHash(r, req, res)
}

// GetFinalizedHead returns the most recently finalized block hash
func (cm *ChainModule) GetFinalizedHead(r *http.Request, req *EmptyRequest, res *ChainHashResponse) error {
	h, err := cm.blockAPI.GetFinalizedHash(0, 0)
	if err != nil {
		return err
	}

	*res = common.BytesToHex(h[:])
	return nil
}

// GetFinalizedHeadByRound returns the hash of the block finalized at the given round and setID
func (cm *ChainModule) GetFinalizedHeadByRound(r *http.Request, req *[]ChainIntRequest, res *ChainHashResponse) error {
	round := (uint64)((*req)[0])
	setID := (uint64)((*req)[1])
	h, err := cm.blockAPI.GetFinalizedHash(round, setID)
	if err != nil {
		return err
	}

	*res = common.BytesToHex(h[:])
	return nil
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

	*res = HeaderToJSON(*header)
	return nil
}

// SubscribeFinalizedHeads handled by websocket handler, but this func should remain
//  here so it's added to rpc_methods list
func (cm *ChainModule) SubscribeFinalizedHeads(r *http.Request, req *EmptyRequest, res *ChainBlockHeaderResponse) error {
	return nil
}

// SubscribeNewHead handled by websocket handler, but this func should remain
//  here so it's added to rpc_methods list
func (cm *ChainModule) SubscribeNewHead(r *http.Request, req *EmptyRequest, res *ChainBlockHeaderResponse) error {
	return nil
}

// SubscribeNewHeads isn't implemented properly yet.
func (cm *ChainModule) SubscribeNewHeads(r *http.Request, req *EmptyRequest, res *ChainBlockHeaderResponse) error {
	return nil
}

func (cm *ChainModule) hashLookup(req *ChainHashRequest) (common.Hash, error) {
	if len(*req) == 0 {
		hash := cm.blockAPI.BestBlockHash()
		return hash, nil
	}
	return common.HexToHash(string(*req))
}

// unwindRequest takes request interface slice and makes call for each element
func (cm *ChainModule) unwindRequest(req interface{}) ([]string, error) {
	res := make([]string, 0)
	switch x := (req).(type) {
	case []interface{}:
		for _, v := range x {
			u, err := cm.unwindRequest(v)
			if err != nil {
				return nil, err
			}
			res = append(res, u[:]...)
		}
	case interface{}:
		h, err := cm.lookupHashByInterface(x)
		if err != nil {
			return nil, err
		}
		res = append(res, h)
	}
	return res, nil
}

// lookupHashByInterface parses given interface to determine block number, then
//  finds hash for that block number
func (cm *ChainModule) lookupHashByInterface(i interface{}) (string, error) {
	num := new(big.Int)
	switch x := i.(type) {
	case float64:
		f := big.NewFloat(x)
		f.Int(num)
	case string:
		// remove leading 0x (if there is one)
		re, err := regexp.Compile(`0x`)
		if err != nil {
			return "", err
		}
		x = re.ReplaceAllString(x, "")

		// cast string to big.Int
		_, ok := num.SetString(x, 10)
		if !ok {
			return "", fmt.Errorf("error setting number from string")
		}

	default:
		return "", fmt.Errorf("unknown request number type: %T", x)
	}

	h, err := cm.blockAPI.GetBlockHash(num)
	if err != nil {
		return "", err
	}
	return h.String(), nil
}

// HeaderToJSON converts types.Header to ChainBlockHeaderResponse
func HeaderToJSON(header types.Header) ChainBlockHeaderResponse {
	res := ChainBlockHeaderResponse{
		ParentHash:     header.ParentHash.String(),
		StateRoot:      header.StateRoot.String(),
		ExtrinsicsRoot: header.ExtrinsicsRoot.String(),
		Digest:         ChainBlockHeaderDigest{},
	}
	if header.Number.Int64() == 0 {
		res.Number = "0x00" // needs two 0 chars for hex decoding to work
	} else {
		res.Number = common.BytesToHex(header.Number.Bytes())
	}
	for _, item := range header.Digest {
		res.Digest.Logs = append(res.Digest.Logs, common.BytesToHex(item))
	}
	return res
}
