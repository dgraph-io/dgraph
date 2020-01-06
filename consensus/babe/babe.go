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
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/big"
	"time"

	schnorrkel "github.com/ChainSafe/go-schnorrkel"
	"github.com/ChainSafe/gossamer/codec"
	tx "github.com/ChainSafe/gossamer/common/transaction"
	"github.com/ChainSafe/gossamer/core/types"
	"github.com/ChainSafe/gossamer/crypto/sr25519"
	"github.com/ChainSafe/gossamer/runtime"
	log "github.com/ChainSafe/log15"
)

// Session contains the VRF keys for the validator, as well as BABE configuation data
type Session struct {
	keypair        *sr25519.Keypair
	rt             *runtime.Runtime
	config         *BabeConfiguration
	authorityIndex uint64
	authorityData  []AuthorityData
	epochThreshold *big.Int // validator threshold for this epoch
	txQueue        *tx.PriorityQueue
	slotToProof    map[uint64][]byte  // for slots where we are a producer, store the vrf output+proof
	newBlocks      chan<- types.Block // send blocks to core service
}

type SessionConfig struct {
	Keypair   *sr25519.Keypair
	Runtime   *runtime.Runtime
	NewBlocks chan<- types.Block
}

// NewSession returns a new Babe session using the provided VRF keys and runtime
func NewSession(cfg *SessionConfig) (*Session, error) {
	if cfg.Keypair == nil {
		return nil, errors.New("cannot start BABE session; no keypair provided")
	}

	babeSession := &Session{
		keypair:     cfg.Keypair,
		rt:          cfg.Runtime,
		txQueue:     new(tx.PriorityQueue),
		slotToProof: make(map[uint64][]byte),
		newBlocks:   cfg.NewBlocks,
	}

	err := babeSession.configurationFromRuntime()
	if err != nil {
		return nil, err
	}

	return babeSession, nil
}

func (b *Session) Start() error {
	var i uint64 = 0
	var err error
	for ; i < b.config.EpochLength; i++ {
		b.slotToProof[i], err = b.runLottery(i)
		if err != nil {
			return fmt.Errorf("BABE: error running slot lottery at slot %d: error %s", i, err)
		}
	}

	//TODO: finish implementation of build block
	go b.invokeBlockAuthoring()

	return nil
}

// PushToTxQueue adds a ValidTransaction to BABE's transaction queue
func (b *Session) PushToTxQueue(vt *tx.ValidTransaction) {
	b.txQueue.Insert(vt)
}

func (b *Session) PeekFromTxQueue() *tx.ValidTransaction {
	return b.txQueue.Peek()
}

func (b *Session) invokeBlockAuthoring() {
	// TODO: we might not actually be starting at slot 0, need to run median algorithm here
	var currentSlot uint64 = 0

	for ; currentSlot < b.config.EpochLength; currentSlot++ {
		// TODO: call buildBlock
		b.newBlocks <- types.Block{
			Header: &types.BlockHeader{
				Number: big.NewInt(0),
			},
		}
		time.Sleep(time.Millisecond * time.Duration(b.config.SlotDuration))
	}
}

// runLottery runs the lottery for a specific slot number
// returns an encoded VrfOutput and VrfProof if validator is authorized to produce a block for that slot, nil otherwise
func (b *Session) runLottery(slot uint64) ([]byte, error) {
	slotBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(slotBytes, slot)
	vrfInput := append(slotBytes, b.config.Randomness)

	output, proof, err := b.vrfSign(vrfInput)
	if err != nil {
		return nil, err
	}

	outbytes := output.Encode()
	outputInt := big.NewInt(0).SetBytes(outbytes[:])
	if b.epochThreshold == nil {
		err = b.setEpochThreshold()
		if err != nil {
			return nil, err
		}
	}

	if outputInt.Cmp(b.epochThreshold) > 0 {
		proofbytes := proof.Encode()
		return append(outbytes[:], proofbytes...), nil
	}

	return nil, nil
}

func (b *Session) vrfSign(input []byte) (*schnorrkel.VrfOutput, *schnorrkel.VrfProof, error) {
	return b.keypair.VrfSign(input)
}

// sets the slot lottery threshold for the current epoch
func (b *Session) setEpochThreshold() error {
	var err error
	if b.config == nil {
		return errors.New("cannot set threshold: no babe config")
	}

	b.epochThreshold, err = calculateThreshold(b.config.C1, b.config.C2, b.authorityIndex, b.authorityWeights())
	if err != nil {
		return err
	}

	return nil
}

func (b *Session) authorityWeights() []uint64 {
	weights := make([]uint64, len(b.authorityData))
	for i, auth := range b.authorityData {
		weights[i] = auth.weight
	}
	return weights
}

// calculates the slot lottery threshold for the authority at authorityIndex.
// equation: threshold = 2^128 * (1 - (1-c)^(w_k/sum(w_i)))
// where k is the authority index, and sum(w_i) is the
// sum of all the authority weights
// see: https://github.com/paritytech/substrate/blob/master/core/consensus/babe/src/lib.rs#L1022
func calculateThreshold(C1, C2, authorityIndex uint64, authorityWeights []uint64) (*big.Int, error) {
	c := float64(C1) / float64(C2)
	if c > 1 {
		return nil, errors.New("invalid C1/C2: greater than 1")
	}

	// sum(w_i)
	var sum uint64 = 0
	for _, weight := range authorityWeights {
		sum += weight
	}

	if sum == 0 {
		return nil, errors.New("invalid authority weights: sums to zero")
	}

	// w_k/sum(w_i)
	theta := float64(authorityWeights[authorityIndex]) / float64(sum)

	// (1-c)^(w_k/sum(w_i)))
	pp := 1 - c
	pp_exp := math.Pow(pp, theta)

	// 1 - (1-c)^(w_k/sum(w_i)))
	p := 1 - pp_exp
	p_rat := new(big.Rat).SetFloat64(p)

	// 1 << 128
	q := new(big.Int).Lsh(big.NewInt(1), 128)

	// (1 << 128) * (1 - (1-c)^(w_k/sum(w_i)))
	return q.Mul(q, p_rat.Num()).Div(q, p_rat.Denom()), nil
}

// construct a block for this slot with the given parent
func (b *Session) buildBlock(parent *types.BlockHeader, slot Slot) (*types.Block, error) {
	log.Debug("build-block", "parent", parent, "slot", slot)

	// initialize block
	encodedHeader, err := codec.Encode(parent)
	if err != nil {
		return nil, err
	}
	err = b.initializeBlock(encodedHeader)
	if err != nil {
		return nil, err
	}

	// add block inherents
	err = b.buildBlockInherents(slot)
	if err != nil {
		return nil, err
	}

	// finalize block
	block, err := b.finalizeBlock()
	if err != nil {
		return nil, err
	}

	block.Header.Number.Add(parent.Number, big.NewInt(1))
	return block, nil
}

// buildBlockInherents applies the inherents for a block
func (b *Session) buildBlockInherents(slot Slot) error {
	// Setup inherents: add timstap0 and babeslot
	idata := NewInherentsData()
	err := idata.SetInt64Inherent(Timstap0, uint64(time.Now().Unix()))
	if err != nil {
		return err
	}

	err = idata.SetInt64Inherent(Babeslot, slot.number)
	if err != nil {
		return err
	}

	ienc, err := idata.Encode()
	if err != nil {
		return err
	}

	// Call BlockBuilder_inherent_extrinsics
	_, err = b.inherentExtrinsics(ienc)
	if err != nil {
		return err
	}

	return nil
}
