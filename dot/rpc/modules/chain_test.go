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

package modules

import (
	"math/big"
	"testing"

	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/genesis"
	"github.com/ChainSafe/gossamer/lib/trie"
	"github.com/ChainSafe/gossamer/lib/utils"

	database "github.com/ChainSafe/chaindb"
	log "github.com/ChainSafe/log15"
	"github.com/stretchr/testify/require"
)

func TestChainGetHeader_Genesis(t *testing.T) {
	chain := newTestChainService(t)
	svc := NewChainModule(chain.Block)
	expected := &ChainBlockHeaderResponse{
		ParentHash:     "0x0000000000000000000000000000000000000000000000000000000000000000",
		Number:         "0x00",
		StateRoot:      "0x03170a2e7597b7b7e3d84c05391d139a62b157e78786d8c082f29dcf4c111314",
		ExtrinsicsRoot: "0x03170a2e7597b7b7e3d84c05391d139a62b157e78786d8c082f29dcf4c111314",
		Digest:         ChainBlockHeaderDigest{},
	}
	res := &ChainBlockHeaderResponse{}
	req := ChainHashRequest("0xc375f478c6887dbcc2d1a4dbcc25f330b3df419325ece49cddfe5a0555663b7e")
	err := svc.GetHeader(nil, &req, res)
	require.Nil(t, err)

	require.Equal(t, expected, res)
}

func TestChainGetHeader_Latest(t *testing.T) {
	chain := newTestChainService(t)
	svc := NewChainModule(chain.Block)
	expected := &ChainBlockHeaderResponse{
		ParentHash:     "0xdbfdd87392d9ee52f499610582737daceecf83dc3ad7946fcadeb01c86e1ef75",
		Number:         "0x01",
		StateRoot:      "0x0000000000000000000000000000000000000000000000000000000000000000",
		ExtrinsicsRoot: "0x0000000000000000000000000000000000000000000000000000000000000000",
		Digest:         ChainBlockHeaderDigest{},
	}
	res := &ChainBlockHeaderResponse{}
	req := ChainHashRequest("") // empty request should return latest hash
	err := svc.GetHeader(nil, &req, res)
	require.Nil(t, err)

	require.Equal(t, expected, res)
}

func TestChainGetHeader_NotFound(t *testing.T) {
	chain := newTestChainService(t)
	svc := NewChainModule(chain.Block)

	res := &ChainBlockHeaderResponse{}
	req := ChainHashRequest("0xea374832a2c3997280d2772c10e6e5b0b493ccd3d09c0ab14050320e34076c2c")
	err := svc.GetHeader(nil, &req, res)
	require.EqualError(t, err, database.ErrKeyNotFound.Error())
}

func TestChainGetHeader_Error(t *testing.T) {
	chain := newTestChainService(t)
	svc := NewChainModule(chain.Block)

	res := &ChainBlockHeaderResponse{}
	req := ChainHashRequest("zz")
	err := svc.GetHeader(nil, &req, res)
	require.EqualError(t, err, "could not byteify non 0x prefixed string")
}

func TestChainGetBlock_Genesis(t *testing.T) {
	chain := newTestChainService(t)
	svc := NewChainModule(chain.Block)
	header := &ChainBlockHeaderResponse{
		ParentHash:     "0x0000000000000000000000000000000000000000000000000000000000000000",
		Number:         "0x00",
		StateRoot:      "0x03170a2e7597b7b7e3d84c05391d139a62b157e78786d8c082f29dcf4c111314",
		ExtrinsicsRoot: "0x03170a2e7597b7b7e3d84c05391d139a62b157e78786d8c082f29dcf4c111314",
		Digest:         ChainBlockHeaderDigest{},
	}
	expected := &ChainBlockResponse{
		Block: ChainBlock{
			Header: *header,
			Body:   nil,
		},
	}

	res := &ChainBlockResponse{}
	req := ChainHashRequest("0xc375f478c6887dbcc2d1a4dbcc25f330b3df419325ece49cddfe5a0555663b7e")
	err := svc.GetBlock(nil, &req, res)
	require.Nil(t, err)

	require.Equal(t, expected, res)
}

func TestChainGetBlock_Latest(t *testing.T) {
	chain := newTestChainService(t)
	svc := NewChainModule(chain.Block)
	header := &ChainBlockHeaderResponse{
		ParentHash:     "0xdbfdd87392d9ee52f499610582737daceecf83dc3ad7946fcadeb01c86e1ef75",
		Number:         "0x01",
		StateRoot:      "0x0000000000000000000000000000000000000000000000000000000000000000",
		ExtrinsicsRoot: "0x0000000000000000000000000000000000000000000000000000000000000000",
		Digest:         ChainBlockHeaderDigest{},
	}
	expected := &ChainBlockResponse{
		Block: ChainBlock{
			Header: *header,
			Body:   nil,
		},
	}

	res := &ChainBlockResponse{}
	req := ChainHashRequest("") // empty request should return latest block
	err := svc.GetBlock(nil, &req, res)
	require.Nil(t, err)

	require.Equal(t, expected, res)
}

func TestChainGetBlock_NoFound(t *testing.T) {
	chain := newTestChainService(t)
	svc := NewChainModule(chain.Block)

	res := &ChainBlockResponse{}
	req := ChainHashRequest("0xea374832a2c3997280d2772c10e6e5b0b493ccd3d09c0ab14050320e34076c2c")
	err := svc.GetBlock(nil, &req, res)
	require.EqualError(t, err, database.ErrKeyNotFound.Error())
}

func TestChainGetBlock_Error(t *testing.T) {
	chain := newTestChainService(t)
	svc := NewChainModule(chain.Block)

	res := &ChainBlockResponse{}
	req := ChainHashRequest("zz")
	err := svc.GetBlock(nil, &req, res)
	require.EqualError(t, err, "could not byteify non 0x prefixed string")
}

func TestChainGetBlockHash_Latest(t *testing.T) {
	chain := newTestChainService(t)
	svc := NewChainModule(chain.Block)

	resString := string("")
	res := ChainHashResponse(resString)
	req := ChainBlockNumberRequest(nil)
	err := svc.GetBlockHash(nil, &req, &res)

	require.Nil(t, err)

	require.Equal(t, "0x80d653de440352760f89366c302c02a92ab059f396e2bfbf7f860e6e256cd698", res)
}

func TestChainGetBlockHash_ByNumber(t *testing.T) {
	chain := newTestChainService(t)
	svc := NewChainModule(chain.Block)

	resString := string("")
	res := ChainHashResponse(resString)
	req := ChainBlockNumberRequest("1")
	err := svc.GetBlockHash(nil, &req, &res)

	require.Nil(t, err)

	require.Equal(t, "0x80d653de440352760f89366c302c02a92ab059f396e2bfbf7f860e6e256cd698", res)
}

func TestChainGetBlockHash_ByHex(t *testing.T) {
	chain := newTestChainService(t)
	svc := NewChainModule(chain.Block)

	resString := string("")
	res := ChainHashResponse(resString)
	req := ChainBlockNumberRequest("0x01")
	err := svc.GetBlockHash(nil, &req, &res)

	require.Nil(t, err)

	require.Equal(t, "0x80d653de440352760f89366c302c02a92ab059f396e2bfbf7f860e6e256cd698", res)
}

func TestChainGetBlockHash_Array(t *testing.T) {
	chain := newTestChainService(t)
	svc := NewChainModule(chain.Block)

	resString := string("")
	res := ChainHashResponse(resString)
	nums := make([]interface{}, 2)
	nums[0] = float64(0)     // as number
	nums[1] = string("0x01") // as hex string
	req := ChainBlockNumberRequest(nums)
	err := svc.GetBlockHash(nil, &req, &res)

	require.Nil(t, err)

	require.Equal(t, []string{"0xdbfdd87392d9ee52f499610582737daceecf83dc3ad7946fcadeb01c86e1ef75", "0x80d653de440352760f89366c302c02a92ab059f396e2bfbf7f860e6e256cd698"}, res)
}

func TestChainGetFinalizedHead(t *testing.T) {
	chain := newTestChainService(t)
	svc := NewChainModule(chain.Block)

	var res ChainHashResponse
	err := svc.GetFinalizedHead(nil, &EmptyRequest{}, &res)
	require.NoError(t, err)
	expected := genesisHeader.Hash()
	require.Equal(t, common.BytesToHex(expected[:]), res)
}

func TestChainGetFinalizedHeadByRound(t *testing.T) {
	chain := newTestChainService(t)
	svc := NewChainModule(chain.Block)

	var res ChainHashResponse
	req := ChainIntRequest(0)
	err := svc.GetFinalizedHeadByRound(nil, &req, &res)
	require.NoError(t, err)
	expected := genesisHeader.Hash()
	require.Equal(t, common.BytesToHex(expected[:]), res)

	testhash := common.Hash{1, 2, 3, 4}
	err = chain.Block.SetFinalizedHash(testhash, 77)
	require.NoError(t, err)

	req = ChainIntRequest(77)
	err = svc.GetFinalizedHeadByRound(nil, &req, &res)
	require.NoError(t, err)
	require.Equal(t, common.BytesToHex(testhash[:]), res)
}

var genesisHeader, _ = types.NewHeader(common.NewHash([]byte{0}), big.NewInt(0), trie.EmptyHash, trie.EmptyHash, [][]byte{})

var firstEpochInfo = &types.EpochInfo{
	Duration:   200,
	FirstBlock: 0,
}

func newTestChainService(t *testing.T) *state.Service {
	testDir := utils.NewTestDir(t)
	defer utils.RemoveTestDir(t)
	stateSrvc := state.NewService(testDir, log.LvlInfo)

	tr := trie.NewEmptyTrie()

	stateSrvc.UseMemDB()
	genesisData := new(genesis.Data)

	err := stateSrvc.Initialize(genesisData, genesisHeader, tr, firstEpochInfo)
	if err != nil {
		t.Fatal(err)
	}

	err = stateSrvc.Start()
	if err != nil {
		t.Fatal(err)
	}

	err = loadTestBlocks(genesisHeader.Hash(), stateSrvc.Block)
	if err != nil {
		t.Fatal(err)
	}
	return stateSrvc
}

func loadTestBlocks(gh common.Hash, bs *state.BlockState) error {
	// Create header
	header0 := &types.Header{
		Number:     big.NewInt(0),
		Digest:     [][]byte{},
		ParentHash: gh,
	}
	// Create blockHash
	blockHash0 := header0.Hash()
	// BlockBody with fake extrinsics
	blockBody0 := types.Body{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}

	block0 := &types.Block{
		Header: header0,
		Body:   &blockBody0,
	}

	err := bs.AddBlock(block0)
	if err != nil {
		return err
	}

	// Create header & blockData for block 1
	header1 := &types.Header{
		Number:     big.NewInt(1),
		Digest:     [][]byte{},
		ParentHash: blockHash0,
	}

	// Create Block with fake extrinsics
	blockBody1 := types.Body{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}

	block1 := &types.Block{
		Header: header1,
		Body:   &blockBody1,
	}

	// Add the block1 to the DB
	err = bs.AddBlock(block1)
	if err != nil {
		return err
	}

	return nil
}
