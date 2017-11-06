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
	"sync/atomic"

	"github.com/dgraph-io/badger"
	"golang.org/x/net/trace"

	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
)

const (
	// MB represents a megabyte.
	MB = 1 << 20
)

// writeBatch performs a batch write of key value pairs to RocksDB.
func writeBatch(ctx context.Context, pstore *badger.ManagedDB, kv chan *protos.KV, che chan error) {
	var hasError int32
	for i := range kv {
		txn := pstore.NewTransactionAt(math.MaxUint64, true)
		if len(i.Val) == 0 {
			pstore.PurgeVersionsBelow(i.Key, math.MaxUint64)
		} else {
			txn.Set(i.Key, i.Val, i.UserMeta[0])
			txn.CommitAt(i.Version, func(err error) {
				// We don't care about exact error
				x.Printf("Error while committing kv to badger %v\n", err)
				if err != nil {
					atomic.StoreInt32(&hasError, 1)
				}
			})
		}
		defer txn.Discard()
	}
	if hasError == 0 {
		che <- nil
	} else {
		che <- x.Errorf("Error while writing to badger")
	}
}

func streamKeys(pstore *badger.ManagedDB, stream protos.Worker_PredicateAndSchemaDataClient) error {
	txn := pstore.NewTransactionAt(math.MaxUint64, false)
	defer txn.Discard()
	iterOpts := badger.DefaultIteratorOptions
	iterOpts.PrefetchValues = false
	it := txn.NewIterator(iterOpts)
	defer it.Close()

	g := &protos.GroupKeys{
		GroupId: groups().groupId(),
	}

	// Do NOT go to next by default. Be careful when you "continue" in loop.
	for it.Rewind(); it.Valid(); {
		iterItem := it.Item()
		k := iterItem.Key()
		pk := x.Parse(k)
		if pk == nil {
			it.Next()
			continue
		}

		if !groups().ServesTablet(pk.Attr) {
			it.Seek(pk.SkipPredicate())
			continue
		} else if pk.IsSchema() {
			// No version check for schema keys.
			it.Seek(pk.SkipSchema())
			continue
		}

		kdup := make([]byte, len(k))
		copy(kdup, k)
		key := &protos.KC{
			Key: kdup,
		}
		key.Timestamp = iterItem.Version()
		g.Keys = append(g.Keys, key)
		if len(g.Keys) >= 1000 {
			if err := stream.Send(g); err != nil {
				return x.Wrapf(err, "While sending group keys to server.")
			}
			g.Keys = g.Keys[:0]
		}
		it.Next()
	}
	if err := stream.Send(g); err != nil {
		return x.Wrapf(err, "While sending group keys to server.")
	}
	return stream.CloseSend()
}

// PopulateShard gets data for predicate pred from server with id serverId and
// writes it to RocksDB.
func populateShard(ctx context.Context, ps *badger.ManagedDB, pl *conn.Pool, group uint32) (int, error) {
	conn := pl.Get()
	c := protos.NewWorkerClient(conn)

	stream, err := c.PredicateAndSchemaData(context.Background())
	if err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf(err.Error())
		}
		return 0, err
	}
	if tr, ok := trace.FromContext(ctx); ok {
		tr.LazyPrintf("Streaming data for group: %v", group)
	}

	if err := streamKeys(ps, stream); err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf(err.Error())
		}
		return 0, x.Wrapf(err, "While streaming keys group")
	}

	kvs := make(chan *protos.KV, 1000)
	che := make(chan error)
	go writeBatch(ctx, ps, kvs, che)

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

func sendKV(stream protos.Worker_PredicateAndSchemaDataServer, it *badger.Iterator) error {
	item := it.Item()
	key := item.Key()
	pk := x.Parse(key)
	if pk == nil {
		it.Next()
		return nil
	}

	var kv *protos.KV
	if pk.IsSchema() {
		val, err := item.Value()
		if err != nil {
			return err
		}
		kv = &protos.KV{
			Key:      key,
			Val:      val,
			UserMeta: []byte{item.UserMeta()},
			Version:  item.Version(),
		}
		it.Next()
	} else {
		l, err := posting.ReadPostingList(key, it)
		if err != nil {
			return nil
		}
		kv, err = l.MarshalToKv()
		if err != nil {
			return err
		}
	}
	if err := stream.Send(kv); err != nil {
		return err
	}
	return nil
}

// PredicateAndSchemaData can be used to return data corresponding to a predicate over
// a stream.
func (w *grpcWorker) PredicateAndSchemaData(stream protos.Worker_PredicateAndSchemaDataServer) error {
	gkeys := &protos.GroupKeys{}

	// Receive all keys from client first.
	// TODO: Don't fetch all at once, use stream and iterate in parallel
	for {
		keys, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return x.Wrap(err)
		}
		if gkeys.GroupId == 0 {
			gkeys.GroupId = keys.GroupId
		}
		x.AssertTruef(gkeys.GroupId == keys.GroupId,
			"Group ids don't match [%v] v/s [%v]", gkeys.GroupId, keys.GroupId)
		// Do we need to check if keys are sorted? They should already be.
		gkeys.Keys = append(gkeys.Keys, keys.Keys...)
	}
	if tr, ok := trace.FromContext(stream.Context()); ok {
		tr.LazyPrintf("Got %d keys from client\n", len(gkeys.Keys))
	}

	if !x.IsTestRun() {
		if !groups().ServesGroup(gkeys.GroupId) {
			return x.Errorf("Group %d not served.", gkeys.GroupId)
		}
		n := groups().Node
		if !n.AmLeader() {
			return x.Errorf("Not leader of group: %d", gkeys.GroupId)
		}
	}

	// Any commit which happens in the future will have commitTs greater than
	// this.
	// TODO: Ensure all deltas have made to disk and read in memory before checking disk.
	timestamp := posting.Oracle().MaxPending()
	txn := pstore.NewTransactionAt(timestamp, false)
	defer txn.Discard()
	iterOpts := badger.DefaultIteratorOptions
	iterOpts.AllVersions = true
	it := txn.NewIterator(iterOpts)
	defer it.Close()

	var count int
	var gidx int
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
		}
		copy(prevKey, k)

		// If a key is present in follower but not in leader, send a kv with empty value
		// so that the follower can delete it
		for gidx < len(gkeys.Keys) && bytes.Compare(gkeys.Keys[gidx].Key, k) < 0 {
			kv := &protos.KV{Key: gkeys.Keys[gidx].Key}
			gidx++
			if err := stream.Send(kv); err != nil {
				return err
			}
		}

		// Found a match, skip if version is <= version on follower
		if gidx < len(gkeys.Keys) && bytes.Equal(k, gkeys.Keys[gidx].Key) {
			t := gkeys.Keys[gidx]
			gidx++
			if iterItem.Version() <= t.Timestamp {
				it.Next()
				continue
			}
		}

		// This key is not present in follower.
		if err := sendKV(stream, it); err != nil {
			return err
		}
		count++
	} // end of iterator
	// All these keys are not present in leader, so mark them for deletion
	for gidx < len(gkeys.Keys) {
		kv := &protos.KV{Key: gkeys.Keys[gidx].Key}
		gidx++
		if err := stream.Send(kv); err != nil {
			return err
		}
	}
	if tr, ok := trace.FromContext(stream.Context()); ok {
		tr.LazyPrintf("Sent %d keys to client. Done.\n", count)
	}
	return nil
}
