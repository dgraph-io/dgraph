/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */
package x

import (
	"encoding/binary"
	"sync"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

const (
	leaseBandwidth = uint64(10000)
)

var (
	emptyAssignedIds = &protos.AssignedIds{}
)

type EmbeddedUidAllocator struct {
	sync.Mutex
	nextLeaseId uint64
	maxLeaseId  uint64
	pstore      *badger.ManagedDB
}

// Start lease from 2, 1 is used by _lease_
func (e *EmbeddedUidAllocator) Init(kv *badger.ManagedDB) {
	e.pstore = kv
	e.maxLeaseId = 1
	var n int
	// All keys start with 0x00 or 0x01 so shouldn't collide
	txn := kv.NewTransactionAt(1, false)
	defer txn.Discard()
	item, err := txn.Get([]byte("uid_lease"))
	if err == badger.ErrKeyNotFound {
		// Do nothing
	} else if err != nil {
		Check(err)
	} else {
		val, err := item.Value()
		Check(err)
		e.maxLeaseId, n = binary.Uvarint(val)
		AssertTrue(n > 0)
	}
	e.nextLeaseId = e.maxLeaseId + 1
}

func (e *EmbeddedUidAllocator) AssignUids(ctx context.Context,
	num *protos.Num) (*protos.AssignedIds, error) {
	val := int(num.Val)
	if val == 0 {
		return emptyAssignedIds, Errorf("Nothing to be marked or assigned")
	}

	e.Lock()
	defer e.Unlock()

	howMany := leaseBandwidth
	if num.Val > leaseBandwidth {
		howMany = num.Val + leaseBandwidth
	}

	if e.nextLeaseId == 0 {
		return nil, errors.New("Server not initialized.")
	}

	available := e.maxLeaseId - e.nextLeaseId + 1

	if available < num.Val {
		e.maxLeaseId += howMany
		val := make([]byte, 10)
		n := binary.PutUvarint(val, e.maxLeaseId)
		txn := e.pstore.NewTransactionAt(1, true)
		defer txn.Discard()
		err := txn.Set([]byte("uid_lease"), val[:n], 0x01)
		if err == nil {
			err = txn.CommitAt(1, nil)
		}
		if err != nil {
			return emptyAssignedIds, err
		}
	}

	out := &protos.AssignedIds{}
	out.StartId = e.nextLeaseId
	out.EndId = out.StartId + num.Val - 1
	e.nextLeaseId = out.EndId + 1
	return out, nil
}
