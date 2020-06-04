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
	"sync"
	"sync/atomic"
	"time"

	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
	"github.com/ChainSafe/gossamer/lib/runtime"

	log "github.com/ChainSafe/log15"
)

// RandomnessLength is the length of the epoch randomness (32 bytes)
const RandomnessLength = 32

// Session contains the VRF keys for the validator, as well as BABE configuation data
type Session struct {
	// Storage interfaces
	blockState       BlockState
	storageState     StorageState
	transactionQueue TransactionQueue

	// BABE authority keypair
	keypair *sr25519.Keypair

	// Current runtime
	rt *runtime.Runtime

	// Epoch configuration data
	config         *types.BabeConfiguration
	randomness     [RandomnessLength]byte
	authorityIndex uint64
	authorityData  []*types.AuthorityData
	epochThreshold *big.Int // validator threshold for this epoch
	startSlot      uint64
	slotToProof    map[uint64]*VrfOutputAndProof // for slots where we are a producer, store the vrf output (bytes 0-32) + proof (bytes 32-96)

	// Channels for inter-process communication
	newBlocks chan<- types.Block // send blocks to core service
	epochDone *sync.WaitGroup    // lets core know when the epoch is done
	kill      <-chan struct{}    // kill session if this is closed
	lock      sync.Mutex
	started   uint32

	// Chain synchronization; session is locked for block building while syncing
	syncLock *sync.Mutex
}

// SessionConfig struct
type SessionConfig struct {
	BlockState       BlockState
	StorageState     StorageState
	TransactionQueue TransactionQueue
	Keypair          *sr25519.Keypair
	Runtime          *runtime.Runtime
	NewBlocks        chan<- types.Block
	AuthData         []*types.AuthorityData
	EpochThreshold   *big.Int // should only be used for testing
	StartSlot        uint64   // slot to begin session at
	EpochDone        *sync.WaitGroup
	Kill             <-chan struct{}
	SyncLock         *sync.Mutex
}

// NewSession returns a new Babe session using the provided VRF keys and runtime
func NewSession(cfg *SessionConfig) (*Session, error) {
	if cfg.Keypair == nil {
		return nil, errors.New("cannot create BABE session; no keypair provided")
	}

	if cfg.Kill == nil {
		return nil, errors.New("kill channel is nil")
	}

	if cfg.SyncLock == nil {
		return nil, errors.New("syncLock is nil")
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
		epochDone:        cfg.EpochDone,
		kill:             cfg.Kill,
		syncLock:         cfg.SyncLock,
	}

	if ok := atomic.CompareAndSwapUint32(&babeSession.started, 0, 1); !ok {
		return nil, errors.New("failed to change Session status from stopped to started")
	}

	var err error
	babeSession.config, err = babeSession.rt.BabeConfiguration()
	if err != nil {
		return nil, err
	}

	log.Info("[babe] config", "SlotDuration (ms)", babeSession.config.SlotDuration, "EpochLength (slots)", babeSession.config.EpochLength)

	if babeSession.authorityData == nil {
		log.Info("[babe] setting authority data to genesis authorities", "authorities", babeSession.config.GenesisAuthorities)

		babeSession.authorityData, err = types.AuthorityDataRawToAuthorityData(babeSession.config.GenesisAuthorities)
		if err != nil {
			return nil, err
		}
	}

	log.Info("[babe]", "authorities", babeSession.authorityData)

	babeSession.randomness = babeSession.config.Randomness

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

	i := b.startSlot
	var err error
	for ; i < b.startSlot+b.config.EpochLength; i++ {
		b.slotToProof[i], err = b.runLottery(i)
		if err != nil {
			return fmt.Errorf("error running slot lottery at slot %d: error %s", i, err)
		}
	}

	go b.invokeBlockAuthoring()

	go func() {
		err := b.checkForKill()
		if err != nil {
			log.Error("error running checkForKill", "error", err)
		}
	}()

	return nil
}

func (b *Session) stop() error {
	b.lock.Lock()
	defer b.lock.Unlock()

	if atomic.LoadUint32(&b.started) == uint32(1) {
		if ok := atomic.CompareAndSwapUint32(&b.started, 1, 0); !ok {
			return errors.New("failed to change Session status from started to stopped")
		}

		close(b.newBlocks)
		b.epochDone.Done()
	}

	return nil
}

// Descriptor returns the NextEpochDescriptor for the current session.
func (b *Session) Descriptor() *NextEpochDescriptor {
	return &NextEpochDescriptor{
		Authorities: b.authorityData,
		Randomness:  b.randomness,
	}
}

func (b *Session) safeSend(msg types.Block) error {
	b.lock.Lock()
	defer b.lock.Unlock()
	if atomic.LoadUint32(&b.started) == uint32(0) {
		return errors.New("session has been stopped")
	}
	b.newBlocks <- msg
	return nil
}

// AuthorityData returns the data related to the authority
func (b *Session) AuthorityData() []*types.AuthorityData {
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

func isClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
	}

	return false
}

func (b *Session) checkForKill() error {
	if isClosed(b.kill) {
		err := b.stop()
		if err != nil {
			return err
		}
	}
	return nil
}

func (b *Session) isStopped() bool {
	return atomic.LoadUint32(&b.started) == uint32(0)
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
	bestNum, err := b.blockState.BestBlockNumber()
	if err != nil {
		log.Error("[babe] Failed to get best block number", "error", err)
		return
	}

	// check if we are starting at genesis, if not, need to calculate slot
	if bestNum.Cmp(big.NewInt(0)) == 1 && slotNum == 0 {
		// if we have at least slotTail blcopks, we can run the slotTime algorithm
		if bestNum.Cmp(big.NewInt(int64(slotTail))) != -1 {
			slotNum, err = b.getCurrentSlot()
			if err != nil {
				log.Error("[babe] cannot get current slot", "error", err)
				return
			}
		} else {
			log.Warn("[babe] cannot use median algorithm, not enough blocks synced")

			slotNum, err = b.estimateCurrentSlot()
			if err != nil {
				log.Error("[babe] cannot get current slot", "error", err)
				return
			}
		}
	}

	log.Debug("[babe]", "calculated slot", slotNum)

	for ; slotNum < b.startSlot+b.config.EpochLength; slotNum++ {
		start := time.Now().Unix()
		b.syncLock.Lock()

		if uint64(time.Now().Unix()-start) <= b.config.SlotDuration*1000000 {
			if b.isStopped() {
				return
			}

			b.handleSlot(slotNum)

			// TODO: change this to sleep until start + slotDuration
			time.Sleep(time.Millisecond * time.Duration(b.config.SlotDuration) * 2)
		}

		b.syncLock.Unlock()
	}

	err = b.stop()
	if err != nil {
		log.Error("[babe] block authoring", "error", err)
		return
	}
}

func (b *Session) handleSlot(slotNum uint64) {
	parentHeader, err := b.blockState.BestBlockHeader()
	if err != nil {
		log.Error("[babe] block authoring", "error", "parent header is nil")
		return
	}

	if parentHeader == nil {
		log.Error("[babe] block authoring", "error", "parent header is nil")
		return
	}

	// there is a chance that the best block header may change in the course of building the block,
	// so let's copy it first.
	parent := parentHeader.DeepCopy()

	currentSlot := Slot{
		start:    uint64(time.Now().Unix()),
		duration: b.config.SlotDuration,
		number:   slotNum,
	}

	// TODO: move block authorization check here
	log.Debug("[babe] going to build block", "parent", parent)

	block, err := b.buildBlock(parent, currentSlot)
	if err != nil {
		log.Error("[babe] block authoring", "error", err)
	} else {
		// TODO: loop until slot is done, attempt to produce multiple blocks

		hash := block.Header.Hash()
		log.Info("[babe]", "built block", hash.String(), "number", block.Header.Number, "slot", slotNum)
		log.Debug("[babe] built block", "header", block.Header, "body", block.Body, "parent", parent)

		err = b.safeSend(*block)
		if err != nil {
			log.Error("[babe] Failed to send block to core", "error", err)
			return
		}
	}
}

// runLottery runs the lottery for a specific slot number
// returns an encoded VrfOutput and VrfProof if validator is authorized to produce a block for that slot, nil otherwise
// output = return[0:32]; proof = return[32:96]
func (b *Session) runLottery(slot uint64) (*VrfOutputAndProof, error) {
	slotBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(slotBytes, slot)
	vrfInput := append(slotBytes, b.randomness[:]...)

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
