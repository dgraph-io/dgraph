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
	"fmt"
	"io"
	"math"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/badger"
	"golang.org/x/net/trace"

	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/x"
)

const (
	// MB represents a megabyte.
	MB = 1 << 20
)

// writeBatch performs a batch write of key value pairs to BadgerDB.
func writeBatch(ctx context.Context, pstore *badger.ManagedDB, kv chan *intern.KV, che chan error) {
	bytesWritten := 0
	t := time.NewTicker(5 * time.Second)
	go func() {
		now := time.Now()
		for range t.C {
			fmt.Printf("Writing Snapshot. Time elapsed: %v, bytes written: %d\n", time.Since(now), bytesWritten)
		}
	}()

	var hasError int32
	for i := range kv {
		txn := pstore.NewTransactionAt(math.MaxUint64, true)
		if len(i.Val) == 0 {
			pstore.PurgeVersionsBelow(i.Key, math.MaxUint64)
		} else {
			bytesWritten += len(i.Key) + len(i.Val)
			txn.SetWithMeta(i.Key, i.Val, i.UserMeta[0])
			txn.CommitAt(i.Version, func(err error) {
				// We don't care about exact error
				if err != nil {
					x.Printf("Error while committing kv to badger %v\n", err)
					atomic.StoreInt32(&hasError, 1)
				}
			})
		}
		defer txn.Discard()
	}
	t.Stop()

	// TODO - Wait for all callbacks to return.
	if hasError == 0 {
		che <- nil
	} else {
		che <- x.Errorf("Error while writing to badger")
	}
}

// PopulateShard gets for a shard from the leader and writes it to BadgerDB on the follower.
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

	ikv, err := stream.Recv()
	if err != nil {
		return 0, err
	}

	// First key has the snapshot ts from the leader.
	x.AssertTrue(bytes.Equal(ikv.Key, []byte("min_ts")))
	n.Lock()
	n.RaftContext.SnapshotTs = ikv.Version
	n.Unlock()

	// We can use count to check the number of posting lists returned in tests.
	count := 0
	for {
		kv, err := stream.Recv()
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
			return 0, err
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

func sendKV(stream intern.Worker_PredicateAndSchemaDataServer, it *badger.Iterator) error {
	item := it.Item()
	pk := x.Parse(item.Key())

	var kv *intern.KV
	if pk.IsSchema() {
		val, err := item.Value()
		if err != nil {
			return err
		}
		kv = &intern.KV{
			Key:      item.Key(),
			Val:      val,
			UserMeta: []byte{item.UserMeta()},
			Version:  item.Version(),
		}
		if err := stream.Send(kv); err != nil {
			return err
		}
		it.Next()
		return nil
	}

	key := make([]byte, len(item.Key()))
	// Key would be modified by ReadPostingList as it advances the iterator and changes the item.
	copy(key, item.Key())

	l, err := posting.ReadPostingList(key, it)
	if err != nil {
		return nil
	}
	kv, err = l.MarshalToKv()
	if err != nil {
		return err
	}
	if err := stream.Send(kv); err != nil {
		return err
	}
	return nil
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
	if err := stream.Send(&intern.KV{
		Key:     []byte("min_ts"),
		Version: min_ts,
	}); err != nil {
		return err
	}

	txn := pstore.NewTransactionAt(min_ts, false)
	defer txn.Discard()
	iterOpts := badger.DefaultIteratorOptions
	iterOpts.AllVersions = true
	iterOpts.PrefetchValues = false
	it := txn.NewIterator(iterOpts)
	defer it.Close()

	var count int
	var prevKey []byte
	// Do NOT it.Next() by default. Be careful when you "continue" in loop!
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

		// TODO - Schema keys version is always one.
		if iterItem.Version() <= clientTs {
			it.Next()
			continue
		}

		// This key is not present in follower.
		if err := sendKV(stream, it); err != nil {
			return err
		}
		count++
	} // end of iterator
	if tr, ok := trace.FromContext(stream.Context()); ok {
		tr.LazyPrintf("Sent %d keys to client. Done.\n", count)
	}
	return nil
}
