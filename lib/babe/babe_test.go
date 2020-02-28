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
	"bytes"
	"io/ioutil"
	"math"
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/dot/core/types"
	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/lib/blocktree"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
	"github.com/ChainSafe/gossamer/lib/runtime"
	"github.com/ChainSafe/gossamer/lib/transaction"
	"github.com/ChainSafe/gossamer/lib/trie"
	"github.com/ChainSafe/gossamer/tests"
)

func createTestSession(t *testing.T, cfg *SessionConfig) *Session {
	rt := runtime.NewTestRuntime(t, tests.POLKADOT_RUNTIME)

	if cfg == nil {
		cfg = &SessionConfig{
			Runtime: rt,
		}
	}

	if cfg.Runtime == nil {
		cfg.Runtime = rt
	}

	var err error
	if cfg.Keypair == nil {
		cfg.Keypair, err = sr25519.GenerateKeypair()
		if err != nil {
			t.Fatal(err)
		}
	}

	if cfg.AuthData == nil {
		auth := &AuthorityData{
			ID:     cfg.Keypair.Public().(*sr25519.PublicKey),
			Weight: 1,
		}
		cfg.AuthData = []*AuthorityData{auth}
	}

	if cfg.TransactionQueue == nil {
		cfg.TransactionQueue = state.NewTransactionQueue()
	}

	babesession, err := NewSession(cfg)
	if err != nil {
		t.Fatal(err)
	}

	return babesession
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
	babesession := createTestSession(t, nil)
	babesession.epochThreshold = big.NewInt(0)

	outAndProof, err := babesession.runLottery(0)
	if err != nil {
		t.Fatal(err)
	}

	if outAndProof == nil {
		t.Fatal("proof was nil when over threshold")
	}
}

func TestRunLottery_False(t *testing.T) {
	babesession := createTestSession(t, nil)
	babesession.epochThreshold = big.NewInt(0).SetBytes([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff})

	outAndProof, err := babesession.runLottery(0)
	if err != nil {
		t.Fatal(err)
	}

	if outAndProof != nil {
		t.Fatal("proof was not nil when under threshold")
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

func createFlatBlockTree(t *testing.T, depth int, blockState BlockState) {
	zeroHash, err := common.HexToHash("0x00")
	if err != nil {
		t.Fatal(err)
	}

	genesisBlock := &types.Block{
		Header: &types.Header{
			ParentHash: zeroHash,
			Number:     big.NewInt(0),
		},
		Body: &types.Body{},
	}
	genesisBlock.SetBlockArrivalTime(uint64(1000))

	bt := blocktree.NewBlockTreeFromGenesis(genesisBlock.Header, nil)
	previousHash := genesisBlock.Header.Hash()
	previousAT := genesisBlock.GetBlockArrivalTime()

	blockState.SetBlock(genesisBlock)

	for i := 1; i <= depth; i++ {
		block := &types.Block{
			Header: &types.Header{
				ParentHash: previousHash,
				Number:     big.NewInt(int64(i)),
			},
			Body: &types.Body{},
		}

		hash := block.Header.Hash()
		block.SetBlockArrivalTime(previousAT + uint64(1000))

		bt.AddBlock(block)
		previousHash = hash
		previousAT = block.GetBlockArrivalTime()
		blockState.AddBlock(block)
	}
}

func TestSlotTime(t *testing.T) {
	t.Skip()
	dataDir, err := ioutil.TempDir("", "./test_data")
	if err != nil {
		t.Fatal(err)
	}

	dbSrv := state.NewService(dataDir)
	err = dbSrv.Initialize(&types.Header{
		Number:    big.NewInt(0),
		StateRoot: trie.EmptyHash,
	}, trie.NewEmptyTrie(nil))
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

	createFlatBlockTree(t, 100, dbSrv.Block)

	res, err := babesession.slotTime(103, 20)
	if err != nil {
		t.Fatal(err)
	}

	expected := uint64(104000)

	if res != expected {
		t.Errorf("Fail: got %v expected %v\n", res, expected)
	}
}

func TestBabeAnnounceMessage(t *testing.T) {
	newBlocks := make(chan types.Block)

	dataDir, err := ioutil.TempDir("", "./test_data")
	if err != nil {
		t.Fatal(err)
	}

	dbSrv := state.NewService(dataDir)
	err = dbSrv.Initialize(&types.Header{
		Number:    big.NewInt(0),
		StateRoot: trie.EmptyHash,
	}, trie.NewEmptyTrie(nil))
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

	TransactionQueue := state.NewTransactionQueue()

	cfg := &SessionConfig{
		NewBlocks:        newBlocks,
		BlockState:       dbSrv.Block,
		StorageState:     dbSrv.Storage,
		TransactionQueue: TransactionQueue,
	}

	babesession := createTestSession(t, cfg)
	err = babesession.configurationFromRuntime()
	if err != nil {
		t.Fatal(err)
	}

	babesession.config = &Configuration{
		SlotDuration:       1,
		EpochLength:        6,
		C1:                 1,
		C2:                 10,
		GenesisAuthorities: []*AuthorityDataRaw{},
		Randomness:         0,
		SecondarySlots:     false,
	}

	babesession.authorityIndex = 0
	babesession.authorityData = []*AuthorityData{
		{nil, 1}, {nil, 1}, {nil, 1},
	}

	err = babesession.Start()
	if err != nil {
		t.Fatal(err)
	}

	block := <-newBlocks
	blockNumber := big.NewInt(int64(1))
	if !reflect.DeepEqual(block.Header.Number, blockNumber) {
		t.Fatalf("Didn't receive the correct block: %+v\nExpected block: %+v", block.Header.Number, blockNumber)
	}
}

func TestSeal(t *testing.T) {
	kp, err := sr25519.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}

	cfg := &SessionConfig{
		Keypair: kp,
	}

	babesession := createTestSession(t, cfg)
	err = babesession.configurationFromRuntime()
	if err != nil {
		t.Fatal(err)
	}

	zeroHash, err := common.HexToHash("0x00")
	if err != nil {
		t.Fatal(err)
	}

	header, err := types.NewHeader(zeroHash, big.NewInt(0), zeroHash, zeroHash, [][]byte{})
	if err != nil {
		t.Fatal(err)
	}

	encHeader, err := header.Encode()
	if err != nil {
		t.Fatal(err)
	}

	seal, err := babesession.buildBlockSeal(header)
	if err != nil {
		t.Fatal(err)
	}

	ok, err := kp.Public().Verify(encHeader, seal.Data)
	if err != nil {
		t.Fatal(err)
	}

	if !ok {
		t.Fatal("could not verify seal")
	}
}

func createTestBlock(babesession *Session, exts [][]byte, t *testing.T) (*types.Block, Slot) {
	// create proof that we can authorize this block
	babesession.epochThreshold = big.NewInt(0)
	babesession.authorityIndex = 0
	var slotNumber uint64 = 1

	outAndProof, err := babesession.runLottery(slotNumber)
	if err != nil {
		t.Fatal(err)
	}

	if outAndProof == nil {
		t.Fatal("proof was nil when over threshold")
	}

	babesession.slotToProof[slotNumber] = outAndProof

	for _, ext := range exts {
		vtx := transaction.NewValidTransaction(types.Extrinsic(ext), &transaction.Validity{})
		babesession.transactionQueue.Push(vtx)
	}

	zeroHash, err := common.HexToHash("0x00")
	if err != nil {
		t.Fatal(err)
	}

	parentHeader := &types.Header{
		ParentHash: zeroHash,
		Number:     big.NewInt(0),
	}

	slot := Slot{
		start:    uint64(time.Now().Unix()),
		duration: uint64(10000000),
		number:   slotNumber,
	}

	// build block
	block, err := babesession.buildBlock(parentHeader, slot)
	if err != nil {
		t.Fatal(err)
	}

	return block, slot
}
func TestBuildBlock_ok(t *testing.T) {
	transactionQueue := state.NewTransactionQueue()

	cfg := &SessionConfig{
		TransactionQueue: transactionQueue,
	}

	babesession := createTestSession(t, cfg)
	err := babesession.configurationFromRuntime()
	if err != nil {
		t.Fatal(err)
	}

	// see https://github.com/noot/substrate/blob/add-blob/core/test-runtime/src/system.rs#L468
	txb := []byte{3, 16, 110, 111, 111, 116, 1, 64, 103, 111, 115, 115, 97, 109, 101, 114, 95, 105, 115, 95, 99, 111, 111, 108}
	exts := [][]byte{txb}

	block, slot := createTestBlock(babesession, exts, t)

	// hash of parent header
	parentHash, err := common.HexToHash("0xdcdd89927d8a348e00257e1ecc8617f45edb5118efff3ea2f9961b2ad9b7690a")
	if err != nil {
		t.Fatal(err)
	}

	stateRoot, err := common.HexToHash("0x31ce5e74d7141520abc11b8a68f884cb1d01b5476a6376a659d93a199c4884e0")
	if err != nil {
		t.Fatal(err)
	}

	extrinsicsRoot, err := common.HexToHash("0xd88e048eda17aaefc427c832ea1208508d67a3e96527be0995db742b5cd91a61")
	if err != nil {
		t.Fatal(err)
	}

	// create pre-digest
	preDigest, err := babesession.buildBlockPreDigest(slot)
	if err != nil {
		t.Fatal(err)
	}

	expectedBlockHeader := &types.Header{
		ParentHash:     parentHash,
		Number:         big.NewInt(1),
		StateRoot:      stateRoot,
		ExtrinsicsRoot: extrinsicsRoot,
		Digest:         [][]byte{preDigest.Encode()},
	}

	// remove seal from built block, since we can't predict the signature
	block.Header.Digest = block.Header.Digest[:1]

	if !reflect.DeepEqual(block.Header, expectedBlockHeader) {
		t.Fatalf("Fail: got %v expected %v", block.Header, expectedBlockHeader)
	}

	// confirm block body is correct
	extsRes, err := block.Body.AsExtrinsics()
	if err != nil {
		t.Fatal(err)
	}

	extsBytes := types.ExtrinsicsArrayToBytesArray(extsRes)
	if !reflect.DeepEqual(extsBytes, exts) {
		t.Fatalf("Fail: got %v expected %v", extsBytes, exts)
	}
}

func TestBuildBlock_failing(t *testing.T) {
	transactionQueue := state.NewTransactionQueue()

	cfg := &SessionConfig{
		TransactionQueue: transactionQueue,
	}

	babesession := createTestSession(t, cfg)
	err := babesession.configurationFromRuntime()
	if err != nil {
		t.Fatal(err)
	}

	babesession.authorityData = []*AuthorityData{{nil, 1}}

	// create proof that we can authorize this block
	babesession.epochThreshold = big.NewInt(0)
	var slotNumber uint64 = 1

	outAndProof, err := babesession.runLottery(slotNumber)
	if err != nil {
		t.Fatal(err)
	}

	if outAndProof == nil {
		t.Fatal("proof was nil when over threshold")
	}

	babesession.slotToProof[slotNumber] = outAndProof

	// see https://github.com/noot/substrate/blob/add-blob/core/test-runtime/src/system.rs#L468
	// add a valid transaction
	txa := []byte{3, 16, 110, 111, 111, 116, 1, 64, 103, 111, 115, 115, 97, 109, 101, 114, 95, 105, 115, 95, 99, 111, 111, 108}
	vtx := transaction.NewValidTransaction(types.Extrinsic(txa), &transaction.Validity{})
	babesession.transactionQueue.Push(vtx)

	// add a transaction that can't be included (transfer from account with no balance)
	// https://github.com/paritytech/substrate/blob/5420de3face1349a97eb954ae71c5b0b940c31de/core/transaction-pool/src/tests.rs#L95
	txb := []byte{1, 212, 53, 147, 199, 21, 253, 211, 28, 97, 20, 26, 189, 4, 169, 159, 214, 130, 44, 133, 88, 133, 76, 205, 227, 154, 86, 132, 231, 165, 109, 162, 125, 142, 175, 4, 21, 22, 135, 115, 99, 38, 201, 254, 161, 126, 37, 252, 82, 135, 97, 54, 147, 201, 18, 144, 156, 178, 38, 170, 71, 148, 242, 106, 72, 69, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 216, 5, 113, 87, 87, 40, 221, 120, 247, 252, 137, 201, 74, 231, 222, 101, 85, 108, 102, 39, 31, 190, 210, 14, 215, 124, 19, 160, 180, 203, 54, 110, 167, 163, 149, 45, 12, 108, 80, 221, 65, 238, 57, 237, 199, 16, 10, 33, 185, 8, 244, 184, 243, 139, 5, 87, 252, 245, 24, 225, 37, 154, 163, 142}
	vtx = transaction.NewValidTransaction(types.Extrinsic(txb), &transaction.Validity{})
	babesession.transactionQueue.Push(vtx)

	zeroHash, err := common.HexToHash("0x00")
	if err != nil {
		t.Fatal(err)
	}

	parentHeader := &types.Header{
		ParentHash: zeroHash,
		Number:     big.NewInt(0),
	}

	slot := Slot{
		start:    uint64(time.Now().Unix()),
		duration: uint64(10000000),
		number:   slotNumber,
	}

	_, err = babesession.buildBlock(parentHeader, slot)
	if err == nil {
		t.Fatal("should error when attempting to include invalid tx")
	}

	txc := babesession.transactionQueue.Peek()
	if !bytes.Equal(txc.Extrinsic, txa) {
		t.Fatal("did not readd valid transaction to queue")
	}
}

func TestDetermineAuthorityIndex(t *testing.T) {
	kpA, err := sr25519.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}

	kpB, err := sr25519.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}

	pubA := kpA.Public().(*sr25519.PublicKey)
	pubB := kpB.Public().(*sr25519.PublicKey)

	authData := []*AuthorityData{
		{ID: pubA, Weight: 1},
		{ID: pubB, Weight: 1},
	}

	bs := &Session{
		authorityData: authData,
		keypair:       kpA,
	}

	err = bs.setAuthorityIndex()
	if err != nil {
		t.Fatal(err)
	}

	if bs.authorityIndex != 0 {
		t.Fatalf("Fail: got %d expected %d", bs.authorityIndex, 0)
	}

	bs = &Session{
		authorityData: authData,
		keypair:       kpB,
	}

	err = bs.setAuthorityIndex()
	if err != nil {
		t.Fatal(err)
	}

	if bs.authorityIndex != 1 {
		t.Fatalf("Fail: got %d expected %d", bs.authorityIndex, 1)
	}
}
