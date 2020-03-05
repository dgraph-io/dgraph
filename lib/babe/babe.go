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
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/big"
	"time"

	"github.com/ChainSafe/gossamer/dot/core/types"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
	"github.com/ChainSafe/gossamer/lib/runtime"

	log "github.com/ChainSafe/log15"
)

// Session contains the VRF keys for the validator, as well as BABE configuation data
type Session struct {
	blockState       BlockState
	storageState     StorageState
	keypair          *sr25519.Keypair
	rt               *runtime.Runtime
	config           *Configuration
	randomness       [sr25519.VrfOutputLength]byte
	authorityIndex   uint64
	authorityData    []*AuthorityData
	epochThreshold   *big.Int // validator threshold for this epoch
	transactionQueue TransactionQueue
	startSlot        uint64
	slotToProof      map[uint64]*VrfOutputAndProof // for slots where we are a producer, store the vrf output (bytes 0-32) + proof (bytes 32-96)
	newBlocks        chan<- types.Block            // send blocks to core service
	done             chan<- struct{}               // lets core know when the epoch is done
}

// SessionConfig struct
type SessionConfig struct {
	BlockState       BlockState
	StorageState     StorageState
	TransactionQueue TransactionQueue
	Keypair          *sr25519.Keypair
	Runtime          *runtime.Runtime
	NewBlocks        chan<- types.Block
	AuthData         []*AuthorityData
	EpochThreshold   *big.Int // should only be used for testing
	StartSlot        uint64   // slot to begin session at
	Done             chan<- struct{}
}

// NewSession returns a new Babe session using the provided VRF keys and runtime
func NewSession(cfg *SessionConfig) (*Session, error) {
	if cfg.Keypair == nil {
		return nil, errors.New("cannot create BABE session; no keypair provided")
	}

	babeSession := &Session{
		blockState:       cfg.BlockState,
		storageState:     cfg.StorageState,
		keypair:          cfg.Keypair,
		rt:               cfg.Runtime,
		transactionQueue: cfg.TransactionQueue,
		slotToProof:      make(map[uint64]*VrfOutputAndProof),
		newBlocks:        cfg.NewBlocks,
		authorityData:    cfg.AuthData,
		epochThreshold:   cfg.EpochThreshold,
		startSlot:        cfg.StartSlot,
		done:             cfg.Done,
	}

	err := babeSession.configurationFromRuntime()
	if err != nil {
		return nil, err
	}

	log.Info("[babe] config", "SlotDuration (ms)", babeSession.config.SlotDuration, "EpochLength (slots)", babeSession.config.EpochLength)

	babeSession.randomness = [sr25519.VrfOutputLength]byte{babeSession.config.Randomness}

	err = babeSession.setAuthorityIndex()
	if err != nil {
		return nil, err
	}

	log.Trace("[babe]", "authority index", babeSession.authorityIndex)

	return babeSession, nil
}

// Start a session
func (b *Session) Start() error {
	if b.epochThreshold == nil {
		err := b.setEpochThreshold()
		if err != nil {
			return err
		}
	}

	log.Trace("[babe]", "epochThreshold", b.epochThreshold)

	var i uint64 = b.startSlot
	var err error
	for ; i < b.startSlot+b.config.EpochLength; i++ {
		b.slotToProof[i], err = b.runLottery(i)
		if err != nil {
			return fmt.Errorf("error running slot lottery at slot %d: error %s", i, err)
		}
	}

	go b.invokeBlockAuthoring()

	return nil
}

// AuthorityData returns the data related to the authority
func (b *Session) AuthorityData() []*AuthorityData {
	return b.authorityData
}

// SetEpochData will set the authorityData and randomness
func (b *Session) SetEpochData(data *NextEpochDescriptor) error {
	b.authorityData = data.Authorities
	b.randomness = data.Randomness
	return b.setAuthorityIndex()
}

func (b *Session) setAuthorityIndex() error {
	pub := b.keypair.Public()

	log.Debug("[babe]", "authority key", pub.Hex(), "authorities", b.authorityData)

	for i, auth := range b.authorityData {
		if bytes.Equal(pub.Encode(), auth.ID.Encode()) {
			b.authorityIndex = uint64(i)
			return nil
		}
	}

	return fmt.Errorf("key not in BABE authority data")
}

func (b *Session) invokeBlockAuthoring() {
	if b.config == nil {
		log.Error("[babe] block authoring", "error", "config is nil")
		return
	}

	if b.blockState == nil {
		log.Error("[babe] block authoring", "error", "blockState is nil")
		return
	}

	if b.storageState == nil {
		log.Error("[babe] block authoring", "error", "storageState is nil")
		return
	}

	slotNum := b.startSlot

	bestNum := b.blockState.HighestBlockNumber()
	log.Debug("[babe]", "highest block num", bestNum)

	var err error
	// check if we are starting at genesis, if not, need to calculate slot
	if bestNum.Cmp(big.NewInt(0)) == 1 && slotNum == 0 {
		// TODO: change this to getCurrentSlot, once BlockResponse messages are implemented
		slotNum, err = b.estimateCurrentSlot()
		if err != nil {
			log.Error("[babe] cannot estimate current slot", "error", err)
			return
		}

		log.Debug("[babe]", "estimated slot", slotNum)
	}

	for ; slotNum < b.startSlot+b.config.EpochLength; slotNum++ {
		b.handleSlot(slotNum)
		time.Sleep(time.Millisecond * time.Duration(b.config.SlotDuration))
	}

	if b.newBlocks != nil {
		close(b.newBlocks)
	}

	if b.done != nil {
		close(b.done)
	}
}

func (b *Session) handleSlot(slotNum uint64) {
	parentHeader, err := b.blockState.BestBlockHeader()
	if err != nil {
		log.Error("BABE block authoring", "error", "parent header is nil")
		return
	}

	if parentHeader == nil {
		log.Error("BABE block authoring", "error", "parent header is nil")
		return
	}

	currentSlot := Slot{
		start:    uint64(time.Now().Unix()),
		duration: b.config.SlotDuration,
		number:   slotNum,
	}

	// TODO: move block authorization check here

	block, err := b.buildBlock(parentHeader, currentSlot)
	if err != nil {
		log.Error("BABE block authoring", "error", err)
	} else {
		// TODO: loop until slot is done, attempt to produce multiple blocks

		hash := block.Header.Hash()
		log.Info("BABE", "built block", hash.String(), "number", block.Header.Number, "slot", slotNum)
		log.Debug("BABE built block", "header", block.Header, "body", block.Body)

		b.newBlocks <- *block
	}
}

// runLottery runs the lottery for a specific slot number
// returns an encoded VrfOutput and VrfProof if validator is authorized to produce a block for that slot, nil otherwise
// output = return[0:32]; proof = return[32:96]
func (b *Session) runLottery(slot uint64) (*VrfOutputAndProof, error) {
	slotBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(slotBytes, slot)
	vrfInput := append(slotBytes, b.config.Randomness)

	output, proof, err := b.vrfSign(vrfInput)
	if err != nil {
		return nil, err
	}

	outputInt := big.NewInt(0).SetBytes(output[:])
	if b.epochThreshold == nil {
		err = b.setEpochThreshold()
		if err != nil {
			return nil, err
		}
	}

	if outputInt.Cmp(b.epochThreshold) > 0 {
		outbytes := [sr25519.VrfOutputLength]byte{}
		copy(outbytes[:], output)
		proofbytes := [sr25519.VrfProofLength]byte{}
		copy(proofbytes[:], proof)
		log.Trace("[babe] lottery", "won slot", slot)
		return &VrfOutputAndProof{
			output: outbytes,
			proof:  proofbytes,
		}, nil
	}

	return nil, nil
}

func (b *Session) vrfSign(input []byte) (out []byte, proof []byte, err error) {
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
		weights[i] = auth.Weight
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
