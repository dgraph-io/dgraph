// Copyright 2020 ChainSafe Systems (ON) Corp.
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

package utils

import (
	"fmt"
	"math/big"
	"strconv"
	"testing"

	"github.com/ChainSafe/gossamer/dot/rpc/modules"
	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/stretchr/testify/require"
)

// GetHeader calls the endpoint chain_getHeader
func GetHeader(t *testing.T, node *Node, hash common.Hash) *types.Header { //nolint
	respBody, err := PostRPC(ChainGetHeader, NewEndpoint(node.RPCPort), "[\""+hash.String()+"\"]")
	require.NoError(t, err)

	header := new(modules.ChainBlockHeaderResponse)
	err = DecodeRPC(t, respBody, header)
	require.Nil(t, err)

	return HeaderResponseToHeader(t, header)
}

// GetChainHead calls the endpoint chain_getHeader to get the latest chain head
func GetChainHead(t *testing.T, node *Node) *types.Header {
	respBody, err := PostRPC(ChainGetHeader, NewEndpoint(node.RPCPort), "[]")
	require.NoError(t, err)

	header := new(modules.ChainBlockHeaderResponse)
	err = DecodeRPC(t, respBody, header)
	require.Nil(t, err)

	return HeaderResponseToHeader(t, header)
}

// GetBlockHash calls the endpoint chain_getBlockHash to get the latest chain head
func GetBlockHash(t *testing.T, node *Node, num string) (common.Hash, error) {
	respBody, err := PostRPC(ChainGetBlockHash, NewEndpoint(node.RPCPort), "["+num+"]")
	if err != nil {
		return common.Hash{}, err
	}

	var hash string
	err = DecodeRPC(t, respBody, &hash)
	if err != nil {
		return common.Hash{}, err
	}
	return common.MustHexToHash(hash), nil
}

// GetFinalizedHead calls the endpoint chain_getFinalizedHead to get the latest finalized head
func GetFinalizedHead(t *testing.T, node *Node) common.Hash {
	respBody, err := PostRPC(ChainGetFinalizedHead, NewEndpoint(node.RPCPort), "[]")
	require.NoError(t, err)

	var hash string
	err = DecodeRPC(t, respBody, &hash)
	require.NoError(t, err)
	return common.MustHexToHash(hash)
}

// GetFinalizedHeadByRound calls the endpoint chain_getFinalizedHeadByRound to get the finalized head at a given round
// TODO: add setID, hard-coded at 1 for now
func GetFinalizedHeadByRound(t *testing.T, node *Node, round uint64) (common.Hash, error) {
	p := strconv.Itoa(int(round))
	respBody, err := PostRPC(ChainGetFinalizedHeadByRound, NewEndpoint(node.RPCPort), "["+p+",1]")
	require.NoError(t, err)

	var hash string
	err = DecodeRPC(t, respBody, &hash)
	if err != nil {
		return common.Hash{}, err
	}

	return common.MustHexToHash(hash), nil
}

// GetBlock calls the endpoint chain_getBlock
func GetBlock(t *testing.T, node *Node, hash common.Hash) *types.Block {
	respBody, err := PostRPC(ChainGetBlock, NewEndpoint(node.RPCPort), "[\""+hash.String()+"\"]")
	require.NoError(t, err)

	block := new(modules.ChainBlockResponse)
	err = DecodeRPC(t, respBody, block)
	if err != nil {
		return nil
	}

	header := block.Block.Header

	parentHash, err := common.HexToHash(header.ParentHash)
	require.NoError(t, err)

	nb, err := common.HexToBytes(header.Number)
	require.NoError(t, err)
	number := big.NewInt(0).SetBytes(nb)

	stateRoot, err := common.HexToHash(header.StateRoot)
	require.NoError(t, err)

	extrinsicsRoot, err := common.HexToHash(header.ExtrinsicsRoot)
	require.NoError(t, err)

	digest := [][]byte{}

	for _, l := range header.Digest.Logs {
		var d []byte
		d, err = common.HexToBytes(l)
		require.NoError(t, err)
		digest = append(digest, d)
	}

	h, err := types.NewHeader(parentHash, number, stateRoot, extrinsicsRoot, digest)
	require.NoError(t, err)

	b, err := types.NewBodyFromExtrinsicStrings(block.Block.Body)
	require.NoError(t, err, fmt.Sprintf("%v", block.Block.Body))

	return &types.Block{
		Header: h,
		Body:   b,
	}
}
