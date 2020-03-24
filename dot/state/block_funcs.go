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

package state

import (
	"github.com/ChainSafe/gossamer/lib/common"
)

// SetReceipt sets a Receipt in the database
func (bs *BlockState) SetReceipt(hash common.Hash, data []byte) error {
	bs.lock.Lock()
	defer bs.lock.Unlock()

	err := bs.db.Put(prefixHash(hash, receiptPrefix), data)
	if err != nil {
		return err
	}

	return nil
}

// GetReceipt retrieves a Receipt from the database
func (bs *BlockState) GetReceipt(hash common.Hash) ([]byte, error) {
	data, err := bs.db.Get(prefixHash(hash, receiptPrefix))
	if err != nil {
		return nil, err
	}

	return data, nil
}

// SetMessageQueue sets a MessageQueue in the database
func (bs *BlockState) SetMessageQueue(hash common.Hash, data []byte) error {
	bs.lock.Lock()
	defer bs.lock.Unlock()

	err := bs.db.Put(prefixHash(hash, messageQueuePrefix), data)
	if err != nil {
		return err
	}

	return nil
}

// GetMessageQueue retrieves a MessageQueue from the database
func (bs *BlockState) GetMessageQueue(hash common.Hash) ([]byte, error) {
	data, err := bs.db.Get(prefixHash(hash, messageQueuePrefix))
	if err != nil {
		return nil, err
	}

	return data, nil
}

// SetJustification sets a Justification in the database
func (bs *BlockState) SetJustification(hash common.Hash, data []byte) error {
	bs.lock.Lock()
	defer bs.lock.Unlock()

	err := bs.db.Put(prefixHash(hash, justificationPrefix), data)
	if err != nil {
		return err
	}

	return nil
}

// GetJustification retrieves a Justification from the database
func (bs *BlockState) GetJustification(hash common.Hash) ([]byte, error) {
	data, err := bs.db.Get(prefixHash(hash, justificationPrefix))
	if err != nil {
		return nil, err
	}

	return data, nil
}

// prefixHash = prefix + hash
func prefixHash(hash common.Hash, prefix []byte) []byte {
	return append(prefix, hash.ToBytes()...)
}
