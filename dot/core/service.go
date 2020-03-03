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
	"fmt"
	"math/big"
	mrand "math/rand"
	"time"

	"golang.org/x/exp/rand"

	"github.com/ChainSafe/gossamer/dot/core/types"
	"github.com/ChainSafe/gossamer/dot/network"
	"github.com/ChainSafe/gossamer/lib/babe"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/common/optional"
	"github.com/ChainSafe/gossamer/lib/crypto"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
	"github.com/ChainSafe/gossamer/lib/keystore"
	"github.com/ChainSafe/gossamer/lib/runtime"
	"github.com/ChainSafe/gossamer/lib/services"
	"github.com/ChainSafe/gossamer/lib/transaction"

	log "github.com/ChainSafe/log15"
)

var _ services.Service = &Service{}

// Service is an overhead layer that allows communication between the runtime,
// BABE session, and network service. It deals with the validation of transactions
// and blocks by calling their respective validation functions in the runtime.
type Service struct {
	blockState        BlockState
	storageState      StorageState
	transactionQueue  TransactionQueue
	rt                *runtime.Runtime
	bs                *babe.Session
	keys              []crypto.Keypair
	blkRec            <-chan types.Block     // receive blocks from BABE session
	msgRec            <-chan network.Message // receive messages from network service
	epochDone         <-chan struct{}        // receive from this channel when BABE epoch changes
	msgSend           chan<- network.Message // send messages to network service
	isBabeAuthority   bool
	requestedBlockIDs map[uint64]bool // track requested block id messages
}

// Config holds the config obj
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

	var srv = &Service{}

	if cfg.IsBabeAuthority {
		if cfg.Keystore.NumSr25519Keys() == 0 {
			return nil, fmt.Errorf("no keys provided for authority node")
		}

		epochDone := make(chan struct{})

		srv = &Service{
			rt:               cfg.Runtime,
			keys:             keys,
			blkRec:           cfg.NewBlocks, // becomes block receive channel in core service
			msgRec:           cfg.MsgRec,
			msgSend:          cfg.MsgSend,
			blockState:       cfg.BlockState,
			storageState:     cfg.StorageState,
			transactionQueue: cfg.TransactionQueue,
			epochDone:        epochDone,
			isBabeAuthority:  true,
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
			keys:             keys,
			blkRec:           cfg.NewBlocks, // becomes block receive channel in core service
			msgRec:           cfg.MsgRec,
			msgSend:          cfg.MsgSend,
			blockState:       cfg.BlockState,
			storageState:     cfg.StorageState,
			transactionQueue: cfg.TransactionQueue,
			isBabeAuthority:  false,
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

	// stop runtime
	if s.rt != nil {
		s.rt.Stop()
	}

	// close message channel to network service
	if s.msgSend != nil {
		close(s.msgSend)
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

		// BABE session configuration
		bsConfig := &babe.SessionConfig{
			Keypair:          s.keys[0].(*sr25519.Keypair),
			Runtime:          s.rt,
			NewBlocks:        newBlocks, // becomes block send channel in BABE session
			BlockState:       s.blockState,
			StorageState:     s.storageState,
			TransactionQueue: s.transactionQueue,
			AuthData:         s.bs.AuthorityData(), // AuthorityData will be updated when the NextEpochDescriptor arrives.
			Done:             epochDone,
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

	// send block announce message to network service
	s.msgSend <- msg

	// TODO: check if host status message needs to be updated based on new block
	// information, if so, generate host status message and send to network service

	// TODO: send updated host status message to network service
	// s.msgSend <- msg

	return nil
}

// handleReceivedMessage handles messages from the network service
func (s *Service) handleReceivedMessage(msg network.Message) (err error) {
	msgType := msg.GetType()

	switch msgType {
	case network.BlockAnnounceMsgType:
		err = s.ProcessBlockAnnounceMessage(msg)
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

		log.Info("[core] imported block", "number", header.Number, "hash", header.Hash())

	} else {
		return err
	}

	chainHead, err := s.blockState.BestBlockHeader()
	if err != nil {
		return err
	}

	latestBlockNum := chainHead.Number
	messageBlockNumMinusOne := big.NewInt(0).Sub(blockAnnounceMessage.Number, big.NewInt(1))

	// check if we should send block request message
	if latestBlockNum.Cmp(messageBlockNumMinusOne) == -1 {

		//generate random ID
		s1 := rand.NewSource(uint64(time.Now().UnixNano()))
		seed := rand.New(s1).Uint64()
		randomID := mrand.New(mrand.NewSource(int64(seed))).Uint64()

		currentHash := chainHead.Hash()

		blockRequest := &network.BlockRequestMessage{
			ID:            randomID, // random
			RequestedData: 2,        // block body
			StartingBlock: append([]byte{0}, currentHash[:]...),
			EndBlockHash:  optional.NewHash(true, header.Hash()),
			Direction:     1,
			Max:           optional.NewUint32(false, 0),
		}

		//track request
		s.requestedBlockIDs[randomID] = true

		// send block request message to network service
		log.Debug("send blockRequest message to network service")
		s.msgSend <- blockRequest

	}

	return nil
}

// ProcessBlockResponseMessage attempts to validate and add the block to the
// chain by calling `core_execute_block`. Valid blocks are stored in the block
// database to become part of the canonical chain.
func (s *Service) ProcessBlockResponseMessage(msg network.Message) error {
	blockData := msg.(*network.BlockResponseMessage).BlockData

	for _, bd := range blockData {
		if bd.Header.Exists() {
			header, err := types.NewHeaderFromOptional(bd.Header)
			if err != nil {
				return err
			}

			err = s.handleConsensusDigest(header)
			if err != nil {
				return err
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

			enc, err := block.Encode()
			if err != nil {
				return err
			}

			err = s.executeBlock(enc)
			if err != nil {
				log.Error("[core] failed to validate block", "err", err)
				return err
			}

			// get block header; if exists, return
			existingHeader, err := s.blockState.GetHeader(bd.Hash)
			if err == nil && existingHeader != nil {
				// header exists, return
				// TODO: store this block in the blocktree
				return nil
			}

			err = s.blockState.SetHeader(header)
			if err != nil {
				return err
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
