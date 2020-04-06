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
	"bytes"
	"math/big"
	"sync"

	"github.com/ChainSafe/gossamer/dot/core/types"
	"github.com/ChainSafe/gossamer/dot/network"
	"github.com/ChainSafe/gossamer/lib/babe"
	"github.com/ChainSafe/gossamer/lib/blocktree"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/common/optional"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
	"github.com/ChainSafe/gossamer/lib/database"
	"github.com/ChainSafe/gossamer/lib/keystore"
	"github.com/ChainSafe/gossamer/lib/runtime"
	"github.com/ChainSafe/gossamer/lib/services"
	"github.com/ChainSafe/gossamer/lib/transaction"

	log "github.com/ChainSafe/log15"
)

var _ services.Service = &Service{}

var maxResponseSize int64 = 8 // maximum number of block datas to reply with in a BlockResponse message.

// Service is an overhead layer that allows communication between the runtime,
// BABE session, and network service. It deals with the validation of transactions
// and blocks by calling their respective validation functions in the runtime.
type Service struct {
	// State interfaces
	blockState       BlockState
	storageState     StorageState
	transactionQueue TransactionQueue

	// Current runtime and hash of the current runtime code
	rt       *runtime.Runtime
	codeHash common.Hash

	// Current BABE session
	bs              *babe.Session
	isBabeAuthority bool

	// Keystore
	keys *keystore.Keystore

	// Channels for inter-process communication
	msgRec    <-chan network.Message // receive messages from network service
	msgSend   chan<- network.Message // send messages to network service
	blkRec    <-chan types.Block     // receive blocks from BABE session
	epochDone <-chan struct{}        // receive from this channel when BABE epoch changes
	babeKill  chan<- struct{}        // close this channel to kill current BABE session
	lock      *sync.Mutex
	closed    bool

	// Block synchronization
	blockNumOut chan<- *big.Int                      // send block numbers from peers to Syncer
	respOut     chan<- *network.BlockResponseMessage // send incoming BlockResponseMessags to Syncer
	syncLock    *sync.Mutex
	syncer      *Syncer
}

// Config holds the configuration for the core Service.
type Config struct {
	BlockState       BlockState
	StorageState     StorageState
	TransactionQueue TransactionQueue
	Keystore         *keystore.Keystore
	Runtime          *runtime.Runtime
	IsBabeAuthority  bool

	NewBlocks chan types.Block // only used for testing purposes
	MsgRec    <-chan network.Message
	MsgSend   chan<- network.Message
	SyncChan  chan *big.Int
}

// NewService returns a new core service that connects the runtime, BABE
// session, and network service.
func NewService(cfg *Config) (*Service, error) {
	if cfg.Keystore == nil {
		return nil, ErrNilKeystore
	}

	keys := cfg.Keystore.Sr25519Keypairs()

	if cfg.NewBlocks == nil {
		cfg.NewBlocks = make(chan types.Block)
	}

	if cfg.BlockState == nil {
		return nil, ErrNilBlockState
	}

	if cfg.StorageState == nil {
		return nil, ErrNilStorageState
	}

	codeHash, err := cfg.StorageState.LoadCodeHash()
	if err != nil {
		return nil, err
	}

	syncerLock := &sync.Mutex{}
	respChan := make(chan *network.BlockResponseMessage, 128)
	chanLock := &sync.Mutex{}

	syncerCfg := &SyncerConfig{
		BlockState:       cfg.BlockState,
		BlockNumIn:       cfg.SyncChan,
		RespIn:           respChan,
		MsgOut:           cfg.MsgSend,
		Lock:             syncerLock,
		ChanLock:         chanLock,
		TransactionQueue: cfg.TransactionQueue,
	}

	syncer, err := NewSyncer(syncerCfg)
	if err != nil {
		return nil, err
	}

	var srv = &Service{}

	if cfg.IsBabeAuthority {
		if cfg.Keystore.NumSr25519Keys() == 0 {
			return nil, ErrNoKeysProvided
		}

		epochDone := make(chan struct{})
		babeKill := make(chan struct{})

		srv = &Service{
			rt:               cfg.Runtime,
			codeHash:         codeHash,
			keys:             cfg.Keystore,
			blkRec:           cfg.NewBlocks, // becomes block receive channel in core service
			msgRec:           cfg.MsgRec,
			msgSend:          cfg.MsgSend,
			blockState:       cfg.BlockState,
			storageState:     cfg.StorageState,
			transactionQueue: cfg.TransactionQueue,
			epochDone:        epochDone,
			babeKill:         babeKill,
			isBabeAuthority:  true,
			lock:             chanLock,
			closed:           false,
			syncer:           syncer,
			syncLock:         syncerLock,
			blockNumOut:      cfg.SyncChan,
			respOut:          respChan,
		}

		authData, err := srv.retrieveAuthorityData()
		if err != nil {
			return nil, err
		}

		// BABE session configuration
		bsConfig := &babe.SessionConfig{
			Keypair:          keys[0].(*sr25519.Keypair),
			Runtime:          cfg.Runtime,
			NewBlocks:        cfg.NewBlocks, // becomes block send channel in BABE session
			BlockState:       cfg.BlockState,
			StorageState:     cfg.StorageState,
			AuthData:         authData,
			Done:             epochDone,
			Kill:             babeKill,
			TransactionQueue: cfg.TransactionQueue,
			SyncLock:         syncerLock,
		}

		// create a new BABE session
		bs, err := babe.NewSession(bsConfig)
		if err != nil {
			srv.isBabeAuthority = false
			log.Error("[core] could not start babe session", "error", err)
			return srv, nil
		}

		srv.bs = bs
	} else {
		srv = &Service{
			rt:               cfg.Runtime,
			codeHash:         codeHash,
			keys:             cfg.Keystore,
			blkRec:           cfg.NewBlocks, // becomes block receive channel in core service
			msgRec:           cfg.MsgRec,
			msgSend:          cfg.MsgSend,
			blockState:       cfg.BlockState,
			storageState:     cfg.StorageState,
			transactionQueue: cfg.TransactionQueue,
			isBabeAuthority:  false,
			lock:             chanLock,
			closed:           false,
			syncer:           syncer,
			syncLock:         syncerLock,
			blockNumOut:      cfg.SyncChan,
			respOut:          respChan,
		}
	}

	// core service
	return srv, nil
}

// Start starts the core service
func (s *Service) Start() error {

	// start receiving blocks from BABE session
	go s.receiveBlocks()

	// start receiving messages from network service
	go s.receiveMessages()

	// start syncer
	s.syncer.Start()

	if s.isBabeAuthority {
		// monitor babe session for epoch changes
		go s.handleBabeSession()

		err := s.bs.Start()
		if err != nil {
			log.Error("[core] could not start BABE", "error", err)
			return err
		}
	}

	return nil
}

// Stop stops the core service
func (s *Service) Stop() error {

	s.lock.Lock()
	defer s.lock.Unlock()

	// close channel to network service and BABE service
	if !s.closed {
		if s.msgSend != nil {
			close(s.msgSend)
		}
		if s.isBabeAuthority {
			close(s.babeKill)
		}
		s.closed = true
	}

	s.syncer.Stop()

	return nil
}

// StorageRoot returns the hash of the runtime storage root
func (s *Service) StorageRoot() (common.Hash, error) {
	if s.storageState == nil {
		return common.Hash{}, ErrNilStorageState
	}
	return s.storageState.StorageRoot()
}

func (s *Service) retrieveAuthorityData() ([]*babe.AuthorityData, error) {
	// TODO: when we update to a new runtime, will need to pass in the latest block number
	return s.grandpaAuthorities()
}

// getLatestSlot returns the slot for the block at the head of the chain
func (s *Service) getLatestSlot() (uint64, error) {
	return s.blockState.GetSlotForBlock(s.blockState.HighestBlockHash())
}

func (s *Service) safeMsgSend(msg network.Message) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.closed {
		return ErrServiceStopped
	}
	s.msgSend <- msg
	return nil
}

func (s *Service) safeBabeKill() error {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.closed {
		return ErrServiceStopped
	}
	close(s.babeKill)
	return nil
}

func (s *Service) handleBabeSession() {
	for {
		<-s.epochDone
		log.Debug("[core] BABE epoch complete, initializing new session")

		// commit the storage trie to the DB
		err := s.storageState.StoreInDB()
		if err != nil {
			log.Error("[core]", "error", err)
		}

		newBlocks := make(chan types.Block)
		s.blkRec = newBlocks

		epochDone := make(chan struct{})
		s.epochDone = epochDone

		babeKill := make(chan struct{})
		s.babeKill = babeKill

		keys := s.keys.Sr25519Keypairs()

		latestSlot, err := s.getLatestSlot()
		if err != nil {
			log.Error("[core]", "error", err)
		}

		// BABE session configuration
		bsConfig := &babe.SessionConfig{
			Keypair:          keys[0].(*sr25519.Keypair),
			Runtime:          s.rt,
			NewBlocks:        newBlocks, // becomes block send channel in BABE session
			BlockState:       s.blockState,
			StorageState:     s.storageState,
			TransactionQueue: s.transactionQueue,
			AuthData:         s.bs.AuthorityData(), // AuthorityData will be updated when the NextEpochDescriptor arrives.
			Done:             epochDone,
			Kill:             babeKill,
			StartSlot:        latestSlot + 1,
			SyncLock:         s.syncLock,
		}

		// create a new BABE session
		bs, err := babe.NewSession(bsConfig)
		if err != nil {
			log.Error("[core] could not initialize BABE", "error", err)
			return
		}

		err = bs.Start()
		if err != nil {
			log.Error("[core] could not start BABE", "error", err)
		}

		s.bs = bs
		log.Trace("[core] BABE session initialized and started")
	}
}

// receiveBlocks starts receiving blocks from the BABE session
func (s *Service) receiveBlocks() {
	for {
		// receive block from BABE session
		block, ok := <-s.blkRec
		if ok {
			err := s.handleReceivedBlock(&block)
			if err != nil {
				log.Error("[core] failed to handle block from BABE session", "err", err)
			}
		}
	}
}

// receiveMessages starts receiving messages from the network service
func (s *Service) receiveMessages() {
	for {
		// receive message from network service
		msg, ok := <-s.msgRec
		if !ok {
			log.Error("[core] failed to receive message from network service")
			return // exit
		}

		err := s.handleReceivedMessage(msg)
		if err == blocktree.ErrDescendantNotFound || err == blocktree.ErrStartNodeNotFound || err == database.ErrKeyNotFound {
			log.Trace("[core] failed to handle message from network service", "err", err)
		} else if err != nil {
			log.Error("[core] failed to handle message from network service", "err", err)
		}
	}
}

// handleReceivedBlock handles blocks from the BABE session
func (s *Service) handleReceivedBlock(block *types.Block) (err error) {
	if s.blockState == nil {
		return ErrNilBlockState
	}

	err = s.blockState.AddBlock(block)
	if err != nil {
		return err
	}

	msg := &network.BlockAnnounceMessage{
		ParentHash:     block.Header.ParentHash,
		Number:         block.Header.Number,
		StateRoot:      block.Header.StateRoot,
		ExtrinsicsRoot: block.Header.ExtrinsicsRoot,
		Digest:         block.Header.Digest,
	}

	err = s.safeMsgSend(msg)
	if err != nil {
		return err
	}

	// TODO: check if host status message needs to be updated based on new block
	// information, if so, generate host status message and send to network service

	// TODO: send updated host status message to network service
	// s.msgSend <- msg

	err = s.checkForRuntimeChanges()
	if err != nil {
		return err
	}

	return nil
}

// handleReceivedMessage handles messages from the network service
func (s *Service) handleReceivedMessage(msg network.Message) (err error) {
	msgType := msg.GetType()

	switch msgType {
	case network.BlockAnnounceMsgType:
		blockAnnounceMessage, ok := msg.(*network.BlockAnnounceMessage)
		if !ok {
			return ErrMessageCast("BlockAnnounceMessage")
		}

		err = s.ProcessBlockAnnounceMessage(blockAnnounceMessage)
	case network.BlockRequestMsgType:
		msg, ok := msg.(*network.BlockRequestMessage)
		if !ok {
			return ErrMessageCast("BlockRequestMessage")
		}

		err = s.ProcessBlockRequestMessage(msg)
	case network.BlockResponseMsgType:
		msg, ok := msg.(*network.BlockResponseMessage)
		if !ok {
			return ErrMessageCast("BlockResponseMessage")
		}

		err = s.ProcessBlockResponseMessage(msg)
	case network.TransactionMsgType:
		msg, ok := msg.(*network.TransactionMessage)
		if !ok {
			return ErrMessageCast("TransactionMessage")
		}

		err = s.ProcessTransactionMessage(msg)
	default:
		err = ErrUnsupportedMsgType(msgType)
	}

	return err
}

// ProcessBlockAnnounceMessage creates a block request message from the block
// announce messages (block announce messages include the header but the full
// block is required to execute `core_execute_block`).
func (s *Service) ProcessBlockAnnounceMessage(msg *network.BlockAnnounceMessage) error {
	log.Debug("[core] got BlockAnnounceMessage")

	header, err := types.NewHeader(msg.ParentHash, msg.Number, msg.StateRoot, msg.ExtrinsicsRoot, msg.Digest)
	if err != nil {
		return err
	}

	_, err = s.blockState.GetHeader(header.Hash())
	if err != nil && err == database.ErrKeyNotFound {
		err = s.blockState.SetHeader(header)
		if err != nil {
			return err
		}

		log.Info("[core] saved block", "number", header.Number, "hash", header.Hash())
	} else if err != nil {
		return err
	}

	_, err = s.blockState.GetBlockBody(header.Hash())
	if err != nil && err == database.ErrKeyNotFound {
		// send block request message
		log.Debug("[core] sending new block to syncer", "number", msg.Number)
		s.blockNumOut <- msg.Number
	} else if err != nil {
		return err
	}

	return nil
}

// ProcessBlockRequestMessage processes a block request message, returning a block response message
func (s *Service) ProcessBlockRequestMessage(msg *network.BlockRequestMessage) error {
	blockResponse, err := s.createBlockResponse(msg)
	if err != nil {
		return err
	}

	return s.safeMsgSend(blockResponse)
}

func (s *Service) createBlockResponse(blockRequest *network.BlockRequestMessage) (*network.BlockResponseMessage, error) {
	var startHash common.Hash
	var endHash common.Hash

	switch c := blockRequest.StartingBlock.Value().(type) {
	case uint64:
		if c == 0 {
			c = 1
		}

		block, err := s.blockState.GetBlockByNumber(big.NewInt(0).SetUint64(c))
		if err != nil {
			return nil, err
		}

		startHash = block.Header.Hash()
	case common.Hash:
		startHash = c
	}

	if blockRequest.EndBlockHash.Exists() {
		endHash = blockRequest.EndBlockHash.Value()
	} else {
		endHash = s.blockState.BestBlockHash()
	}

	log.Debug("[core] got BlockRequestMessage", "startHash", startHash, "endHash", endHash)

	// get sub-chain of block hashes
	subchain, err := s.blockState.SubChain(startHash, endHash)
	if err != nil {
		return nil, err
	}

	if len(subchain) > int(maxResponseSize) {
		subchain = subchain[:maxResponseSize]
	}

	log.Trace("[core] subchain", "start", subchain[0], "end", subchain[len(subchain)-1])

	responseData := []*types.BlockData{}

	for _, hash := range subchain {

		blockData := new(types.BlockData)
		blockData.Hash = hash

		// set defaults
		blockData.Header = optional.NewHeader(false, nil)
		blockData.Body = optional.NewBody(false, nil)
		blockData.Receipt = optional.NewBytes(false, nil)
		blockData.MessageQueue = optional.NewBytes(false, nil)
		blockData.Justification = optional.NewBytes(false, nil)

		// header
		if (blockRequest.RequestedData & 1) == 1 {
			retData, err := s.blockState.GetHeader(hash)
			if err == nil && retData != nil {
				blockData.Header = retData.AsOptional()
			}
		}
		// body
		if (blockRequest.RequestedData&2)>>1 == 1 {
			retData, err := s.blockState.GetBlockBody(hash)
			if err == nil && retData != nil {
				blockData.Body = retData.AsOptional()
			}
		}
		// receipt
		if (blockRequest.RequestedData&4)>>2 == 1 {
			retData, err := s.blockState.GetReceipt(hash)
			if err == nil && retData != nil {
				blockData.Receipt = optional.NewBytes(true, retData)
			}
		}
		// message queue
		if (blockRequest.RequestedData&8)>>3 == 1 {
			retData, err := s.blockState.GetMessageQueue(hash)
			if err == nil && retData != nil {
				blockData.MessageQueue = optional.NewBytes(true, retData)
			}
		}
		// justification
		if (blockRequest.RequestedData&16)>>4 == 1 {
			retData, err := s.blockState.GetJustification(hash)
			if err == nil && retData != nil {
				blockData.Justification = optional.NewBytes(true, retData)
			}
		}

		responseData = append(responseData, blockData)
	}

	return &network.BlockResponseMessage{
		ID:        blockRequest.ID,
		BlockData: responseData,
	}, nil
}

// ProcessBlockResponseMessage attempts to validate and add the block to the
// chain by calling `core_execute_block`. Valid blocks are stored in the block
// database to become part of the canonical chain.
func (s *Service) ProcessBlockResponseMessage(msg *network.BlockResponseMessage) error {
	log.Debug("[core] received BlockResponseMessage")
	s.respOut <- msg
	return s.checkForRuntimeChanges()
}

// checkForRuntimeChanges checks if changes to the runtime code have occurred; if so, load the new runtime
func (s *Service) checkForRuntimeChanges() error {
	currentCodeHash, err := s.storageState.LoadCodeHash()
	if err != nil {
		return err
	}

	if !bytes.Equal(currentCodeHash[:], s.codeHash[:]) {
		code, err := s.storageState.LoadCode()
		if err != nil {
			return err
		}

		s.rt.Stop()

		s.rt, err = runtime.NewRuntime(code, s.storageState, s.keys)
		if err != nil {
			return err
		}

		// kill babe session, handleBabeSession will reload it with the new runtime
		if s.isBabeAuthority {
			err = s.safeBabeKill()
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// ProcessTransactionMessage validates each transaction in the message and
// adds valid transactions to the transaction queue of the BABE session
func (s *Service) ProcessTransactionMessage(msg *network.TransactionMessage) error {
	// get transactions from message extrinsics
	txs := msg.Extrinsics

	for _, tx := range txs {
		tx := tx // pin

		// validate each transaction
		val, err := s.ValidateTransaction(tx)
		if err != nil {
			log.Error("[core] failed to validate transaction", "err", err)
			return err // exit
		}

		// create new valid transaction
		vtx := transaction.NewValidTransaction(tx, val)

		if s.isBabeAuthority {
			// push to the transaction queue of BABE session
			hash, err := s.transactionQueue.Push(vtx)
			if err != nil {
				log.Trace("[core] Failed to push transaction to queue", "error", err)
			} else {
				log.Trace("[core] Added transaction to queue", "hash", hash)
			}
		}
	}

	return nil
}

// handle authority and randomness changes over transitions from one epoch to the next
//nolint
func (s *Service) handleConsensusDigest(header *types.Header) (err error) {
	var item types.DigestItem
	for _, digest := range header.Digest {
		item, err = types.DecodeDigestItem(digest)
		if err != nil {
			return err
		}

		if item.Type() == types.ConsensusDigestType {
			break
		}
	}

	// TODO: if this block is the first in the epoch and it doesn't have a consensus digest, this is an error
	if item == nil {
		return nil
	}

	consensusDigest := item.(*types.ConsensusDigest)

	epochData := new(babe.NextEpochDescriptor)
	err = epochData.Decode(consensusDigest.Data)
	if err != nil {
		return err
	}

	if s.isBabeAuthority {
		// TODO: if this block isn't the first in the epoch, and it has a consensus digest, this is an error
		err = s.bs.SetEpochData(epochData)
		if err != nil {
			return err
		}
	}

	return nil
}
