package babe

import (
	"io/ioutil"
	"math/big"
	"testing"

	"github.com/ChainSafe/gossamer/dot/core/types"
	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/lib/trie"
)

func TestMedian_OddLength(t *testing.T) {
	us := []uint64{3, 2, 1, 4, 5}
	res, err := median(us)
	if err != nil {
		t.Fatal(err)
	}

	var expected uint64 = 3

	if res != expected {
		t.Errorf("Fail: got %v expected %v\n", res, expected)
	}

}

func TestMedian_EvenLength(t *testing.T) {
	us := []uint64{1, 4, 2, 4, 5, 6}
	res, err := median(us)
	if err != nil {
		t.Fatal(err)
	}

	var expected uint64 = 4

	if res != expected {
		t.Errorf("Fail: got %v expected %v\n", res, expected)
	}

}

func TestSlotOffset_Failing(t *testing.T) {
	var st uint64 = 1000001
	var se uint64 = 1000000

	_, err := slotOffset(st, se)
	if err == nil {
		t.Fatal("Fail: did not err for c>1")
	}

}

func TestSlotOffset(t *testing.T) {
	var st uint64 = 1000000
	var se uint64 = 1000001

	res, err := slotOffset(st, se)
	if err != nil {
		t.Fatal(err)
	}

	var expected uint64 = 1

	if res != expected {
		t.Errorf("Fail: got %v expected %v\n", res, expected)
	}
}

func addBlocksToState(t *testing.T, depth int, blockState BlockState) {
	previousHash := blockState.BestBlockHash()
	previousAT := uint64(0)

	for i := 1; i <= depth; i++ {
		block := &types.Block{
			Header: &types.Header{
				ParentHash: previousHash,
				Number:     big.NewInt(int64(i)),
			},
			Body: &types.Body{},
		}

		arrivalTime := previousAT + uint64(1000)
		previousHash = block.Header.Hash()
		previousAT = arrivalTime

		err := blockState.AddBlockWithArrivalTime(block, arrivalTime)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestSlotTime(t *testing.T) {
	dataDir, err := ioutil.TempDir("", "./test_data")
	if err != nil {
		t.Fatal(err)
	}

	genesisHeader := &types.Header{
		Number:    big.NewInt(0),
		StateRoot: trie.EmptyHash,
	}

	dbSrv := state.NewService(dataDir)
	err = dbSrv.Initialize(genesisHeader, trie.NewEmptyTrie(nil))
	if err != nil {
		t.Fatal(err)
	}

	err = dbSrv.Start()
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		err = dbSrv.Stop()
		if err != nil {
			t.Fatal(err)
		}
	}()

	cfg := &SessionConfig{
		BlockState:   dbSrv.Block,
		StorageState: dbSrv.Storage,
	}

	babesession := createTestSession(t, cfg)

	addBlocksToState(t, 100, dbSrv.Block)

	res, err := babesession.slotTime(103, 20)
	if err != nil {
		t.Fatal(err)
	}

	expected := uint64(104000)

	if res != expected {
		t.Errorf("Fail: got %v expected %v\n", res, expected)
	}
}
