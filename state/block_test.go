package state

import (
	"io/ioutil"
	"math/big"
	"reflect"
	"testing"

	"github.com/ChainSafe/gossamer/core/types"
)

func TestGetBlockByNumber(t *testing.T) {
	dataDir, err := ioutil.TempDir("", "TestGetBlockByNumber")
	if err != nil {
		t.Error("Failed to create temp folder for TestGetBlockByNumber test", "err", err)
		return
	}
	// Create & start a new State service
	stateService := NewService(dataDir)
	err = stateService.Start()
	if err != nil {
		t.Fatal(err)
	}

	// Create a header & blockData
	blockHeader := &types.BlockHeader{
		Number: big.NewInt(1),
	}
	hash := blockHeader.Hash()

	// BlockBody with fake extrinsics
	blockBody := &types.BlockBody{}
	*blockBody = append(*blockBody, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}...)

	blockData := types.BlockData{
		Hash:   hash,
		Header: blockHeader,
		Body:   blockBody,
	}

	// Set the block's header & blockData in the blockState
	// SetHeader also sets mapping [blockNumber : hash] in DB
	err = stateService.Block.SetHeader(*blockHeader)
	if err != nil {
		t.Fatal(err)
	}

	err = stateService.Block.SetBlockData(hash, blockData)
	if err != nil {
		t.Fatal(err)
	}

	// Get block & check if it's the same as the expectedBlock
	expectedBlock := types.Block{
		Header: blockHeader,
		Body:   blockBody,
	}
	retBlock, err := stateService.Block.GetBlockByNumber(blockHeader.Number)
	if err != nil {
		t.Fatal(err)
	}
	retBlock.Header.Hash()

	if !reflect.DeepEqual(expectedBlock, retBlock) {
		t.Fatalf("Fail: got %+v\nexpected %+v", retBlock, expectedBlock)
	}

}
