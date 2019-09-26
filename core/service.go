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
	log "github.com/ChainSafe/log15"

	scale "github.com/ChainSafe/gossamer/codec"
	tx "github.com/ChainSafe/gossamer/common/transaction"
	"github.com/ChainSafe/gossamer/consensus/babe"
	"github.com/ChainSafe/gossamer/core/types"
	"github.com/ChainSafe/gossamer/p2p"
	"github.com/ChainSafe/gossamer/runtime"
)

// Service is a overhead layer that allows for communication between the runtime, BABE, and the p2p layer.
// It deals with the validation of transactions and blocks by calling their respective validation functions
// in the runtime.
type Service struct {
	rt *runtime.Runtime
	b  *babe.Session

	msgChan <-chan p2p.Message
}

// NewService returns a Service that connects the runtime, BABE, and the p2p messages.
func NewService(rt *runtime.Runtime, b *babe.Session, msgChan <-chan p2p.Message) *Service {
	return &Service{
		rt:      rt,
		b:       b,
		msgChan: msgChan,
	}
}

// Start begins the service. This begins watching the message channel for new block or transaction messages.
func (s *Service) Start() <-chan error {
	e := make(chan error)
	go s.start(e)
	return e
}

func (s *Service) start(e chan error) {
	go func(msgChan <-chan p2p.Message) {
		msg := <-msgChan
		msgType := msg.GetType()
		switch msgType {
		case p2p.TransactionMsgType:
			// process tx
		case p2p.BlockAnnounceMsgType:
			// process block
		default:
			log.Error("core service", "error", "got unsupported message type")
		}
	}(s.msgChan)

	e <- nil
}

func (s *Service) Stop() <-chan error {
	e := make(chan error)

	return e
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
func (s *Service) ProcessBlock(b *types.BlockHeader) error {
	return nil
}

// runs the extrinsic through runtime function TaggedTransactionQueue_validate_transaction
// and returns *Validity
func (s *Service) validateTransaction(e types.Extrinsic) (*tx.Validity, error) {
	ret, err := s.rt.Exec("TaggedTransactionQueue_validate_transaction", 1, 0)
	if err != nil {
		return nil, err
	}

	v := new(tx.Validity)
	_, err = scale.Decode(ret, v)
	return v, err
}
