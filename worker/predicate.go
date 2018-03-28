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

package worker

import (
	"bytes"
	"context"
	"io"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/badger"
	"golang.org/x/net/trace"

	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/x"
	humanize "github.com/dustin/go-humanize"
)

const (
	// MB represents a megabyte.
	MB = 1 << 20
)

// writeBatch performs a batch write of key value pairs to BadgerDB.
func writeBatch(ctx context.Context, pstore *badger.ManagedDB, kv chan *intern.KV, che chan error) {
	var bytesWritten uint64
	t := time.NewTicker(5 * time.Second)
	go func() {
		now := time.Now()
		for range t.C {
			dur := time.Since(now)
			x.Printf("Getting SNAPSHOT: Time elapsed: %v, bytes written: %s, bytes/sec %d\n",
				x.FixedDuration(dur), humanize.Bytes(bytesWritten), bytesWritten/uint64(dur.Seconds()))
		}
	}()

	var hasError int32
	var wg sync.WaitGroup // to wait for all callbacks to return
	for i := range kv {
		txn := pstore.NewTransactionAt(math.MaxUint64, true)
		bytesWritten += uint64(i.Size())
		txn.SetWithMeta(i.Key, i.Val, i.UserMeta[0])
		wg.Add(1)
		txn.CommitAt(i.Version, func(err error) {
			// We don't care about exact error
			wg.Done()
			if err != nil {
				x.Printf("Error while committing kv to badger %v\n", err)
				atomic.StoreInt32(&hasError, 1)
			}
		})
	}
	wg.Wait()
	t.Stop()

	if hasError == 0 {
		che <- nil
	} else {
		che <- x.Errorf("Error while writing to badger")
	}
}

// populateShard gets data for a shard from the leader and writes it to BadgerDB on the follower.
func (n *node) populateShard(ps *badger.ManagedDB, pl *conn.Pool) (int, error) {
	conn := pl.Get()
	c := intern.NewWorkerClient(conn)

	n.RLock()
	ctx := n.ctx
	group := n.gid
	stream, err := c.PredicateAndSchemaData(ctx,
		&intern.SnapshotMeta{
			ClientTs: n.RaftContext.SnapshotTs,
			GroupId:  group,
		})
	n.RUnlock()
	if err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf(err.Error())
		}
		return 0, err
	}
	if tr, ok := trace.FromContext(ctx); ok {
		tr.LazyPrintf("Streaming data for group: %v", group)
	}

	kvs := make(chan *intern.KV, 1000)
	che := make(chan error)
	go writeBatch(ctx, ps, kvs, che)

	keyValues, err := stream.Recv()
	if err != nil {
		return 0, err
	}

	x.AssertTrue(len(keyValues.Kv) == 1)
	ikv := keyValues.Kv[0]
	// First key has the snapshot ts from the leader.
	x.AssertTrue(bytes.Equal(ikv.Key, []byte("min_ts")))
	n.Lock()
	n.RaftContext.SnapshotTs = ikv.Version
	n.Unlock()

	// We can use count to check the number of posting lists returned in tests.
	count := 0
	for {
		keyValues, err = stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			if tr, ok := trace.FromContext(ctx); ok {
				tr.LazyPrintf(err.Error())
			}
			close(kvs)
			return count, err
		}
		for _, kv := range keyValues.Kv {
			count++

			// We check for errors, if there are no errors we send value to channel.
			select {
			case kvs <- kv:
				// OK
			case <-ctx.Done():
				if tr, ok := trace.FromContext(ctx); ok {
					tr.LazyPrintf("Context timed out while streaming group: %v", group)
				}
				close(kvs)
				return 0, ctx.Err()
			case err := <-che:
				if tr, ok := trace.FromContext(ctx); ok {
					tr.LazyPrintf("Error while doing a batch write for group: %v", group)
				}
				close(kvs)
				// Important: Don't put return count, err
				// There was a compiler bug which was fixed in 1.8.1
				// https://github.com/golang/go/issues/21722.
				// Probably should be ok to return count, err now
				return 0, err
			}
		}
	}
	close(kvs)

	if err := <-che; err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Error while doing a batch write for group: %v", group)
		}
		return count, err
	}
	if tr, ok := trace.FromContext(ctx); ok {
		tr.LazyPrintf("Streaming complete for group: %v", group)
	}
	return count, nil
}

func toKV(it *badger.Iterator, pk *x.ParsedKey) (*intern.KV, error) {
	item := it.Item()
	var kv *intern.KV

	key := make([]byte, len(item.Key()))
	// Key would be modified by ReadPostingList as it advances the iterator and changes the item.
	copy(key, item.Key())

	if pk.IsSchema() {
		val, err := item.ValueCopy(nil)
		if err != nil {
			return nil, err
		}
		kv = &intern.KV{
			Key:      key,
			Val:      val,
			UserMeta: []byte{item.UserMeta()},
			Version:  item.Version(),
		}
		it.Next()
		return kv, nil
	}

	l, err := posting.ReadPostingList(key, it)
	if err != nil {
		return nil, err
	}
	kv, err = l.MarshalToKv()
	if err != nil {
		return nil, err
	}
	return kv, nil
}

func (w *grpcWorker) PredicateAndSchemaData(m *intern.SnapshotMeta, stream intern.Worker_PredicateAndSchemaDataServer) error {
	clientTs := m.ClientTs

	if !x.IsTestRun() {
		if !groups().ServesGroup(m.GroupId) {
			return x.Errorf("Group %d not served.", m.GroupId)
		}
		n := groups().Node
		if !n.AmLeader() {
			return x.Errorf("Not leader of group: %d", m.GroupId)
		}
	}

	// Any commit which happens in the future will have commitTs greater than
	// this.
	// TODO: Ensure all deltas have made to disk and read in memory before checking disk.
	min_ts := posting.Txns().MinTs()

	// Send ts as first KV.
	if err := stream.Send(&intern.KVS{
		Kv: []*intern.KV{&intern.KV{
			Key:     []byte("min_ts"),
			Version: min_ts,
		}},
	}); err != nil {
		return err
	}

	txn := pstore.NewTransactionAt(min_ts, false)
	defer txn.Discard()
	iterOpts := badger.DefaultIteratorOptions
	iterOpts.AllVersions = true
	it := txn.NewIterator(iterOpts)
	defer it.Close()

	var count int
	var batchSize int
	var prevKey []byte
	kvs := &intern.KVS{}
	// Do NOT it.Next() by default. Be careful when you "continue" in loop!
	var bytesSent uint64
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	go func() {
		now := time.Now()
		for range t.C {
			dur := time.Since(now)
			x.Printf("Sending SNAPSHOT: Time elapsed: %v, bytes sent: %s, bytes/sec %d\n",
				x.FixedDuration(dur), humanize.Bytes(bytesSent), bytesSent/uint64(dur.Seconds()))
		}
	}()
	for it.Rewind(); it.Valid(); {
		iterItem := it.Item()
		k := iterItem.Key()
		if bytes.Equal(k, prevKey) {
			it.Next()
			continue
		}

		if cap(prevKey) < len(k) {
			prevKey = make([]byte, len(k))
		} else {
			prevKey = prevKey[:len(k)]
		}
		copy(prevKey, k)

		pk := x.Parse(prevKey)
		// Schema keys always have version 1. So we send it irrespective of the timestamp.
		if iterItem.Version() <= clientTs && !pk.IsSchema() {
			it.Next()
			continue
		}

		// This key is not present in follower.
		kv, err := toKV(it, pk)
		if err != nil {
			return err
		}
		kvs.Kv = append(kvs.Kv, kv)
		batchSize += kv.Size()
		bytesSent += uint64(kv.Size())
		count++
		if batchSize < MB { // 1MB
			continue
		}
		if err := stream.Send(kvs); err != nil {
			return err
		}
	} // end of iterator
	if batchSize > 0 {
		if err := stream.Send(kvs); err != nil {
			return err
		}
	}
	if tr, ok := trace.FromContext(stream.Context()); ok {
		tr.LazyPrintf("Sent %d keys to client. Done.\n", count)
	}
	return nil
}
