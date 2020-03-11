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
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	mrand "math/rand"
	"sync"
	"time"

	"golang.org/x/exp/rand"

	"github.com/ChainSafe/gossamer/dot/core/types"
	"github.com/ChainSafe/gossamer/dot/network"
	"github.com/ChainSafe/gossamer/lib/babe"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/common/optional"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
	"github.com/ChainSafe/gossamer/lib/keystore"
	"github.com/ChainSafe/gossamer/lib/runtime"
	"github.com/ChainSafe/gossamer/lib/services"
	"github.com/ChainSafe/gossamer/lib/transaction"

	log "github.com/ChainSafe/log15"
)

var _ services.Service = &Service{}

var maxResponseSize = 8 // maximum number of block datas to reply with in a BlockResponse message.

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
	lock      sync.Mutex
	closed    bool

	// TODO: add to network state
	requestedBlockIDs map[uint64]bool // track requested block id messages
}

// Config holds the configuration for the core Service.
type Config struct {
	BlockState       BlockState
	StorageState     StorageState
	TransactionQueue TransactionQueue
	Keystore         *keystore.Keystore
	Runtime          *runtime.Runtime
	MsgRec           <-chan network.Message
	MsgSend          chan<- network.Message
	NewBlocks        chan types.Block // only used for testing purposes
	IsBabeAuthority  bool
}

// NewService returns a new core service that connects the runtime, BABE
// session, and network service.
func NewService(cfg *Config) (*Service, error) {
	if cfg.Keystore == nil {
		return nil, fmt.Errorf("no keystore provided")
	}

	keys := cfg.Keystore.Sr25519Keypairs()

	if cfg.NewBlocks == nil {
		cfg.NewBlocks = make(chan types.Block)
	}

	if cfg.BlockState == nil {
		return nil, fmt.Errorf("block state is nil")
	}

	if cfg.StorageState == nil {
		return nil, fmt.Errorf("storage state is nil")
	}

	codeHash, err := cfg.StorageState.LoadCodeHash()
	if err != nil {
		return nil, err
	}

	var srv = &Service{}

	if cfg.IsBabeAuthority {
		if cfg.Keystore.NumSr25519Keys() == 0 {
			return nil, fmt.Errorf("no keys provided for authority node")
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
			closed:           false,
		}

		authData, err := srv.retrieveAuthorityData()
		if err != nil {
			return nil, fmt.Errorf("could not retrieve authority data: %s", err)
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
			closed:           false,
		}
	}

	srv.requestedBlockIDs = make(map[uint64]bool)

	// core service
	return srv, nil
}

// Start starts the core service
func (s *Service) Start() error {

	// start receiving blocks from BABE session
	go s.receiveBlocks()

	// start receiving messages from network service
	go s.receiveMessages()

	if s.isBabeAuthority {
		// monitor babe session for epoch changes
		go s.handleBabeSession()

		err := s.bs.Start()
		if err != nil {
			log.Error("[core] could not start BABE", "error", err)
		}

		return err
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

	return nil
}

// StorageRoot returns the hash of the runtime storage root
func (s *Service) StorageRoot() (common.Hash, error) {
	if s.storageState == nil {
		return common.Hash{}, fmt.Errorf("storage state is nil")
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
		return errors.New("service has been stopped")
	}
	s.msgSend <- msg
	return nil
}

func (s *Service) safeBabeKill() error {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.closed {
		return errors.New("service has been stopped")
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
		if err != nil {
			log.Error("[core] failed to handle message from network service", "err", err)
		}
	}
}

// handleReceivedBlock handles blocks from the BABE session
func (s *Service) handleReceivedBlock(block *types.Block) (err error) {
	if s.blockState == nil {
		return fmt.Errorf("blockState is nil")
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
		err = s.ProcessBlockAnnounceMessage(msg)
	case network.BlockRequestMsgType:
		err = s.ProcessBlockRequestMessage(msg)
	case network.BlockResponseMsgType:
		err = s.ProcessBlockResponseMessage(msg)
	case network.TransactionMsgType:
		err = s.ProcessTransactionMessage(msg)
	default:
		err = fmt.Errorf("Received unsupported message type %d", msgType)
	}

	return err
}

// ProcessBlockAnnounceMessage creates a block request message from the block
// announce messages (block announce messages include the header but the full
// block is required to execute `core_execute_block`).
func (s *Service) ProcessBlockAnnounceMessage(msg network.Message) error {
	blockAnnounceMessage, ok := msg.(*network.BlockAnnounceMessage)
	if !ok {
		return errors.New("could not cast network.Message to BlockAnnounceMessage")
	}

	header, err := types.NewHeader(blockAnnounceMessage.ParentHash, blockAnnounceMessage.Number, blockAnnounceMessage.StateRoot, blockAnnounceMessage.ExtrinsicsRoot, blockAnnounceMessage.Digest)
	if err != nil {
		return err
	}

	_, err = s.blockState.GetHeader(header.Hash())
	if err != nil && err.Error() == "Key not found" {
		err = s.blockState.SetHeader(header)
		if err != nil {
			return err
		}

		log.Info("[core] saved block", "number", header.Number, "hash", header.Hash())

	} else {
		return err
	}

	bestNum, err := s.blockState.BestBlockNumber()
	if err != nil {
		return err
	}

	messageBlockNumMinusOne := big.NewInt(0).Sub(blockAnnounceMessage.Number, big.NewInt(1))

	// check if we should send block request message
	if bestNum.Cmp(messageBlockNumMinusOne) == -1 {

		//generate random ID
		s1 := rand.NewSource(uint64(time.Now().UnixNano()))
		seed := rand.New(s1).Uint64()
		randomID := mrand.New(mrand.NewSource(int64(seed))).Uint64()

		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, uint64(bestNum.Int64()))

		blockRequest := &network.BlockRequestMessage{
			ID:            randomID, // random
			RequestedData: 3,        // block header + body
			StartingBlock: append([]byte{1}, buf...),
			EndBlockHash:  optional.NewHash(true, header.Hash()),
			Direction:     1,
			Max:           optional.NewUint32(false, 0),
		}

		//track request
		s.requestedBlockIDs[randomID] = true

		// send block request message to network service
		log.Debug("send blockRequest message to network service")

		err = s.safeMsgSend(blockRequest)
		if err != nil {
			return err
		}
	}

	return nil
}

// ProcessBlockRequestMessage processes a block request message, returning a block response message
func (s *Service) ProcessBlockRequestMessage(msg network.Message) error {
	blockRequest := msg.(*network.BlockRequestMessage)

	startPrefix := blockRequest.StartingBlock[0]
	startData := blockRequest.StartingBlock[1:]

	var startHash common.Hash
	var endHash common.Hash

	// TODO: update BlockRequest starting block to be variadic type
	if startPrefix == 1 {
		start := binary.LittleEndian.Uint64(startData)

		// check if we have start block
		block, err := s.blockState.GetBlockByNumber(big.NewInt(int64(start)))
		if err != nil {
			return err
		}

		startHash = block.Header.Hash()
	} else if startPrefix == 0 {
		startHash = common.NewHash(startData)
	} else {
		return errors.New("invalid start block in BlockRequest")
	}

	if blockRequest.EndBlockHash.Exists() {
		endHash = blockRequest.EndBlockHash.Value()
	} else {
		endHash = s.blockState.BestBlockHash()
	}

	// get sub-chain of block hashes
	subchain := s.blockState.SubChain(startHash, endHash)

	if len(subchain) > maxResponseSize {
		subchain = subchain[:maxResponseSize]
	}

	responseData := []*types.BlockData{}

	for _, hash := range subchain {
		data, err := s.blockState.GetBlockData(hash)
		if err != nil {
			return err
		}

		blockData := new(types.BlockData)
		blockData.Hash = hash

		// TODO: checks for the existence of the following fields should be implemented once #596 is addressed.

		// header
		if blockRequest.RequestedData&1 == 1 {
			blockData.Header = data.Header
		} else {
			blockData.Header = optional.NewHeader(false, nil)
		}

		// body
		if (blockRequest.RequestedData&2)>>1 == 1 {
			blockData.Body = data.Body
		} else {
			blockData.Body = optional.NewBody(false, nil)
		}

		// receipt
		if (blockRequest.RequestedData&4)>>2 == 1 {
			blockData.Receipt = data.Receipt
		} else {
			blockData.Receipt = optional.NewBytes(false, nil)
		}

		// message queue
		if (blockRequest.RequestedData&8)>>3 == 1 {
			blockData.MessageQueue = data.MessageQueue
		} else {
			blockData.MessageQueue = optional.NewBytes(false, nil)
		}

		// justification
		if (blockRequest.RequestedData&16)>>4 == 1 {
			blockData.Justification = data.Justification
		} else {
			blockData.Justification = optional.NewBytes(false, nil)
		}

		responseData = append(responseData, blockData)
	}

	blockResponse := &network.BlockResponseMessage{
		ID:        blockRequest.ID,
		BlockData: responseData,
	}

	return s.safeMsgSend(blockResponse)
}

// ProcessBlockResponseMessage attempts to validate and add the block to the
// chain by calling `core_execute_block`. Valid blocks are stored in the block
// database to become part of the canonical chain.
func (s *Service) ProcessBlockResponseMessage(msg network.Message) error {
	blockData := msg.(*network.BlockResponseMessage).BlockData

	bestNum, err := s.blockState.BestBlockNumber()
	if err != nil {
		return err
	}

	for _, bd := range blockData {
		if bd.Header.Exists() {
			header, err := types.NewHeaderFromOptional(bd.Header)
			if err != nil {
				return err
			}

			// get block header; if exists, return
			existingHeader, err := s.blockState.GetHeader(bd.Hash)
			if err != nil && existingHeader == nil {
				err = s.blockState.SetHeader(header)
				if err != nil {
					return err
				}

				log.Info("[core] saved block header", "hash", header.Hash(), "number", header.Number)

				// TODO: handle consensus digest, if first in epoch
				// err = s.handleConsensusDigest(header)
				// if err != nil {
				// 	return err
				// }
			}
		}

		if bd.Header.Exists() && bd.Body.Exists {
			header, err := types.NewHeaderFromOptional(bd.Header)
			if err != nil {
				return err
			}

			body, err := types.NewBodyFromOptional(bd.Body)
			if err != nil {
				return err
			}

			block := &types.Block{
				Header: header,
				Body:   body,
			}

			// TODO: why doesn't execute block work with block we built?

			// blockWithoutDigests := block
			// blockWithoutDigests.Header.Digest = [][]byte{{}}

			// enc, err := block.Encode()
			// if err != nil {
			// 	return err
			// }

			// err = s.executeBlock(enc)
			// if err != nil {
			// 	log.Error("[core] failed to validate block", "err", err)
			// 	return err
			// }

			if header.Number.Cmp(bestNum) == 1 {
				err = s.blockState.AddBlock(block)
				if err != nil {
					return err
				}

				log.Info("[core] imported block", "number", header.Number, "hash", header.Hash())

				err = s.checkForRuntimeChanges()
				if err != nil {
					return err
				}
			}
		}

		err := s.compareAndSetBlockData(bd)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Service) compareAndSetBlockData(bd *types.BlockData) error {
	if s.blockState == nil {
		return fmt.Errorf("no blockState")
	}

	existingData, err := s.blockState.GetBlockData(bd.Hash)
	if err != nil {
		// no block data exists, ok
		return s.blockState.SetBlockData(bd)
	}

	if existingData == nil {
		return s.blockState.SetBlockData(bd)
	}

	if existingData.Header == nil || (!existingData.Header.Exists() && bd.Header.Exists()) {
		existingData.Header = bd.Header
	}

	if existingData.Body == nil || (!existingData.Body.Exists && bd.Body.Exists) {
		existingData.Body = bd.Body
	}

	if existingData.Receipt == nil || (!existingData.Receipt.Exists() && bd.Receipt.Exists()) {
		existingData.Receipt = bd.Receipt
	}

	if existingData.MessageQueue == nil || (!existingData.MessageQueue.Exists() && bd.MessageQueue.Exists()) {
		existingData.MessageQueue = bd.MessageQueue
	}

	if existingData.Justification == nil || (!existingData.Justification.Exists() && bd.Justification.Exists()) {
		existingData.Justification = bd.Justification
	}

	return s.blockState.SetBlockData(existingData)
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
func (s *Service) ProcessTransactionMessage(msg network.Message) error {

	// get transactions from message extrinsics
	txs := msg.(*network.TransactionMessage).Extrinsics

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
			s.transactionQueue.Push(vtx)
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
