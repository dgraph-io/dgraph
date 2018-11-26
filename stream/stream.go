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

package stream

import (
	"bytes"
	"sync"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/y"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"

	humanize "github.com/dustin/go-humanize"
	"github.com/golang/glog"
	"golang.org/x/net/context"
)

const pageSize = 4 << 20 // 4MB

type kvStream interface {
	Send(*pb.KVS) error
}

type Lists struct {
	Stream        kvStream
	Predicate     string
	DB            *badger.DB
	ChooseKeyFunc func(item *badger.Item) bool
	ItemToKVFunc  func(key []byte, itr *badger.Iterator) (*pb.KV, error)
}

// keyRange is [start, end), including start, excluding end. Do ensure that the start,
// end byte slices are owned by keyRange struct.
type keyRange struct {
	start []byte
	end   []byte
}

func (sl *Lists) Orchestrate(ctx context.Context, logPrefix string, ts uint64) error {
	keyCh := make(chan keyRange, 3) // Contains keys for posting lists.
	// kvChan should only have a small capacity to ensure that we don't buffer up too much data, if
	// sending is slow. So, setting this to 3.
	kvChan := make(chan *pb.KVS, 3) // Contains marshaled posting lists.
	errCh := make(chan error, 1)    // Stores error by consumeKeys.

	// Read the predicate keys and stream to keysCh.
	go sl.produceRanges(ctx, ts, keyCh)

	// Read the posting lists corresponding to keys and send to kvChan.
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := sl.produceKVs(ctx, ts, keyCh, kvChan); err != nil {
				select {
				case errCh <- err:
				default:
				}
			}
		}()
	}

	// Pick up key-values from kvChan and send to stream.
	kvErr := make(chan error, 1)
	go func() {
		kvErr <- sl.streamKVs(ctx, logPrefix, kvChan)
	}()
	wg.Wait()     // Wait for produceKVs to be over.
	close(kvChan) // Now we can close kvChan.

	select {
	case err := <-errCh: // Check error from produceKVs.
		return err
	default:
	}

	// Wait for key streaming to be over.
	if err := <-kvErr; err != nil {
		return err
	}
	return nil
}

func (sl *Lists) produceRanges(ctx context.Context, ts uint64, keyCh chan keyRange) {
	var prefix []byte
	if len(sl.Predicate) > 0 {
		prefix = x.PredicatePrefix(sl.Predicate)
	}
	splits := sl.DB.KeySplits(prefix)
	start := prefix
	for _, key := range splits {
		keyCh <- keyRange{start: start, end: y.SafeCopy(nil, []byte(key))}
		start = y.SafeCopy(nil, []byte(key))
	}
	// Edge case: prefix is empty and no splits exist. In that case, we should have at least one
	// keyRange output.
	keyCh <- keyRange{start: start}
	close(keyCh)
}

func (sl *Lists) produceKVs(ctx context.Context, ts uint64,
	keyCh chan keyRange, kvChan chan *pb.KVS) error {
	var prefix []byte
	if len(sl.Predicate) > 0 {
		prefix = x.PredicatePrefix(sl.Predicate)
	}

	var size int
	txn := sl.DB.NewTransactionAt(ts, false)
	defer txn.Discard()
	iterate := func(kr keyRange) error {
		iterOpts := badger.DefaultIteratorOptions
		iterOpts.AllVersions = true
		iterOpts.Prefix = prefix
		iterOpts.PrefetchValues = false
		it := txn.NewIterator(iterOpts)
		defer it.Close()

		kvs := new(pb.KVS)
		var prevKey []byte
		for it.Seek(kr.start); it.Valid(); {
			// it.Valid would only return true for keys with the provided Prefix in iterOpts.
			item := it.Item()
			if bytes.Equal(item.Key(), prevKey) {
				it.Next()
				continue
			}
			prevKey = append(prevKey[:0], item.Key()...)

			// Check if we reached the end of the key range.
			if len(kr.end) > 0 && bytes.Compare(item.Key(), kr.end) >= 0 {
				break
			}
			// Check if we should pick this key.
			if sl.ChooseKeyFunc != nil && !sl.ChooseKeyFunc(item) {
				continue
			}

			// Now convert to key value.
			kv, err := sl.ItemToKVFunc(item.KeyCopy(nil), it)
			if err != nil {
				return err
			}
			if kv != nil {
				kvs.Kv = append(kvs.Kv, kv)
				size += kv.Size()
			}
			if size >= pageSize {
				kvChan <- kvs
				kvs = new(pb.KVS)
				size = 0
			}
		}
		if len(kvs.Kv) > 0 {
			kvChan <- kvs
		}
		return nil
	}

	for {
		select {
		case kr, ok := <-keyCh:
			if !ok {
				// Done with the keys.
				return nil
			}
			if err := iterate(kr); err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (sl *Lists) streamKVs(ctx context.Context, logPrefix string, kvChan chan *pb.KVS) error {
	var count int
	var bytesSent uint64
	t := time.NewTicker(time.Second)
	defer t.Stop()
	now := time.Now()

	slurp := func(batch *pb.KVS) error {
	loop:
		for {
			select {
			case kvs, ok := <-kvChan:
				if !ok {
					break loop
				}
				x.AssertTrue(kvs != nil)
				batch.Kv = append(batch.Kv, kvs.Kv...)
			default:
				break loop
			}
		}
		sz := uint64(batch.Size())
		bytesSent += sz
		count += len(batch.Kv)
		t := time.Now()
		if err := sl.Stream.Send(batch); err != nil {
			return err
		}
		glog.V(2).Infof("%s Created batch of size: %s in %s.\n",
			logPrefix, humanize.Bytes(sz), time.Since(t))
		return nil
	}

outer:
	for {
		var batch *pb.KVS
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-t.C:
			dur := time.Since(now)
			durSec := uint64(dur.Seconds())
			if durSec == 0 {
				continue
			}
			speed := bytesSent / durSec
			glog.Infof("%s Time elapsed: %s, bytes sent: %s, speed: %s/sec\n",
				logPrefix, x.FixedDuration(dur), humanize.Bytes(bytesSent), humanize.Bytes(speed))

		case kvs, ok := <-kvChan:
			if !ok {
				break outer
			}
			x.AssertTrue(kvs != nil)
			batch = kvs
			if err := slurp(batch); err != nil {
				return err
			}
		}
	}

	glog.Infof("%s Sent %d keys\n", logPrefix, count)
	return nil
}
