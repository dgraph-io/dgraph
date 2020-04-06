package core

import (
	"math/big"
	mrand "math/rand"
	"sync"
	"time"

	"golang.org/x/exp/rand"

	"github.com/ChainSafe/gossamer/dot/core/types"
	"github.com/ChainSafe/gossamer/dot/network"
	"github.com/ChainSafe/gossamer/lib/blocktree"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/common/optional"
	"github.com/ChainSafe/gossamer/lib/common/variadic"

	log "github.com/ChainSafe/log15"
)

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

	// Core service control
	chanLock *sync.Mutex
	stopped  bool
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

	return &Syncer{
		blockState:       cfg.BlockState,
		blockNumIn:       cfg.BlockNumIn,
		respIn:           cfg.RespIn,
		msgOut:           cfg.MsgOut,
		lock:             cfg.Lock,
		chanLock:         cfg.ChanLock,
		synced:           true,
		stopped:          false,
		requestStart:     1,
		highestSeenBlock: big.NewInt(0),
		transactionQueue: cfg.TransactionQueue,
	}, nil
}

// Start begins the syncer
func (s *Syncer) Start() {
	go s.watchForBlocks()
	go s.watchForResponses()
}

// Stop stops the syncer
func (s *Syncer) Stop() {
	// stop goroutines
	s.stopped = true
}

func (s *Syncer) watchForBlocks() {
	for {
		if s.stopped {
			return
		}

		blockNum, ok := <-s.blockNumIn
		if !ok || blockNum == nil {
			log.Warn("[sync] Failed to receive from blockNumIn channel")
			return
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
		if s.stopped {
			return
		}

		var msg *network.BlockResponseMessage
		var ok bool

		select {
		case msg, ok = <-s.respIn:
			// handle response
			if !ok || msg == nil {
				log.Warn("[sync] Failed to receive from respIn channel")
				return
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
			log.Debug("[sync] Retrying block request", "start", s.requestStart)
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
	if s.stopped {
		return ErrServiceStopped
	}
	s.msgOut <- msg
	return nil
}

func (s *Syncer) sendBlockRequest() {
	//generate random ID
	s1 := rand.NewSource(uint64(time.Now().UnixNano()))
	seed := rand.New(s1).Uint64()
	randomID := mrand.New(mrand.NewSource(int64(seed))).Uint64()

	start, err := variadic.NewUint64OrHash(uint64(s.requestStart))
	if err != nil {
		log.Error("[sync] Failed to create StartingBlock", "error", err)
		return
	}

	log.Debug("[sync] Block request", "start", start)

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
	// TODO: update blockState to include Has function
	existingHeader, err := s.blockState.GetHeader(header.Hash())
	if err != nil && existingHeader == nil {
		err = s.blockState.SetHeader(header)
		if err != nil {
			return 0, err
		}

		log.Info("[sync] saved block header", "hash", header.Hash(), "number", header.Number)

		// TODO: handle consensus digest, if first in epoch
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
	// TODO: execute block and verify authorship right

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
		log.Info("[sync] imported block", "number", block.Header.Number, "hash", block.Header.Hash())
	}

	return nil
}
