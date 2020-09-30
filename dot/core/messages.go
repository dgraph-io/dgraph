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
	"github.com/ChainSafe/gossamer/dot/network"
	"github.com/ChainSafe/gossamer/lib/transaction"
)

// ProcessTransactionMessage validates each transaction in the message and
// adds valid transactions to the transaction queue of the BABE session
func (s *Service) ProcessTransactionMessage(msg *network.TransactionMessage) error {
	s.logger.Debug("received TransactionMessage")

	// get transactions from message extrinsics
	txs := msg.Extrinsics

	for _, tx := range txs {
		tx := tx // pin

		// validate each transaction
		val, err := s.rt.ValidateTransaction(tx)
		if err != nil {
			s.logger.Error("failed to validate transaction", "err", err)
			return err // exit
		}

		// create new valid transaction
		vtx := transaction.NewValidTransaction(tx, val)

		if s.isBlockProducer {
			// push to the transaction queue of BABE session
			hash := s.transactionState.AddToPool(vtx)
			s.logger.Trace("Added transaction to queue", "hash", hash)
		}
	}

	return nil
}
