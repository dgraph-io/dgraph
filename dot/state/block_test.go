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

package state

import (
	"io/ioutil"
	"math/big"
	"reflect"
	"testing"

	"github.com/ChainSafe/gossamer/dot/core/types"
	"github.com/ChainSafe/gossamer/lib/blocktree"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/database"
	"github.com/ChainSafe/gossamer/lib/trie"

	"github.com/stretchr/testify/require"
)

func newTestBlockState(t *testing.T, header *types.Header) *BlockState {
	datadir, err := ioutil.TempDir("", "./test_data")
	require.Nil(t, err)

	db, err := database.NewBadgerDB(datadir)

	blockDb := NewBlockDB(db)
	require.Nil(t, err)

	if header == nil {
		return &BlockState{
			db: blockDb,
		}
	}

	return &BlockState{
		db: blockDb,
		bt: blocktree.NewBlockTreeFromGenesis(header, db),
	}
}

func TestSetAndGetHeader(t *testing.T) {
	bs := newTestBlockState(t, nil)
	defer bs.db.db.Close()

	header := &types.Header{
		Number:    big.NewInt(0),
		StateRoot: trie.EmptyHash,
	}

	err := bs.SetHeader(header)
	if err != nil {
		t.Fatal(err)
	}

	res, err := bs.GetHeader(header.Hash())
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(res, header) {
		t.Fatalf("Fail: got %v expected %v", res, header)
	}
}

func TestGetBlockByNumber(t *testing.T) {
	bs := newTestBlockState(t, nil)
	defer bs.db.db.Close()

	// Create a header & blockData
	blockHeader := &types.Header{
		Number: big.NewInt(1),
	}
	hash := blockHeader.Hash()

	// BlockBody with fake extrinsics
	blockBody := &types.Body{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}

	blockData := &types.BlockData{
		Hash:   hash,
		Header: blockHeader.AsOptional(),
		Body:   blockBody.AsOptional(),
	}

	// Set the block's header & blockData in the blockState
	// SetHeader also sets mapping [blockNumber : hash] in DB
	err := bs.SetHeader(blockHeader)
	require.Nil(t, err)

	err = bs.SetBlockData(blockData)
	require.Nil(t, err)

	// Get block & check if it's the same as the expectedBlock
	expectedBlock := &types.Block{
		Header: blockHeader,
		Body:   blockBody,
	}

	retBlock, err := bs.GetBlockByNumber(blockHeader.Number)
	require.Nil(t, err)

	retBlock.Header.Hash()

	require.Equal(t, expectedBlock, retBlock, "Could not validate returned retBlock as expected")

}

func TestAddBlock(t *testing.T) {
	genesisHeader := &types.Header{
		Number: big.NewInt(0),
	}

	bs := newTestBlockState(t, genesisHeader)
	defer bs.db.db.Close()

	// Create header
	header0 := &types.Header{
		Number:     big.NewInt(0),
		ParentHash: genesisHeader.Hash(),
	}
	// Create blockHash
	blockHash0 := header0.Hash()
	// BlockBody with fake extrinsics
	blockBody0 := types.Body{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}

	block0 := &types.Block{
		Header: header0,
		Body:   &blockBody0,
	}

	// Add the block0 to the DB
	err := bs.AddBlock(block0)
	require.Nil(t, err)

	// Create header & blockData for block 1
	header1 := &types.Header{
		ParentHash: blockHash0,
		Number:     big.NewInt(1),
	}
	blockHash1 := header1.Hash()

	// Create Block with fake extrinsics
	blockBody1 := types.Body{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}

	block1 := &types.Block{
		Header: header1,
		Body:   &blockBody1,
	}

	// Add the block1 to the DB
	err = bs.AddBlock(block1)
	require.Nil(t, err)

	// Get the blocks & check if it's the same as the added blocks
	retBlock, err := bs.GetBlockByHash(blockHash0)
	require.Nil(t, err)

	require.Equal(t, block0, retBlock, "Could not validate returned block0 as expected")

	retBlock, err = bs.GetBlockByHash(blockHash1)
	require.Nil(t, err)

	// this will panic if not successful, so catch and fail it so
	func() {
		hash := retBlock.Header.Hash()
		defer func() {
			if r := recover(); r != nil {
				t.Fatal("got panic when processing retBlock.Header.Hash() ", r)
			}
		}()
		require.NotEqual(t, hash, common.Hash{})
	}()

	require.Equal(t, block1, retBlock, "Could not validate returned block1 as expected")

	// Check if latestBlock is set correctly
	require.Equal(t, block1.Header.Hash(), bs.BestBlockHash(), "Latest Header Block Check Fail")
}
