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
	"os"
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

var (
	// MaxThreshold is the maximum BABE threshold (node authorized to produce a block every slot)
	MaxThreshold = big.NewInt(0).SetBytes([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
	// MinThreshold is the minimum BABE threshold (node never authorized to produce a block)
	MinThreshold = big.NewInt(0)
)

// AuthorityData is an alias for []*types.BABEAuthorityData
type AuthorityData []*types.BABEAuthorityData

// String returns the AuthorityData as a formatted string
func (d AuthorityData) String() string {
	str := ""
	for _, di := range []*types.BABEAuthorityData(d) {
		str = str + fmt.Sprintf("[key=0x%x idx=%d] ", di.ID.Encode(), di.Weight)
	}
	return str
}

// Service contains the VRF keys for the validator, as well as BABE configuation data
type Service struct {
	logger log.Logger

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
	authorityData  []*types.BABEAuthorityData
	epochThreshold *big.Int // validator threshold for this epoch
	startSlot      uint64
	slotToProof    map[uint64]*VrfOutputAndProof // for slots where we are a producer, store the vrf output (bytes 0-32) + proof (bytes 32-96)

	// Channels for inter-process communication
	blockChan chan types.Block // send blocks to core service

	// State variables
	lock    sync.Mutex
	started atomic.Value
}

// ServiceConfig represents a BABE configuration
type ServiceConfig struct {
	LogLvl           log.Lvl
	BlockState       BlockState
	StorageState     StorageState
	TransactionQueue TransactionQueue
	Keypair          *sr25519.Keypair
	Runtime          *runtime.Runtime
	AuthData         []*types.BABEAuthorityData
	EpochThreshold   *big.Int // for development purposes
	SlotDuration     uint64   // for development purposes; in milliseconds
	StartSlot        uint64   // slot to start at
}

// NewService returns a new Babe Service using the provided VRF keys and runtime
func NewService(cfg *ServiceConfig) (*Service, error) {
	if cfg.Keypair == nil {
		return nil, errors.New("cannot create BABE Service; no keypair provided")
	}

	if cfg.BlockState == nil {
		return nil, errors.New("blockState is nil")
	}

	if cfg.Runtime == nil {
		return nil, errors.New("runtime is nil")
	}

	logger := log.New("pkg", "babe")
	h := log.StreamHandler(os.Stdout, log.TerminalFormat())
	logger.SetHandler(log.LvlFilterHandler(cfg.LogLvl, h))

	babeService := &Service{
		logger:           logger,
		blockState:       cfg.BlockState,
		storageState:     cfg.StorageState,
		keypair:          cfg.Keypair,
		rt:               cfg.Runtime,
		transactionQueue: cfg.TransactionQueue,
		slotToProof:      make(map[uint64]*VrfOutputAndProof),
		blockChan:        make(chan types.Block),
		authorityData:    cfg.AuthData,
		epochThreshold:   cfg.EpochThreshold,
		startSlot:        cfg.StartSlot,
	}

	babeService.started.Store(false)

	var err error
	babeService.config, err = babeService.rt.BabeConfiguration()
	if err != nil {
		return nil, err
	}

	// if slot duration is set via the config file, overwrite the runtime value
	if cfg.SlotDuration > 0 {
		babeService.config.SlotDuration = cfg.SlotDuration
	}

	logger.Info("config", "slot duration (ms)", babeService.config.SlotDuration, "epoch length (slots)", babeService.config.EpochLength)

	if babeService.authorityData == nil {
		logger.Info("setting authority data to genesis authorities", "authorities", babeService.config.GenesisAuthorities)

		babeService.authorityData, err = types.BABEAuthorityDataRawToAuthorityData(babeService.config.GenesisAuthorities)
		if err != nil {
			return nil, err
		}
	}

	logger.Info("created BABE service", "authorities", AuthorityData(babeService.authorityData))

	babeService.randomness = babeService.config.Randomness

	err = babeService.setAuthorityIndex()
	if err != nil {
		return nil, err
	}

	logger.Debug("created BABE service", "authority index", babeService.authorityIndex, "threshold", babeService.epochThreshold)

	return babeService, nil
}

// Start a Service
func (b *Service) Start() error {
	b.started.Store(true)

	if b.epochThreshold == nil {
		err := b.setEpochThreshold()
		if err != nil {
			return err
		}
	}

	b.logger.Debug("[babe]", "epochThreshold", b.epochThreshold)

	i := b.startSlot
	var err error
	for ; i < b.startSlot+b.config.EpochLength; i++ {
		b.slotToProof[i], err = b.runLottery(i)
		if err != nil {
			return fmt.Errorf("error running slot lottery at slot %d: error %s", i, err)
		}
	}

	go b.initiate()
	return nil
}

// Pause pauses the service ie. halts block production
func (b *Service) Pause() error {
	b.started.Store(false)
	b.logger.Info("service paused")
	return nil
}

// Resume resumes the service ie. resumes block production
func (b *Service) Resume() error {
	b.started.Store(true)
	go b.initiate()
	b.logger.Info("service resumed")
	return nil
}

// Stop stops the service. If stop is called, it cannot be resumed.
func (b *Service) Stop() error {
	b.lock.Lock()
	defer b.lock.Unlock()

	if b.started.Load().(bool) {
		b.started.Store(false)
		close(b.blockChan)
	}

	return nil
}

// IsStopped returns true if the service is stopped (ie not producing blocks)
func (b *Service) IsStopped() bool {
	return !b.started.Load().(bool)
}

// SetRuntime sets the service's runtime
func (b *Service) SetRuntime(rt *runtime.Runtime) error {
	b.rt = rt

	var err error
	b.config, err = b.rt.BabeConfiguration()
	return err
}

// GetBlockChannel returns the channel where new blocks are passed
func (b *Service) GetBlockChannel() <-chan types.Block {
	return b.blockChan
}

// Descriptor returns the EpochDescriptor for the current Service.
func (b *Service) Descriptor() *EpochDescriptor {
	return &EpochDescriptor{
		AuthorityData: b.authorityData,
		Randomness:    b.randomness,
		Threshold:     b.epochThreshold,
	}
}

// Authorities returns the current BABE authorities
func (b *Service) Authorities() []*types.BABEAuthorityData {
	return b.authorityData
}

// SetAuthorities sets the current BABE authorities
func (b *Service) SetAuthorities(a []*types.BABEAuthorityData) {
	b.authorityData = a
}

// SetEpochData will set the authorityData and randomness
func (b *Service) SetEpochData(data *NextEpochDescriptor) error {
	b.authorityData = data.Authorities
	b.randomness = data.Randomness
	return b.setAuthorityIndex()
}

func (b *Service) safeSend(msg types.Block) error {
	b.lock.Lock()
	defer b.lock.Unlock()
	if !b.started.Load().(bool) {
		return errors.New("Service has been stopped")
	}
	b.blockChan <- msg
	return nil
}

func (b *Service) setAuthorityIndex() error {
	pub := b.keypair.Public()

	b.logger.Debug("set authority index", "authority key", pub.Hex(), "authorities", AuthorityData(b.authorityData))

	for i, auth := range b.authorityData {
		if bytes.Equal(pub.Encode(), auth.ID.Encode()) {
			b.authorityIndex = uint64(i)
			return nil
		}
	}

	return fmt.Errorf("key not in BABE authority data")
}

func (b *Service) slotDuration() time.Duration {
	return time.Duration(b.config.SlotDuration * 1000000) // SlotDuration in ms, time.Duration in ns
}

func (b *Service) initiate() {
	if b.config == nil {
		b.logger.Error("block authoring", "error", "config is nil")
		return
	}

	if b.blockState == nil {
		b.logger.Error("block authoring", "error", "blockState is nil")
		return
	}

	if b.storageState == nil {
		b.logger.Error("block authoring", "error", "storageState is nil")
		return
	}

	slotNum := b.startSlot
	bestNum, err := b.blockState.BestBlockNumber()
	if err != nil {
		b.logger.Error("Failed to get best block number", "error", err)
		return
	}

	// check if we are starting at genesis, if not, need to calculate slot
	if bestNum.Cmp(big.NewInt(0)) == 1 && slotNum == 0 {
		// if we have at least slotTail blcopks, we can run the slotTime algorithm
		if bestNum.Cmp(big.NewInt(int64(slotTail))) != -1 {
			slotNum, err = b.getCurrentSlot()
			if err != nil {
				b.logger.Error("cannot get current slot", "error", err)
				return
			}
		} else {
			b.logger.Warn("cannot use median algorithm, not enough blocks synced")

			slotNum, err = b.estimateCurrentSlot()
			if err != nil {
				b.logger.Error("cannot get current slot", "error", err)
				return
			}
		}
	}

	b.logger.Debug("[babe]", "calculated slot", slotNum)
	b.invokeBlockAuthoring(slotNum)
}

func (b *Service) invokeBlockAuthoring(startSlot uint64) {
	for slotNum := startSlot; slotNum < startSlot+b.config.EpochLength; slotNum++ {
		start := time.Now()

		if time.Since(start) <= b.slotDuration() {
			if b.IsStopped() {
				return
			}

			b.handleSlot(slotNum)

			// sleep until the slot ends
			until := time.Until(start.Add(b.slotDuration()))
			time.Sleep(until)
		}
	}

	// loop forever
	// TODO: signal to verifier that epoch has changed
	b.invokeBlockAuthoring(startSlot + b.config.EpochLength)
}

func (b *Service) handleSlot(slotNum uint64) {
	if b.slotToProof[slotNum] == nil {
		// if we don't have a proof already set, re-run lottery.
		proof, err := b.runLottery(slotNum)
		if err != nil {
			b.logger.Warn("failed to run lottery", "slot", slotNum)
			return
		}

		if proof == nil {
			b.logger.Debug("not authorized to produce block", "slot", slotNum)
			return
		}

		b.slotToProof[slotNum] = proof
	}

	parentHeader, err := b.blockState.BestBlockHeader()
	if err != nil {
		b.logger.Error("block authoring", "error", "parent header is nil")
		return
	}

	if parentHeader == nil {
		b.logger.Error("block authoring", "error", "parent header is nil")
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
	b.logger.Debug("going to build block", "parent", parent)

	block, err := b.buildBlock(parent, currentSlot)
	if err != nil {
		b.logger.Debug("block authoring", "error", err)
	} else {
		// TODO: loop until slot is done, attempt to produce multiple blocks

		hash := block.Header.Hash()
		b.logger.Info("[babe]", "built block", hash.String(), "number", block.Header.Number, "slot", slotNum)
		b.logger.Debug("built block", "header", block.Header, "body", block.Body, "parent", parent)

		err = b.safeSend(*block)
		if err != nil {
			b.logger.Error("Failed to send block to core", "error", err)
			return
		}
	}
}

// runLottery runs the lottery for a specific slot number
// returns an encoded VrfOutput and VrfProof if validator is authorized to produce a block for that slot, nil otherwise
// output = return[0:32]; proof = return[32:96]
func (b *Service) runLottery(slot uint64) (*VrfOutputAndProof, error) {
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

	if outputInt.Cmp(b.epochThreshold) < 0 {
		outbytes := [sr25519.VrfOutputLength]byte{}
		copy(outbytes[:], output)
		proofbytes := [sr25519.VrfProofLength]byte{}
		copy(proofbytes[:], proof)
		b.logger.Trace("lottery", "won slot", slot)
		return &VrfOutputAndProof{
			output: outbytes,
			proof:  proofbytes,
		}, nil
	}

	return nil, nil
}

func (b *Service) vrfSign(input []byte) (out []byte, proof []byte, err error) {
	return b.keypair.VrfSign(input)
}

// sets the slot lottery threshold for the current epoch
func (b *Service) setEpochThreshold() error {
	var err error
	if b.config == nil {
		return errors.New("cannot set threshold: no babe config")
	}

	b.epochThreshold, err = CalculateThreshold(b.config.C1, b.config.C2, len(b.Authorities()))
	if err != nil {
		return err
	}

	b.logger.Debug("set epoch threshold", "threshold", b.epochThreshold.Bytes())
	return nil
}

// CalculateThreshold calculates the slot lottery threshold
// equation: threshold = 2^128 * (1 - (1-c)^(1/len(authorities))
func CalculateThreshold(C1, C2 uint64, numAuths int) (*big.Int, error) {
	c := float64(C1) / float64(C2)
	if c > 1 {
		return nil, errors.New("invalid C1/C2: greater than 1")
	}

	// 1 / len(authorities)
	theta := float64(1) / float64(numAuths)

	// (1-c)^(theta)
	pp := 1 - c
	pp_exp := math.Pow(pp, theta)

	// 1 - (1-c)^(theta)
	p := 1 - pp_exp
	p_rat := new(big.Rat).SetFloat64(p)

	// 1 << 256
	q := new(big.Int).Lsh(big.NewInt(1), 256)

	// (1 << 128) * (1 - (1-c)^(w_k/sum(w_i)))
	return q.Mul(q, p_rat.Num()).Div(q, p_rat.Denom()), nil
}
