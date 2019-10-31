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
	"github.com/ChainSafe/gossamer/internal/services"
	log "github.com/ChainSafe/log15"

	"github.com/ChainSafe/gossamer/common"
	tx "github.com/ChainSafe/gossamer/common/transaction"
	"github.com/ChainSafe/gossamer/consensus/babe"
	"github.com/ChainSafe/gossamer/core/types"
	"github.com/ChainSafe/gossamer/p2p"
	"github.com/ChainSafe/gossamer/runtime"
)

var _ services.Service = &Service{}

// Service is a overhead layer that allows for communication between the runtime, BABE, and the p2p layer.
// It deals with the validation of transactions and blocks by calling their respective validation functions
// in the runtime.
type Service struct {
	rt *runtime.Runtime
	b  *babe.Session

	msgChan <-chan []byte
}

// NewService returns a Service that connects the runtime, BABE, and the p2p messages.
func NewService(rt *runtime.Runtime, b *babe.Session, msgChan <-chan []byte) *Service {
	return &Service{
		rt:      rt,
		b:       b,
		msgChan: msgChan,
	}
}

// Start begins the service. This begins watching the message channel for new block or transaction messages.
func (s *Service) Start() error {
	e := make(chan error)
	go s.start(e)
	return <-e
}

func (s *Service) start(e chan error) {
	e <- nil

	for {
		msg, ok := <-s.msgChan
		if !ok {
			log.Warn("core service message watcher", "error", "channel closed")
			break
		}

		msgType := msg[0]
		switch msgType {
		case p2p.TransactionMsgType:
			// process tx
			err := s.ProcessTransaction(msg[1:])
			if err != nil {
				log.Error("core service", "error", err)
				e <- err
			}
			e <- nil
		case p2p.BlockAnnounceMsgType:
			// get extrinsics by sending BlockRequest message
			// process block
		case p2p.BlockResponseMsgType:
			// process response
			err := s.ProcessBlock(msg[1:])
			if err != nil {
				log.Error("core service", "error", err)
				e <- err
			}
			e <- nil
		default:
			log.Error("core service", "error", "got unsupported message type")
		}
	}
}

func (s *Service) Stop() error {
	if s.rt != nil {
		s.rt.Stop()
	}
	return nil
}

func (s *Service) StorageRoot() (common.Hash, error) {
	return s.rt.StorageRoot()
}

// ProcessTransaction attempts to validates the transaction
// if it is validated, it is added to the transaction pool of the BABE session
func (s *Service) ProcessTransaction(e types.Extrinsic) error {
	validity, err := s.validateTransaction(e)
	if err != nil {
		log.Error("ProcessTransaction", "error", err)
		return err
	}

	vtx := tx.NewValidTransaction(&e, validity)
	s.b.PushToTxQueue(vtx)

	return nil
}

// ProcessBlock attempts to add a block to the chain by calling `core_execute_block`
// if the block is validated, it is stored in the block DB and becomes part of the canonical chain
func (s *Service) ProcessBlock(b []byte) error {
	err := s.validateBlock(b)
	return err
}
