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

package core

import (
	"errors"
	"math/big"
	mrand "math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ChainSafe/gossamer/dot/network"
	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/babe"
	"github.com/ChainSafe/gossamer/lib/blocktree"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/common/optional"
	"github.com/ChainSafe/gossamer/lib/common/variadic"
	"github.com/ChainSafe/gossamer/lib/runtime"

	log "github.com/ChainSafe/log15"
	"golang.org/x/exp/rand"
)

// Verifier deals with block verification, as well as NextEpochDescriptors.
type Verifier interface {
	VerifyBlock(header *types.Header) (bool, error)

	// IncrementEpoch is called when we have received all the blocks for an epoch.
	IncrementEpoch() (*babe.NextEpochDescriptor, error)

	EpochNumber() uint64
}

// Syncer deals with chain syncing by sending block request messages and watching for responses.
type Syncer struct {
	// State interfaces
	blockState       BlockState // retrieve our current head of chain from BlockState
	transactionQueue TransactionQueue

	// Synchronization channels and variables
	blockNumIn       <-chan *big.Int                      // incoming block numbers seen from other nodes that are higher than ours
	msgOut           chan<- network.Message               // channel to send BlockRequest messages to network service
	respIn           <-chan *network.BlockResponseMessage // channel to receive BlockResponse messages from
	lock             *sync.Mutex                          // lock BABE session when syncing
	synced           bool
	requestStart     int64    // block number from which to begin block requests
	highestSeenBlock *big.Int // highest block number we have seen
	runtime          *runtime.Runtime

	// Core service control
	chanLock *sync.Mutex
	started  uint32

	// BABE verification
	verifier Verifier
}

// SyncerConfig is the configuration for the Syncer.
type SyncerConfig struct {
	BlockState       BlockState
	BlockNumIn       <-chan *big.Int
	RespIn           <-chan *network.BlockResponseMessage
	MsgOut           chan<- network.Message
	Lock             *sync.Mutex
	ChanLock         *sync.Mutex
	TransactionQueue TransactionQueue
	Runtime          *runtime.Runtime
	Verifier         Verifier
}

var responseTimeout = 3 * time.Second

// NewSyncer returns a new Syncer
func NewSyncer(cfg *SyncerConfig) (*Syncer, error) {
	if cfg.BlockState == nil {
		return nil, ErrNilBlockState
	}

	if cfg.BlockNumIn == nil {
		return nil, ErrNilChannel("BlockNumIn")
	}

	if cfg.MsgOut == nil {
		return nil, ErrNilChannel("MsgOut")
	}

	if cfg.Verifier == nil {
		return nil, ErrNilVerifier
	}

	if cfg.Runtime == nil {
		return nil, ErrNilRuntime
	}

	return &Syncer{
		blockState:       cfg.BlockState,
		blockNumIn:       cfg.BlockNumIn,
		respIn:           cfg.RespIn,
		msgOut:           cfg.MsgOut,
		lock:             cfg.Lock,
		chanLock:         cfg.ChanLock,
		synced:           true,
		requestStart:     1,
		highestSeenBlock: big.NewInt(0),
		transactionQueue: cfg.TransactionQueue,
		runtime:          cfg.Runtime,
		verifier:         cfg.Verifier,
	}, nil
}

// Start begins the syncer
func (s *Syncer) Start() error {
	if ok := atomic.CompareAndSwapUint32(&s.started, 0, 1); !ok {
		return errors.New("failed to change Syncer from stopped to started")
	}

	go s.watchForBlocks()
	go s.watchForResponses()

	return nil
}

// Stop stops the syncer
func (s *Syncer) Stop() error {
	if ok := atomic.CompareAndSwapUint32(&s.started, 1, 0); !ok {
		return errors.New("failed to change Syncer from started to stopped")
	}

	return nil
}

func (s *Syncer) watchForBlocks() {
	for {
		if atomic.LoadUint32(&s.started) == uint32(0) {
			return
		}

		blockNum, ok := <-s.blockNumIn
		if !ok || blockNum == nil {
			log.Warn("[sync] Failed to receive from blockNumIn channel")
			continue
		}

		if blockNum != nil && s.highestSeenBlock.Cmp(blockNum) == -1 {

			if s.synced {
				s.requestStart = s.highestSeenBlock.Add(s.highestSeenBlock, big.NewInt(1)).Int64()
				s.synced = false
				s.lock.Lock()
			} else {
				s.requestStart = s.highestSeenBlock.Int64()
			}

			s.highestSeenBlock = blockNum
			go s.sendBlockRequest()
		}
	}
}

func (s *Syncer) watchForResponses() {
	for {
		if atomic.LoadUint32(&s.started) == uint32(0) {
			return
		}

		var msg *network.BlockResponseMessage
		var ok bool

		select {
		case msg, ok = <-s.respIn:
			// handle response
			if !ok || msg == nil {
				log.Warn("[sync] Failed to receive from respIn channel")
				continue
			}

			s.processBlockResponse(msg)
		case <-time.After(responseTimeout):
			log.Debug("[sync] timeout waiting for BlockResponse")
			if !s.synced {
				s.lock.Unlock()
				s.synced = true
			}
		}
	}
}

func (s *Syncer) processBlockResponse(msg *network.BlockResponseMessage) {
	// highestInResp will be the highest block in the response
	// it's set to 0 if err != nil
	highestInResp, err := s.processBlockResponseData(msg)
	if err != nil {

		// if we cannot find the parent block in our blocktree, we are missing some blocks, and need to request
		// blocks from farther back in the chain
		if err == blocktree.ErrParentNotFound {
			// set request start
			s.requestStart = s.requestStart - maxResponseSize
			if s.requestStart <= 0 {
				s.requestStart = 1
			}
			log.Trace("[sync] Retrying block request", "start", s.requestStart)
			go s.sendBlockRequest()
		} else {
			log.Error("[sync]", "error", err)
		}

	} else {
		// TODO: max retries before unlocking, in case no response is received

		bestNum, err := s.blockState.BestBlockNumber()
		if err != nil {
			log.Crit("[sync] Failed to get best block number", "error", err)
		} else {

			// check if we are synced or not
			if bestNum.Cmp(s.highestSeenBlock) >= 0 && bestNum.Cmp(big.NewInt(0)) != 0 {
				log.Debug("[sync] All synced up!", "number", bestNum)

				if !s.synced {
					s.lock.Unlock()
					s.synced = true
				}
			} else {
				// not yet synced, send another block request for the following blocks
				s.requestStart = highestInResp + 1
				go s.sendBlockRequest()
			}
		}
	}
}

func (s *Syncer) safeMsgSend(msg network.Message) error {
	s.chanLock.Lock()
	defer s.chanLock.Unlock()

	if atomic.LoadUint32(&s.started) == uint32(0) {
		return ErrServiceStopped
	}

	s.msgOut <- msg
	return nil
}

func (s *Syncer) sendBlockRequest() {
	// generate random ID
	s1 := rand.NewSource(uint64(time.Now().UnixNano()))
	seed := rand.New(s1).Uint64()
	randomID := mrand.New(mrand.NewSource(int64(seed))).Uint64()

	start, err := variadic.NewUint64OrHash(uint64(s.requestStart))
	if err != nil {
		log.Error("[sync] Failed to create StartingBlock", "error", err)
		return
	}

	log.Trace("[sync] Block request", "start", start)

	blockRequest := &network.BlockRequestMessage{
		ID:            randomID, // random
		RequestedData: 3,        // block header + body
		StartingBlock: start,
		EndBlockHash:  optional.NewHash(false, common.Hash{}),
		Direction:     1,
		Max:           optional.NewUint32(false, 0),
	}

	// send block request message to network service
	err = s.safeMsgSend(blockRequest)
	if err != nil {
		log.Error("[sync] Failed to send block request", "error", err)
	}
}

func (s *Syncer) processBlockResponseData(msg *network.BlockResponseMessage) (int64, error) {
	blockData := msg.BlockData
	highestInResp := int64(0)

	for _, bd := range blockData {
		if bd.Header.Exists() {
			header, err := types.NewHeaderFromOptional(bd.Header)
			if err != nil {
				return 0, err
			}

			highestInResp, err = s.handleHeader(header)
			if err != nil {
				return 0, err
			}
		}

		if bd.Body.Exists {
			body, err := types.NewBodyFromOptional(bd.Body)
			if err != nil {
				return 0, err
			}

			err = s.handleBody(body)
			if err != nil {
				return 0, err
			}
		}

		if bd.Header.Exists() && bd.Body.Exists {
			header, err := types.NewHeaderFromOptional(bd.Header)
			if err != nil {
				return 0, err
			}

			body, err := types.NewBodyFromOptional(bd.Body)
			if err != nil {
				return 0, err
			}

			block := &types.Block{
				Header: header,
				Body:   body,
			}

			err = s.handleBlock(block)
			if err != nil {
				return 0, err
			}
		}

		err := s.blockState.CompareAndSetBlockData(bd)
		if err != nil {
			return highestInResp, err
		}
	}

	return highestInResp, nil
}

// handleHeader handles headers included in BlockResponses
func (s *Syncer) handleHeader(header *types.Header) (int64, error) {
	highestInResp := int64(0)

	// get block header; if exists, return
	has, err := s.blockState.HasHeader(header.Hash())
	if err != nil {
		return 0, err
	}

	if !has {
		err = s.blockState.SetHeader(header)
		if err != nil {
			return 0, err
		}

		log.Info("[sync] saved block header", "hash", header.Hash(), "number", header.Number)
	}

	ok, err := s.verifier.VerifyBlock(header)
	if err != nil {
		return 0, err
	}

	if !ok {
		return 0, ErrInvalidBlock
	}

	if header.Number.Int64() > highestInResp {
		highestInResp = header.Number.Int64()
	}

	return highestInResp, nil
}

// handleHeader handles block bodies included in BlockResponses
func (s *Syncer) handleBody(body *types.Body) error {
	exts, err := body.AsExtrinsics()
	if err != nil {
		log.Error("[sync] cannot parse body as extrinsics", "error", err)
		return err
	}

	for _, ext := range exts {
		s.transactionQueue.RemoveExtrinsic(ext)
	}

	return err
}

// handleHeader handles blocks (header+body) included in BlockResponses
func (s *Syncer) handleBlock(block *types.Block) error {
	_, err := s.executeBlock(block)
	if err != nil {
		return err
	}

	err = s.blockState.AddBlock(block)
	if err != nil {
		if err == blocktree.ErrParentNotFound && block.Header.Number.Cmp(big.NewInt(0)) != 0 {
			return err
		} else if err == blocktree.ErrBlockExists || block.Header.Number.Cmp(big.NewInt(0)) == 0 {
			// this is fine
		} else {
			return err
		}
	} else {
		log.Info("[sync] imported block", "number", block.Header.Number, "hash", block.Header.Hash())
		log.Debug("[sync] imported block", "header", block.Header, "body", block.Body)
	}

	// TODO: if block is from the next epoch, increment epoch

	return nil
}

// runs the block through runtime function Core_execute_block
//  It doesn't seem to return data on success (although the spec say it should return
//  a boolean value that indicate success.  will error if the call isn't successful
func (s *Syncer) executeBlock(block *types.Block) ([]byte, error) {
	// copy block since we're going to modify it
	b := block.DeepCopy()

	b.Header.Digest = [][]byte{}
	bdEnc, err := b.Encode()
	if err != nil {
		return nil, err
	}

	return s.runtime.Exec(runtime.CoreExecuteBlock, bdEnc)
}

func (s *Syncer) executeBlockBytes(bd []byte) ([]byte, error) {
	return s.runtime.Exec(runtime.CoreExecuteBlock, bd)
}
