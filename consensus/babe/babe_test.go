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

package babe

import (
	"io"
	"math"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/common"
	tx "github.com/ChainSafe/gossamer/common/transaction"
	"github.com/ChainSafe/gossamer/core/blocktree"
	"github.com/ChainSafe/gossamer/core/types"
	db "github.com/ChainSafe/gossamer/polkadb"
	"github.com/ChainSafe/gossamer/runtime"
	"github.com/ChainSafe/gossamer/trie"
)

const POLKADOT_RUNTIME_FP string = "../../substrate_test_runtime.compact.wasm"
const POLKADOT_RUNTIME_URL string = "https://github.com/noot/substrate/blob/add-blob/core/test-runtime/wasm/wasm32-unknown-unknown/release/wbuild/substrate-test-runtime/substrate_test_runtime.compact.wasm?raw=true"

var zeroHash, _ = common.HexToHash("0x00")

// getRuntimeBlob checks if the polkadot runtime wasm file exists and if not, it fetches it from github
func getRuntimeBlob() (n int64, err error) {
	if Exists(POLKADOT_RUNTIME_FP) {
		return 0, nil
	}

	out, err := os.Create(POLKADOT_RUNTIME_FP)
	if err != nil {
		return 0, err
	}
	defer out.Close()

	resp, err := http.Get(POLKADOT_RUNTIME_URL)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	n, err = io.Copy(out, resp.Body)
	return n, err
}

// Exists reports whether the named file or directory exists.
func Exists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

func newRuntime(t *testing.T) *runtime.Runtime {
	_, err := getRuntimeBlob()
	if err != nil {
		t.Fatalf("Fail: could not get polkadot runtime")
	}

	fp, err := filepath.Abs(POLKADOT_RUNTIME_FP)
	if err != nil {
		t.Fatal("could not create filepath")
	}

	ss := NewTestRuntimeStorage()

	r, err := runtime.NewRuntimeFromFile(fp, ss, nil)
	if err != nil {
		t.Fatal(err)
	} else if r == nil {
		t.Fatal("did not create new VM")
	}

	return r
}

func TestCalculateThreshold(t *testing.T) {
	// C = 1
	var C1 uint64 = 1
	var C2 uint64 = 1
	var authorityIndex uint64 = 0
	authorityWeights := []uint64{1, 1, 1}

	expected := new(big.Int).Lsh(big.NewInt(1), 128)

	threshold, err := calculateThreshold(C1, C2, authorityIndex, authorityWeights)
	if err != nil {
		t.Fatal(err)
	}

	if threshold.Cmp(expected) != 0 {
		t.Fatalf("Fail: got %d expected %d", threshold, expected)
	}

	// C = 1/2
	C2 = 2

	theta := float64(1) / float64(3)
	c := float64(C1) / float64(C2)
	pp := 1 - c
	pp_exp := math.Pow(pp, theta)
	p := 1 - pp_exp
	p_rat := new(big.Rat).SetFloat64(p)
	q := new(big.Int).Lsh(big.NewInt(1), 128)
	expected = q.Mul(q, p_rat.Num()).Div(q, p_rat.Denom())

	threshold, err = calculateThreshold(C1, C2, authorityIndex, authorityWeights)
	if err != nil {
		t.Fatal(err)
	}

	if threshold.Cmp(expected) != 0 {
		t.Fatalf("Fail: got %d expected %d", threshold, expected)
	}
}

func TestCalculateThreshold_AuthorityWeights(t *testing.T) {
	var C1 uint64 = 5
	var C2 uint64 = 17
	var authorityIndex uint64 = 3
	authorityWeights := []uint64{3, 1, 4, 6, 10}

	theta := float64(6) / float64(24)
	c := float64(C1) / float64(C2)
	pp := 1 - c
	pp_exp := math.Pow(pp, theta)
	p := 1 - pp_exp
	p_rat := new(big.Rat).SetFloat64(p)
	q := new(big.Int).Lsh(big.NewInt(1), 128)
	expected := q.Mul(q, p_rat.Num()).Div(q, p_rat.Denom())

	threshold, err := calculateThreshold(C1, C2, authorityIndex, authorityWeights)
	if err != nil {
		t.Fatal(err)
	}

	if threshold.Cmp(expected) != 0 {
		t.Fatalf("Fail: got %d expected %d", threshold, expected)
	}
}

func TestRunLottery(t *testing.T) {
	rt := newRuntime(t)

	cfg := &SessionConfig{
		Runtime: rt,
	}

	babesession, err := NewSession(cfg)
	if err != nil {
		t.Fatal(err)
	}

	babesession.authorityIndex = 0
	babesession.authorityData = []AuthorityData{
		{nil, 1}, {nil, 1}, {nil, 1},
	}
	conf := &BabeConfiguration{
		SlotDuration:       1000,
		EpochLength:        6,
		C1:                 3,
		C2:                 10,
		GenesisAuthorities: []AuthorityDataRaw{},
		Randomness:         0,
		SecondarySlots:     false,
	}
	babesession.config = conf

	_, err = babesession.runLottery(0)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCalculateThreshold_Failing(t *testing.T) {
	var C1 uint64 = 5
	var C2 uint64 = 4
	var authorityIndex uint64 = 3
	authorityWeights := []uint64{3, 1, 4, 6, 10}

	_, err := calculateThreshold(C1, C2, authorityIndex, authorityWeights)
	if err == nil {
		t.Fatal("Fail: did not err for c>1")
	}
}

func TestConfigurationFromRuntime(t *testing.T) {
	rt := newRuntime(t)
	cfg := &SessionConfig{
		Runtime: rt,
	}

	babesession, err := NewSession(cfg)
	if err != nil {
		t.Fatal(err)
	}

	err = babesession.configurationFromRuntime()
	if err != nil {
		t.Fatal(err)
	}

	// see: https://github.com/paritytech/substrate/blob/7b1d822446982013fa5b7ad5caff35ca84f8b7d0/core/test-runtime/src/lib.rs#L621
	expected := &BabeConfiguration{
		SlotDuration:       1000,
		EpochLength:        6,
		C1:                 3,
		C2:                 10,
		GenesisAuthorities: []AuthorityDataRaw{},
		Randomness:         0,
		SecondarySlots:     false,
	}

	if babesession.config == expected {
		t.Errorf("Fail: got %v expected %v\n", babesession.config, expected)
	}
}

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

func createFlatBlockTree(t *testing.T, depth int) *blocktree.BlockTree {
	genesisBlock := types.Block{
		Header: &types.BlockHeader{
			ParentHash: zeroHash,
			Number:     big.NewInt(0),
		},
		Body: &types.BlockBody{},
	}
	genesisBlock.SetBlockArrivalTime(uint64(1000))

	d := &db.BlockDB{
		Db: db.NewMemDatabase(),
	}

	bt := blocktree.NewBlockTreeFromGenesis(genesisBlock, d)
	previousHash := genesisBlock.Header.Hash()
	previousAT := genesisBlock.GetBlockArrivalTime()

	for i := 1; i <= depth; i++ {
		block := types.Block{
			Header: &types.BlockHeader{
				ParentHash: previousHash,
				Number:     big.NewInt(int64(i)),
			},
			Body: &types.BlockBody{},
		}

		hash := block.Header.Hash()
		block.SetBlockArrivalTime(previousAT + uint64(1000))

		bt.AddBlock(block)
		previousHash = hash
		previousAT = block.GetBlockArrivalTime()
	}

	return bt
}

func TestSlotTime(t *testing.T) {
	rt := newRuntime(t)
	bt := createFlatBlockTree(t, 100)
	cfg := &SessionConfig{
		Runtime: rt,
	}

	babesession, err := NewSession(cfg)
	if err != nil {
		t.Fatal(err)
	}

	err = babesession.configurationFromRuntime()
	if err != nil {
		t.Fatal(err)
	}

	res, err := babesession.slotTime(103, bt, 20)
	if err != nil {
		t.Fatal(err)
	}

	var expected uint64 = 104000

	if res != expected {
		t.Errorf("Fail: got %v expected %v\n", res, expected)
	}

}

func TestStart(t *testing.T) {
	rt := newRuntime(t)
	cfg := &SessionConfig{
		Runtime:   rt,
		NewBlocks: make(chan types.Block),
	}

	babesession, err := NewSession(cfg)
	if err != nil {
		t.Fatal(err)
	}

	babesession.authorityIndex = 0
	babesession.authorityData = []AuthorityData{{nil, 1}}
	conf := &BabeConfiguration{
		SlotDuration:       1,
		EpochLength:        6,
		C1:                 1,
		C2:                 10,
		GenesisAuthorities: []AuthorityDataRaw{},
		Randomness:         0,
		SecondarySlots:     false,
	}
	babesession.config = conf

	err = babesession.Start()
	if err != nil {
		t.Fatal(err)
	}
}

func TestBabeAnnounceMessage(t *testing.T) {
	rt := newRuntime(t)

	newBlocks := make(chan types.Block)

	cfg := &SessionConfig{
		Runtime:   rt,
		NewBlocks: newBlocks,
	}

	babesession, err := NewSession(cfg)
	if err != nil {
		t.Fatal(err)
	}

	babesession.authorityIndex = 0
	babesession.authorityData = []AuthorityData{
		{nil, 1}, {nil, 1}, {nil, 1},
	}

	err = babesession.Start()
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Duration(babesession.config.SlotDuration) * time.Duration(babesession.config.EpochLength) * time.Millisecond)

	for i := 0; i < int(babesession.config.EpochLength); i++ {
		block := <-newBlocks
		blockNumber := big.NewInt(int64(0))
		if !reflect.DeepEqual(block.Header.Number, blockNumber) {
			t.Fatalf("Didn't receive the correct block: %+v\nExpected block: %+v", block.Header.Number, blockNumber)
		}
	}
}

func TestBuildBlock(t *testing.T) {
	rt := newRuntime(t)
	cfg := &SessionConfig{
		Runtime: rt,
	}

	babesession, err := NewSession(cfg)
	if err != nil {
		t.Fatal(err)
	}
	err = babesession.configurationFromRuntime()
	if err != nil {
		t.Fatal(err)
	}

	// https://github.com/paritytech/substrate/blob/5420de3face1349a97eb954ae71c5b0b940c31de/core/transaction-pool/src/tests.rs#L95
	txb := []byte{1, 212, 53, 147, 199, 21, 253, 211, 28, 97, 20, 26, 189, 4, 169, 159, 214, 130, 44, 133, 88, 133, 76, 205, 227, 154, 86, 132, 231, 165, 109, 162, 125, 142, 175, 4, 21, 22, 135, 115, 99, 38, 201, 254, 161, 126, 37, 252, 82, 135, 97, 54, 147, 201, 18, 144, 156, 178, 38, 170, 71, 148, 242, 106, 72, 69, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 216, 5, 113, 87, 87, 40, 221, 120, 247, 252, 137, 201, 74, 231, 222, 101, 85, 108, 102, 39, 31, 190, 210, 14, 215, 124, 19, 160, 180, 203, 54, 110, 167, 163, 149, 45, 12, 108, 80, 221, 65, 238, 57, 237, 199, 16, 10, 33, 185, 8, 244, 184, 243, 139, 5, 87, 252, 245, 24, 225, 37, 154, 163, 142}

	validity, err := babesession.validateTransaction(txb)
	if err != nil {
		t.Fatal(err)
	}

	// https://github.com/paritytech/substrate/blob/ea2644a235f4b189c8029b9c9eac9d4df64ee91e/core/test-runtime/src/system.rs#L190
	expected := &tx.Validity{
		Priority: 69,
		Requires: [][]byte{{}},
		// https://github.com/paritytech/substrate/blob/ea2644a235f4b189c8029b9c9eac9d4df64ee91e/core/test-runtime/src/system.rs#L173
		Provides:  [][]byte{{146, 157, 61, 99, 63, 98, 30, 242, 128, 49, 150, 90, 140, 165, 187, 249}},
		Longevity: 64,
		Propagate: true,
	}

	if !reflect.DeepEqual(expected, validity) {
		t.Error(
			"received unexpected validity",
			"\nexpected:", expected,
			"\nreceived:", validity,
		)
	}

	vtx := tx.NewValidTransaction(types.Extrinsic(txb), expected)
	babesession.PushToTxQueue(vtx)

	zeroHash, err := common.HexToHash("0x00")
	if err != nil {
		t.Fatal(err)
	}

	parentHeader := &types.BlockHeader{
		ParentHash: zeroHash,
		Number:     big.NewInt(0),
	}

	slot := Slot{
		start:    uint64(time.Now().Unix()),
		duration: uint64(10),
		number:   1,
	}

	block, err := babesession.buildBlock(parentHeader, slot)
	if err != nil {
		t.Fatal(err)
	}

	// the hash of an empty trie
	emptyRootHash, err := common.HexToHash("0x03170a2e7597b7b7e3d84c05391d139a62b157e78786d8c082f29dcf4c111314")
	if err != nil {
		t.Fatal(err)
	}

	extrinsicsHash, err := common.HexToHash("0x4f4c5f3ebd6112b5c4ec8c354712978db0b0465b30ef06bb46f528802b04407c")
	if err != nil {
		t.Fatal(err)
	}

	expectedBlockHeader := &types.BlockHeader{
		ParentHash:     zeroHash,
		Number:         big.NewInt(1),
		StateRoot:      emptyRootHash,
		ExtrinsicsRoot: extrinsicsHash,
		Digest:         []byte{},
	}

	if !reflect.DeepEqual(block.Header, expectedBlockHeader) {
		t.Fatalf("Fail: got %v expected %v", block.Header, expectedBlockHeader)
	}
}

func NewTestRuntimeStorage() *TestRuntimeStorage {
	return &TestRuntimeStorage{
		trie: trie.NewEmptyTrie(nil),
	}
}

type TestRuntimeStorage struct {
	trie *trie.Trie
}

func (trs TestRuntimeStorage) SetStorage(key []byte, value []byte) error {
	return trs.trie.Put(key, value)
}
func (trs TestRuntimeStorage) GetStorage(key []byte) ([]byte, error) {
	return trs.trie.Get(key)
}
func (trs TestRuntimeStorage) StorageRoot() (common.Hash, error) {
	return trs.trie.Hash()
}
func (trs TestRuntimeStorage) SetStorageChild(keyToChild []byte, child *trie.Trie) error {
	return trs.trie.PutChild(keyToChild, child)
}
func (trs TestRuntimeStorage) SetStorageIntoChild(keyToChild, key, value []byte) error {
	return trs.trie.PutIntoChild(keyToChild, key, value)
}
func (trs TestRuntimeStorage) GetStorageFromChild(keyToChild, key []byte) ([]byte, error) {
	return trs.trie.GetFromChild(keyToChild, key)
}
func (trs TestRuntimeStorage) ClearStorage(key []byte) error {
	return trs.trie.Delete(key)
}
func (trs TestRuntimeStorage) Entries() map[string][]byte {
	return trs.trie.Entries()
}
