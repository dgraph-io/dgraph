package babe

import (
	"math/big"
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/runtime"
	"github.com/ChainSafe/gossamer/tests"

	"github.com/ChainSafe/gossamer/crypto/sr25519"
)

func TestVerifySlotWinner(t *testing.T) {
	rt := runtime.NewTestRuntime(t, tests.POLKADOT_RUNTIME)
	kp, err := sr25519.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}

	cfg := &SessionConfig{
		Runtime: rt,
		Keypair: kp,
	}

	babesession, err := NewSession(cfg)
	if err != nil {
		t.Fatal(err)
	}
	err = babesession.configurationFromRuntime()
	if err != nil {
		t.Fatal(err)
	}

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

	slot := Slot{
		start:    uint64(time.Now().Unix()),
		duration: uint64(10000000),
		number:   slotNumber,
	}

	// create babe header
	babeHeader, err := babesession.buildBlockBabeHeader(slot)
	if err != nil {
		t.Fatal(err)
	}

	babesession.authorityData = make([]AuthorityData, 1)
	babesession.authorityData[0] = AuthorityData{
		id: kp.Public().(*sr25519.PublicKey),
	}

	ok, err := babesession.verifySlotWinner(slot.number, babeHeader)
	if err != nil {
		t.Fatal(err)
	}

	if !ok {
		t.Fatal("did not verify slot winner")
	}
}

func TestVerifyAuthorshipRight(t *testing.T) {
	rt := runtime.NewTestRuntime(t, tests.POLKADOT_RUNTIME)
	kp, err := sr25519.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}

	cfg := &SessionConfig{
		Runtime: rt,
		Keypair: kp,
	}

	babesession, err := NewSession(cfg)
	if err != nil {
		t.Fatal(err)
	}
	err = babesession.configurationFromRuntime()
	if err != nil {
		t.Fatal(err)
	}

	babesession.authorityData = make([]AuthorityData, 1)
	babesession.authorityData[0] = AuthorityData{
		id:     kp.Public().(*sr25519.PublicKey),
		weight: 1,
	}

	block, slot := createTestBlock(babesession, t)

	ok, err := babesession.verifyAuthorshipRight(slot.number, block.Header)
	if err != nil {
		t.Fatal(err)
	}

	if !ok {
		t.Fatal("did not verify authorship right")
	}
}
