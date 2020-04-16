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
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
	"github.com/ChainSafe/gossamer/lib/transaction"

	"github.com/stretchr/testify/require"
)

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

func addAuthorshipProof(t *testing.T, babesession *Session, slotNumber uint64) {
	outAndProof, err := babesession.runLottery(slotNumber)
	if err != nil {
		t.Fatal(err)
	}

	if outAndProof == nil {
		t.Fatal("proof was nil when over threshold")
	}

	babesession.slotToProof[slotNumber] = outAndProof
}

func createTestBlock(t *testing.T, babesession *Session, exts [][]byte) (*types.Block, Slot) {
	// create proof that we can authorize this block
	babesession.epochThreshold = big.NewInt(0)
	babesession.authorityIndex = 0

	slotNumber := uint64(1)

	addAuthorshipProof(t, babesession, slotNumber)

	for _, ext := range exts {
		vtx := transaction.NewValidTransaction(ext, &transaction.Validity{})
		_, _ = babesession.transactionQueue.Push(vtx)
	}

	parentHeader := genesisHeader

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

	block, slot := createTestBlock(t, babesession, exts)

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
		ParentHash:     genesisHeader.Hash(),
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
	require.Equal(t, "cannot build extrinsics: Error during apply extrinsic: Apply error, type: Payment",
		err.Error(), "Did not receive expected error text")

	txc := babesession.transactionQueue.Peek()
	if !bytes.Equal(txc.Extrinsic, txa) {
		t.Fatal("did not readd valid transaction to queue")
	}
}
