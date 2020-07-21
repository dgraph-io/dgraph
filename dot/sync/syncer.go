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

package sync

import (
	"errors"
	"math/big"
	mrand "math/rand"
	"os"
	"time"

	"github.com/ChainSafe/gossamer/dot/network"
	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/blocktree"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/common/optional"
	"github.com/ChainSafe/gossamer/lib/common/variadic"
	"github.com/ChainSafe/gossamer/lib/runtime"

	log "github.com/ChainSafe/log15"
	"golang.org/x/exp/rand"
)

// Service deals with chain syncing by sending block request messages and watching for responses.
type Service struct {
	logger log.Logger

	// State interfaces
	blockState       BlockState // retrieve our current head of chain from BlockState
	transactionQueue TransactionQueue
	blockProducer    BlockProducer

	// Synchronization variables
	synced           bool
	requestStart     int64    // block number from which to begin block requests
	highestSeenBlock *big.Int // highest block number we have seen
	runtime          *runtime.Runtime

	// BABE verification
	verifier Verifier

	// Consensus digest handling
	digestHandler DigestHandler

	// Benchmarker
	benchmarker *benchmarker
}

// Config is the configuration for the sync Service.
type Config struct {
	LogLvl           log.Lvl
	BlockState       BlockState
	BlockProducer    BlockProducer
	TransactionQueue TransactionQueue
	Runtime          *runtime.Runtime
	Verifier         Verifier
	DigestHandler    DigestHandler
}

// NewService returns a new *sync.Service
func NewService(cfg *Config) (*Service, error) {
	if cfg.BlockState == nil {
		return nil, ErrNilBlockState
	}

	if cfg.Verifier == nil {
		return nil, ErrNilVerifier
	}

	if cfg.Runtime == nil {
		return nil, ErrNilRuntime
	}

	if cfg.BlockProducer == nil {
		cfg.BlockProducer = newMockBlockProducer()
	}

	logger := log.New("pkg", "sync")
	h := log.StreamHandler(os.Stdout, log.TerminalFormat())
	logger.SetHandler(log.LvlFilterHandler(cfg.LogLvl, h))

	return &Service{
		logger:           logger,
		blockState:       cfg.BlockState,
		blockProducer:    cfg.BlockProducer,
		synced:           true,
		requestStart:     1,
		highestSeenBlock: big.NewInt(0),
		transactionQueue: cfg.TransactionQueue,
		runtime:          cfg.Runtime,
		verifier:         cfg.Verifier,
		digestHandler:    cfg.DigestHandler,
		benchmarker:      newBenchmarker(logger),
	}, nil
}

// HandleSeenBlocks handles a block that is newly "seen" ie. a block that a peer claims to have through a StatusMessage
func (s *Service) HandleSeenBlocks(blockNum *big.Int) *network.BlockRequestMessage {
	if blockNum == nil || s.highestSeenBlock.Cmp(blockNum) != -1 {
		return nil
	}

	// need to sync
	if s.synced {
		s.requestStart = s.highestSeenBlock.Add(s.highestSeenBlock, big.NewInt(1)).Int64()
		s.synced = false

		err := s.blockProducer.Pause()
		if err != nil {
			s.logger.Warn("failed to pause block production")
		}
	} else {
		s.requestStart = s.highestSeenBlock.Int64()
	}

	s.highestSeenBlock = blockNum
	s.benchmarker.begin(uint64(s.requestStart))
	return s.createBlockRequest()
}

// HandleBlockAnnounce creates a block request message from the block
// announce messages (block announce messages include the header but the full
// block is required to execute `core_execute_block`).
func (s *Service) HandleBlockAnnounce(msg *network.BlockAnnounceMessage) *network.BlockRequestMessage {
	s.logger.Debug("received BlockAnnounceMessage")

	// create header from message
	header, err := types.NewHeader(
		msg.ParentHash,
		msg.Number,
		msg.StateRoot,
		msg.ExtrinsicsRoot,
		msg.Digest,
	)
	if err != nil {
		s.logger.Error("failed to handle BlockAnnounce", "error", err)
		return nil
	}

	// check if block header is stored in block state
	has, err := s.blockState.HasHeader(header.Hash())
	if err != nil {
		s.logger.Error("failed to handle BlockAnnounce", "error", err)
	}

	// save block header if we don't have it already
	if !has {
		err = s.blockState.SetHeader(header)
		if err != nil {
			s.logger.Error("failed to handle BlockAnnounce", "error", err)
		}
		s.logger.Debug(
			"saved block header to block state",
			"number", header.Number,
			"hash", header.Hash(),
		)
	}

	// check if block body is stored in block state (ie. if we have the full block already)
	_, err = s.blockState.GetBlockBody(header.Hash())
	if err != nil && err.Error() == "Key not found" {
		s.logger.Debug(
			"sending block number to syncer",
			"number", msg.Number,
		)

		// create block request to send
		s.requestStart = header.Number.Int64()
		return s.createBlockRequest()
	} else if err != nil {
		s.logger.Error("failed to handle BlockAnnounce", "error", err)
	}

	return nil
}

// HandleBlockResponse handles a BlockResponseMessage by processing the blocks found in it and adding them to the BlockState if necessary.
// If the node is still not synced after processing, it creates and returns the next BlockRequestMessage to send.
func (s *Service) HandleBlockResponse(msg *network.BlockResponseMessage) *network.BlockRequestMessage {
	// highestInResp will be the highest block in the response
	// it's set to 0 if err != nil
	highestInResp, err := s.processBlockResponseData(msg)

	// if we cannot find the parent block in our blocktree, we are missing some blocks, and need to request
	// blocks from farther back in the chain
	if err == blocktree.ErrParentNotFound {
		// set request start
		s.requestStart = s.requestStart - maxResponseSize
		if s.requestStart <= 0 {
			s.requestStart = 1
		}
		s.logger.Trace("Retrying block request", "start", s.requestStart)
		return s.createBlockRequest()
	} else if err != nil {
		s.logger.Error("failed to process block response", "error", err)
		return nil
	}

	// TODO: max retries before unlocking BlockProducer, in case no response is received
	bestNum, err := s.blockState.BestBlockNumber()
	if err != nil {
		s.logger.Error("failed to get best block number", "error", err)
		bestNum = big.NewInt(0)
	}

	// check if we are synced or not
	if bestNum.Cmp(s.highestSeenBlock) >= 0 && bestNum.Cmp(big.NewInt(0)) != 0 {
		s.logger.Debug("all synced up!", "number", bestNum)
		s.benchmarker.end(uint64(bestNum.Int64()))

		if !s.synced {
			err = s.blockProducer.Resume()
			if err != nil {
				s.logger.Warn("failed to resume block production")
			}
			s.synced = true
		}
		return nil
	}

	// not yet synced, send another block request for the following blocks
	s.requestStart = highestInResp + 1
	return s.createBlockRequest()
}

func (s *Service) createBlockRequest() *network.BlockRequestMessage {
	// generate random ID
	s1 := rand.NewSource(uint64(time.Now().UnixNano()))
	seed := rand.New(s1).Uint64()
	randomID := mrand.New(mrand.NewSource(int64(seed))).Uint64()

	start, err := variadic.NewUint64OrHash(uint64(s.requestStart))
	if err != nil {
		s.logger.Error("failed to create block request start block", "error", err)
		return nil
	}

	s.logger.Trace("sending block request", "start", start)

	blockRequest := &network.BlockRequestMessage{
		ID:            randomID, // random
		RequestedData: 3,        // block header + body
		StartingBlock: start,
		EndBlockHash:  optional.NewHash(false, common.Hash{}),
		Direction:     1,
		Max:           optional.NewUint32(false, 0),
	}

	return blockRequest
}

func (s *Service) processBlockResponseData(msg *network.BlockResponseMessage) (int64, error) {
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
func (s *Service) handleHeader(header *types.Header) (int64, error) {
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

		s.logger.Info("saved block header", "hash", header.Hash(), "number", header.Number)
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
func (s *Service) handleBody(body *types.Body) error {
	exts, err := body.AsExtrinsics()
	if err != nil {
		s.logger.Error("cannot parse body as extrinsics", "error", err)
		return err
	}

	for _, ext := range exts {
		s.transactionQueue.RemoveExtrinsic(ext)
	}

	return err
}

// handleHeader handles blocks (header+body) included in BlockResponses
func (s *Service) handleBlock(block *types.Block) error {
	// TODO: needs to be fixed by #941
	// _, err := s.executeBlock(block)
	// if err != nil {
	// 	return err
	// }

	err := s.blockState.AddBlock(block)
	if err != nil {
		if err == blocktree.ErrParentNotFound && block.Header.Number.Cmp(big.NewInt(0)) != 0 {
			return err
		} else if err == blocktree.ErrBlockExists || block.Header.Number.Cmp(big.NewInt(0)) == 0 {
			// this is fine
		} else {
			return err
		}
	} else {
		s.logger.Info("imported block", "number", block.Header.Number, "hash", block.Header.Hash())
		s.logger.Debug("imported block", "header", block.Header, "body", block.Body)
	}

	// TODO: if block is from the next epoch, increment epoch

	// handle consensus digest for authority changes
	if s.digestHandler != nil {
		err = s.handleDigests(block.Header)
		if err != nil {
			return err
		}
	}

	return nil
}

// runs the block through runtime function Core_execute_block
//  It doesn't seem to return data on success (although the spec say it should return
//  a boolean value that indicate success.  will error if the call isn't successful
func (s *Service) executeBlock(block *types.Block) ([]byte, error) {
	// copy block since we're going to modify it
	b := block.DeepCopy()

	b.Header.Digest = [][]byte{}
	bdEnc, err := b.Encode()
	if err != nil {
		return nil, err
	}

	return s.runtime.Exec(runtime.CoreExecuteBlock, bdEnc)
}

func (s *Service) executeBlockBytes(bd []byte) ([]byte, error) {
	return s.runtime.Exec(runtime.CoreExecuteBlock, bd)
}

func (s *Service) handleDigests(header *types.Header) error {
	for _, d := range header.Digest {
		dg, err := types.DecodeDigestItem(d)
		if err != nil {
			return err
		}

		if dg.Type() == types.ConsensusDigestType {
			cd, ok := dg.(*types.ConsensusDigest)
			if !ok {
				return errors.New("cannot cast invalid consensus digest item")
			}

			err = s.digestHandler.HandleConsensusDigest(cd)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
