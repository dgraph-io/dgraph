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
	"github.com/dgraph-io/badger/pb"
	"github.com/golang/glog"
)

// TxnWriter is in charge or writing transactions to badger.
type TxnWriter struct {
	db  *badger.DB
	wg  sync.WaitGroup
	che chan error
}

// NewTxnWriter returns a new TxnWriter instance.
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

// Write stores the given key-value pairs in badger.
func (w *TxnWriter) Write(kvs *pb.KVList) error {
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

func (w *TxnWriter) update(commitTs uint64, f func(txn *badger.Txn) error) error {
	if commitTs == 0 {
		return nil
	}
	txn := w.db.NewTransactionAt(math.MaxUint64, true)
	defer txn.Discard()

	err := f(txn)
	if err == badger.ErrTxnTooBig {
		// continue to commit.
	} else if err != nil {
		return err
	}
	w.wg.Add(1)
	return txn.CommitAt(commitTs, w.cb)
}

// SetAt writes a key-value pair at the given timestamp.
func (w *TxnWriter) SetAt(key, val []byte, meta byte, ts uint64) error {
	return w.update(ts, func(txn *badger.Txn) error {
		switch meta {
		case BitCompletePosting, BitEmptyPosting:
			err := txn.SetEntry((&badger.Entry{
				Key:      key,
				Value:    val,
				UserMeta: meta,
			}).WithDiscard())
			if err != nil {
				return err
			}
		default:
			err := txn.SetEntry(&badger.Entry{
				Key:      key,
				Value:    val,
				UserMeta: meta,
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// Flush waits until all operations are done and all data is written to disk.
func (w *TxnWriter) Flush() error {
	defer func() {
		if err := w.db.Sync(); err != nil {
			glog.Errorf("Error while calling Sync from TxnWriter.Flush: %v", err)
		}
	}()
	w.wg.Wait()
	select {
	case err := <-w.che:
		return err
	default:
		return nil
	}
}
