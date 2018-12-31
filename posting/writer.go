/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package posting

import (
	"math"
	"sync"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
)

type TxnWriter struct {
	db  *badger.DB
	wg  sync.WaitGroup
	che chan error

	// This can be set to allow overwrites during Set.
	BlindWrite bool
}

func NewTxnWriter(db *badger.DB) *TxnWriter {
	return &TxnWriter{
		db:  db,
		che: make(chan error, 1),
	}
}

func (w *TxnWriter) cb(err error) {
	defer w.wg.Done()
	if err == nil {
		return
	}

	glog.Errorf("TxnWriter got error during callback: %v", err)
	select {
	case w.che <- err:
	default:
	}
}

func (w *TxnWriter) Send(kvs *pb.KVS) error {
	for _, kv := range kvs.Kv {
		var meta byte
		if len(kv.UserMeta) > 0 {
			meta = kv.UserMeta[0]
		}
		if err := w.SetAt(kv.Key, kv.Value, meta, kv.Version); err != nil {
			return err
		}
	}
	return nil
}

func (w *TxnWriter) Delete(key []byte, ts uint64) error {
	if ts == 0 {
		return nil
	}
	txn := w.db.NewTransactionAt(math.MaxUint64, true)
	defer txn.Discard()
	if err := txn.Delete(key); err != nil {
		return err
	}
	w.wg.Add(1)
	return txn.CommitAt(ts, w.cb)
}

func (w *TxnWriter) SetAt(key, val []byte, meta byte, ts uint64) error {
	if ts == 0 {
		return nil
	}

	txn := w.db.NewTransactionAt(math.MaxUint64, true)
	defer txn.Discard()

	if !w.BlindWrite {
		// We do a Get to ensure that we don't end up overwriting an already
		// existing delta or state at the ts.
		if item, err := txn.Get(key); err == badger.ErrKeyNotFound {
			// pass
		} else if err != nil {
			return err

		} else if item.Version() >= ts {
			// Found an existing commit at an equal or higher timestamp. So, skip writing.
			if glog.V(2) {
				pk := x.Parse(key)
				glog.Warningf("Existing >= Commit [%d >= %d]. Skipping write: %v",
					item.Version(), ts, pk)
			}
			return nil
		}
	}
	switch meta {
	case BitCompletePosting, BitEmptyPosting:
		if err := txn.SetWithDiscard(key, val, meta); err != nil {
			return err
		}
	default:
		if err := txn.SetWithMeta(key, val, meta); err != nil {
			return err
		}
	}
	w.wg.Add(1)
	return txn.CommitAt(ts, w.cb)
}

func (w *TxnWriter) Flush() error {
	w.wg.Wait()
	select {
	case err := <-w.che:
		return err
	default:
		return nil
	}
}
