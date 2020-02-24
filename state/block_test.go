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

	"github.com/ChainSafe/gossamer/common"
	"github.com/ChainSafe/gossamer/core/types"
	"github.com/ChainSafe/gossamer/trie"
	"github.com/stretchr/testify/require"
)

func TestSetAndGetHeader(t *testing.T) {
	dataDir, err := ioutil.TempDir("", "./test_data")
	require.Nil(t, err)

	blockDb, err := NewBlockDB(dataDir)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		err = blockDb.Db.Close()
		require.Nil(t, err, "BlockDB close err: ", err)
	}()

	bs := &BlockState{
		db: blockDb,
	}

	header := &types.Header{
		Number:    big.NewInt(0),
		StateRoot: trie.EmptyHash,
	}

	err = bs.SetHeader(header)
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
	dataDir, err := ioutil.TempDir("", "TestGetBlockByNumber")
	require.Nil(t, err)

	// Create & start a new State service
	stateService := NewService(dataDir)
	err = stateService.Initialize(&types.Header{
		Number:    big.NewInt(0),
		StateRoot: trie.EmptyHash,
	}, trie.NewEmptyTrie(nil))
	if err != nil {
		t.Fatal(err)
	}

	err = stateService.Start()
	require.Nil(t, err)

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
	err = stateService.Block.SetHeader(blockHeader)
	require.Nil(t, err)

	err = stateService.Block.SetBlockData(blockData)
	require.Nil(t, err)

	// Get block & check if it's the same as the expectedBlock
	expectedBlock := &types.Block{
		Header: blockHeader,
		Body:   blockBody,
	}

	retBlock, err := stateService.Block.GetBlockByNumber(blockHeader.Number)
	require.Nil(t, err)

	retBlock.Header.Hash()

	require.Equal(t, expectedBlock, retBlock, "Could not validate returned retBlock as expected")

}

func TestAddBlock(t *testing.T) {
	dataDir, err := ioutil.TempDir("", "TestAddBlock")
	require.Nil(t, err)

	blockDb, err := NewBlockDB(dataDir)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		err = blockDb.Db.Close()
		require.Nil(t, err, "BlockDB close err: ", err)
	}()

	blockState := &BlockState{
		db: blockDb,
	}

	// Create header
	header0 := &types.Header{
		Number: big.NewInt(0),
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
	err = blockState.AddBlock(block0)
	require.Nil(t, err)

	// Create header & blockData for block 1
	header1 := &types.Header{
		Number: big.NewInt(1),
	}
	blockHash1 := header1.Hash()

	// Create Block with fake extrinsics
	blockBody1 := types.Body{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}

	block1 := &types.Block{
		Header: header1,
		Body:   &blockBody1,
	}

	// Add the block1 to the DB
	err = blockState.AddBlock(block1)
	require.Nil(t, err)

	// Get the blocks & check if it's the same as the added blocks
	retBlock, err := blockState.GetBlockByHash(blockHash0)
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

	require.Equal(t, block0, retBlock, "Could not validate returned block0 as expected")

	retBlock, err = blockState.GetBlockByHash(blockHash1)
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
	require.Equal(t, block1.Header, blockState.latestHeader, "Latest Header Block Check Fail")

}
