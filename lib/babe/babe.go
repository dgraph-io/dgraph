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
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"os"
	"sync"
	"time"

	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
	"github.com/ChainSafe/gossamer/lib/runtime"
	log "github.com/ChainSafe/log15"
)

var (
	// MaxThreshold is the maximum BABE threshold (node authorized to produce a block every slot)
	MaxThreshold = big.NewInt(0).SetBytes([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
	// MinThreshold is the minimum BABE threshold (node never authorized to produce a block)
	MinThreshold = big.NewInt(0)
)

// Service contains the VRF keys for the validator, as well as BABE configuation data
type Service struct {
	logger log.Logger
	ctx    context.Context
	cancel context.CancelFunc
	paused bool

	// Storage interfaces
	blockState       BlockState
	storageState     StorageState
	transactionQueue TransactionQueue
	epochState       EpochState

	// BABE authority keypair
	keypair *sr25519.Keypair // TODO: change to BABE keystore

	// Current runtime
	rt *runtime.Runtime

	// Epoch configuration data
	config         *types.BabeConfiguration
	randomness     [types.RandomnessLength]byte
	authorityIndex uint64
	authorityData  []*types.Authority
	epochThreshold *big.Int // validator threshold
	startSlot      uint64
	slotToProof    map[uint64]*VrfOutputAndProof // for slots where we are a producer, store the vrf output (bytes 0-32) + proof (bytes 32-96)

	// Channels for inter-process communication
	blockChan chan types.Block // send blocks to core service

	// State variables
	lock  sync.Mutex
	pause chan struct{}
}

// ServiceConfig represents a BABE configuration
type ServiceConfig struct {
	LogLvl           log.Lvl
	BlockState       BlockState
	StorageState     StorageState
	TransactionQueue TransactionQueue
	EpochState       EpochState
	Keypair          *sr25519.Keypair
	Runtime          *runtime.Runtime
	AuthData         []*types.Authority
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

	if cfg.EpochState == nil {
		return nil, errors.New("epochState is nil")
	}

	if cfg.Runtime == nil {
		return nil, errors.New("runtime is nil")
	}

	logger := log.New("pkg", "babe")
	h := log.StreamHandler(os.Stdout, log.TerminalFormat())
	h = log.CallerFileHandler(h)
	logger.SetHandler(log.LvlFilterHandler(cfg.LogLvl, h))

	ctx, cancel := context.WithCancel(context.Background())

	babeService := &Service{
		logger:           logger,
		ctx:              ctx,
		cancel:           cancel,
		blockState:       cfg.BlockState,
		storageState:     cfg.StorageState,
		epochState:       cfg.EpochState,
		keypair:          cfg.Keypair,
		rt:               cfg.Runtime,
		transactionQueue: cfg.TransactionQueue,
		slotToProof:      make(map[uint64]*VrfOutputAndProof),
		blockChan:        make(chan types.Block),
		authorityData:    cfg.AuthData,
		epochThreshold:   cfg.EpochThreshold,
		startSlot:        cfg.StartSlot,
		pause:            make(chan struct{}),
	}

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

		babeService.authorityData, err = types.BABEAuthorityRawToAuthority(babeService.config.GenesisAuthorities)
		if err != nil {
			return nil, err
		}
	}

	err = babeService.setAuthorityIndex()
	if err != nil {
		return nil, err
	}

	logger.Debug("created BABE service", "authorities", AuthorityData(babeService.authorityData), "authority index", babeService.authorityIndex, "threshold", babeService.epochThreshold)
	return babeService, nil
}

// Start starts BABE block authoring
func (b *Service) Start() error {
	if b.epochThreshold == nil {
		err := b.setEpochThreshold()
		if err != nil {
			return err
		}
	}

	epoch, err := b.epochState.GetCurrentEpoch()
	if err != nil {
		b.logger.Error("failed to get current epoch", "error", err)
		return err
	}

	err = b.initiateEpoch(epoch, b.startSlot)
	if err != nil {
		b.logger.Error("failed to initiate epoch", "error", err)
		return err
	}

	go b.initiate()
	return nil
}

// Pause pauses the service ie. halts block production
func (b *Service) Pause() error {
	if b.paused {
		return errors.New("service already paused")
	}

	select {
	case b.pause <- struct{}{}:
		b.logger.Info("service paused")
	default:
	}

	b.paused = true
	return nil
}

// Resume resumes the service ie. resumes block production
func (b *Service) Resume() error {
	if !b.paused {
		return errors.New("service not paused")
	}

	go b.initiate()
	b.paused = false
	b.logger.Info("service resumed")
	return nil
}

// Stop stops the service. If stop is called, it cannot be resumed.
func (b *Service) Stop() error {
	b.lock.Lock()
	defer b.lock.Unlock()

	if b.ctx.Err() != nil {
		return errors.New("service already stopped")
	}

	b.cancel()
	close(b.blockChan)
	return nil
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

// Descriptor returns the Descriptor for the current Service.
func (b *Service) Descriptor() *Descriptor {
	return &Descriptor{
		AuthorityData: b.authorityData,
		Randomness:    b.randomness,
		Threshold:     b.epochThreshold,
	}
}

// Authorities returns the current BABE authorities
func (b *Service) Authorities() []*types.Authority {
	return b.authorityData
}

// SetAuthorities sets the current Block Producer Authorities and sets Authority index
func (b *Service) SetAuthorities(data []*types.Authority) error {
	// check key is in new Authorities list before we update Authorities Data
	pub := b.keypair.Public()
	found := false
	for _, auth := range data {
		if bytes.Equal(pub.Encode(), auth.Key.Encode()) {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("key not in BABE authority data")
	}

	b.authorityData = data
	return b.setAuthorityIndex()
}

// SetEpochThreshold sets Epoch Threshold for BABE producer
func (b *Service) SetEpochThreshold(a *big.Int) {
	b.epochThreshold = a
}

// SetRandomness sets randomness for BABE service
// Note that this only takes effect at the end of an epoch, where it is used to calculate the next epoch's randomness
func (b *Service) SetRandomness(a [types.RandomnessLength]byte) {
	b.randomness = a
}

// IsStopped returns true if the service is stopped (ie not producing blocks)
func (b *Service) IsStopped() bool {
	return b.ctx.Err() != nil
}

// IsPaused returns if the service is paused or not (ie. producing blocks)
func (b *Service) IsPaused() bool {
	return b.paused
}

func (b *Service) safeSend(msg types.Block) error {
	defer func() {
		if err := recover(); err != nil {
			b.logger.Error("recovered from panic", "error", err)
		}
	}()

	b.lock.Lock()
	defer b.lock.Unlock()

	if b.IsStopped() {
		return errors.New("Service has been stopped")
	}

	b.blockChan <- msg
	return nil
}

func (b *Service) setAuthorityIndex() error {
	pub := b.keypair.Public()

	b.logger.Debug("set authority index", "authority key", pub.Hex(), "authorities", AuthorityData(b.authorityData))

	for i, auth := range b.authorityData {
		if bytes.Equal(pub.Encode(), auth.Key.Encode()) {
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

	b.logger.Debug("calculated slot", "number", slotNum)
	b.invokeBlockAuthoring(slotNum)
}

func (b *Service) invokeBlockAuthoring(startSlot uint64) {
	currEpoch, err := b.epochState.GetCurrentEpoch()
	if err != nil {
		b.logger.Error("failed to get current epoch", "error", err)
		return
	}

	// get start slot for current epoch
	epochStart, err := b.epochState.GetStartSlotForEpoch(0)
	if err != nil {
		b.logger.Error("failed to get start slot for current epoch", "epoch", currEpoch, "error", err)
		return
	}

	intoEpoch := startSlot - epochStart
	b.logger.Info("current epoch", "epoch", currEpoch, "slots into epoch", intoEpoch)

	// starting slot for next epoch
	nextStartSlot := startSlot + b.config.EpochLength - intoEpoch

	slotDone := make([]<-chan time.Time, b.config.EpochLength-intoEpoch)
	for i := 0; i < int(b.config.EpochLength-intoEpoch); i++ {
		slotDone[i] = time.After(b.slotDuration() * time.Duration(i))
	}

	for i := 0; i < int(b.config.EpochLength-intoEpoch); i++ {
		select {
		case <-b.ctx.Done():
			return
		case <-b.pause:
			return
		case <-slotDone[i]:
			slotNum := startSlot + uint64(i)
			err = b.handleSlot(slotNum)
			if err != nil {
				b.logger.Warn("failed to handle slot", "slot", slotNum, "error", err)
				continue
			}
		}
	}

	// setup next epoch, re-invoke block authoring
	next, err := b.incrementEpoch()
	if err != nil {
		b.logger.Error("failed to increment epoch", "error", err)
		return
	}

	b.logger.Info("initiating epoch", "number", next, "start slot", nextStartSlot)

	err = b.initiateEpoch(next, nextStartSlot)
	if err != nil {
		b.logger.Error("failed to initiate epoch", "epoch", next, "error", err)
		return
	}

	b.invokeBlockAuthoring(nextStartSlot)
}

func (b *Service) handleSlot(slotNum uint64) error {
	if b.slotToProof[slotNum] == nil {
		// if we don't have a proof already set, re-run lottery.
		proof, err := b.runLottery(slotNum)
		if err != nil {
			b.logger.Warn("failed to run lottery", "slot", slotNum)
			return errors.New("failed to run lottery")
		}

		if proof == nil {
			b.logger.Debug("not authorized to produce block", "slot", slotNum)
			return ErrNotAuthorized
		}

		b.slotToProof[slotNum] = proof
	}

	parentHeader, err := b.blockState.BestBlockHeader()
	if err != nil {
		b.logger.Error("block authoring", "error", err)
		return err
	}

	if parentHeader == nil {
		b.logger.Error("block authoring", "error", "parent header is nil")
		return err
	}

	// there is a chance that the best block header may change in the course of building the block,
	// so let's copy it first.
	parent := parentHeader.DeepCopy()

	currentSlot := Slot{
		start:    uint64(time.Now().Unix()),
		duration: b.config.SlotDuration,
		number:   slotNum,
	}

	b.logger.Debug("going to build block", "parent", parent)

	// set runtime trie before building block
	// if block building is successful, store the resulting trie in the storage state
	ts, err := b.storageState.TrieState(&parent.StateRoot)
	if err != nil {
		b.logger.Error("failed to get parent trie", "parent state root", parent.StateRoot, "error", err)
		return err
	}

	b.rt.SetContext(ts)

	block, err := b.buildBlock(parent, currentSlot)
	if err != nil {
		b.logger.Debug("block authoring", "error", err)
		return nil
	}

	// block built successfully, store resulting trie in storage state
	// TODO: why does StateRoot not match the root of the trie after building a block?
	err = b.storageState.StoreTrie(block.Header.StateRoot, ts)
	if err != nil {
		b.logger.Error("failed to store trie in storage state", "error", err)
		return err
	}

	hash := block.Header.Hash()
	b.logger.Info("built block", "hash", hash.String(), "number", block.Header.Number, "slot", slotNum)
	b.logger.Debug("built block", "header", block.Header, "body", block.Body, "parent", parent.Hash())

	err = b.safeSend(*block)
	if err != nil {
		b.logger.Error("failed to send block to core", "error", err)
		return err
	}
	return nil
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
