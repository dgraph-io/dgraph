package core

import (
	"encoding/binary"
	"errors"
	"math/big"
	mrand "math/rand"
	"sync"
	"time"

	"golang.org/x/exp/rand"

	"github.com/ChainSafe/gossamer/dot/network"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/common/optional"
	"github.com/ChainSafe/gossamer/lib/common/variadic"

	log "github.com/ChainSafe/log15"
)

// Syncer deals with chain syncing by sending block request messages and watching for responses.
type Syncer struct {
	blockState    BlockState             // retrieve our current head of chain from BlockState
	blockNumberIn <-chan *big.Int        // incoming block numbers seen from other nodes that are higher than ours
	msgOut        chan<- network.Message // channel to send message to network service
	lock          *sync.Mutex
	synced        bool
}

// SyncerConfig is the configuration for the Syncer.
type SyncerConfig struct {
	BlockState    BlockState
	BlockNumberIn <-chan *big.Int
	MsgOut        chan<- network.Message
	Lock          *sync.Mutex
}

// NewSyncer returns a new Syncer
func NewSyncer(cfg *SyncerConfig) (*Syncer, error) {
	if cfg.BlockState == nil {
		return nil, errors.New("cannot have nil BlockState")
	}

	if cfg.BlockNumberIn == nil {
		return nil, errors.New("cannot have nil BlockNumberIn channel")
	}

	if cfg.MsgOut == nil {
		return nil, errors.New("cannot have nil MsgOut channel")
	}

	return &Syncer{
		blockState:    cfg.BlockState,
		blockNumberIn: cfg.BlockNumberIn,
		msgOut:        cfg.MsgOut,
		lock:          cfg.Lock,
		synced:        true,
	}, nil
}

// Start begins the syncer
func (s *Syncer) Start() {
	go s.watchForBlocks()
}

func (s *Syncer) watchForBlocks() {
	for {
		blockNum := <-s.blockNumberIn
		if blockNum != nil {
			if s.synced {
				s.synced = false
				s.lock.Lock()
			}

			err := s.sendBlockRequest()
			if err != nil {
				log.Error("[sync] Failed to send block request", "error", err)
			}

			go s.watchForResponses(blockNum)
		}
	}
}

func (s *Syncer) watchForResponses(blockNum *big.Int) {
	for {
		bestNum, err := s.blockState.BestBlockNumber()
		if err != nil {
			log.Error("[sync] Failed to get best block number", "error", err)

			if !s.synced {
				s.lock.Unlock()
			}

			return
		}

		if bestNum.Cmp(blockNum) == 0 && bestNum.Cmp(big.NewInt(0)) != 0 {
			log.Debug("[sync] All synced up!", "number", bestNum)

			if !s.synced {
				s.lock.Unlock()
			}

			s.synced = true
			return
		}

		time.Sleep(time.Second)
	}
}

func (s *Syncer) sendBlockRequest() error {
	bestNum, err := s.blockState.BestBlockNumber()
	if err != nil {
		log.Error("[sync] Failed to get best block number", "error", err)
		return err
	}

	//generate random ID
	s1 := rand.NewSource(uint64(time.Now().UnixNano()))
	seed := rand.New(s1).Uint64()
	randomID := mrand.New(mrand.NewSource(int64(seed))).Uint64()

	// TODO: can't request from /our/ best block number, need to start requesting from the best block num we have of /theirs/
	// otherwise there's a chance we might build a block, then miss a block of theirs, causing error="cannot find parent block in blocktree"
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(bestNum.Int64()))

	blockRequest := &network.BlockRequestMessage{
		ID:            randomID, // random
		RequestedData: 3,        // block header + body
		StartingBlock: variadic.NewUint64OrHash(append([]byte{1}, buf...)),
		EndBlockHash:  optional.NewHash(false, common.Hash{}),
		Direction:     1,
		Max:           optional.NewUint32(false, 0),
	}

	// send block request message to network service
	s.msgOut <- blockRequest

	return nil
}
