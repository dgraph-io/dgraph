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
	"crypto/md5"
	"io"

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
func writeBatch(ctx context.Context, pstore *badger.DB, kv chan *protos.KV, che chan error) {
	txn := pstore.NewTransaction(true)
	defer txn.Discard()
	batchSize := 0
	batchWriteNum := 1
	for i := range kv {
		if len(i.Val) == 0 {
			txn.Delete(i.Key)
		} else {
			txn.Set(i.Key, i.Val, i.UserMeta[0])
		}
		batchSize += len(i.Key) + len(i.Val)
		// We write in batches of size 32MB.
		if batchSize >= 32*MB {
			if tr, ok := trace.FromContext(ctx); ok {
				tr.LazyPrintf("SNAPSHOT: Doing batch write num: %d", batchWriteNum)
			}
			if err := txn.Commit(nil); err != nil {
				che <- err
				return
			}

			batchWriteNum++
			// Resetting batch size after a batch write.
			batchSize = 0
			// Since we are writing data in batches, we need to clear up items enqueued
			// for batch write after every successful write.
			txn = pstore.NewTransaction(true)
		}
	}
	if batchSize > 0 {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Doing batch write %d.", batchWriteNum)
		}
		if err := txn.Commit(nil); err != nil {
			che <- err
			return
		}
	}
	che <- nil
}

func streamKeys(pstore *badger.DB, stream protos.Worker_PredicateAndSchemaDataClient) error {
	txn := pstore.NewTransaction(false)
	defer txn.Discard()
	it := txn.NewIterator(badger.DefaultIteratorOptions)
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
		// No need to send KC for schema keys, since we won't save anything
		// by sending checksum of schema key
		if pk.IsSchema() {
			it.Seek(pk.SkipSchema())
			// Do not go next.
			continue
		}

		if !groups().ServesTablet(pk.Attr) {
			it.Seek(pk.SkipPredicate())
			continue
		}

		kdup := make([]byte, len(k))
		copy(kdup, k)
		key := &protos.KC{
			Key: kdup,
		}
		val, err := iterItem.Value()
		if err != nil {
			return err
		}
		checksum := md5.Sum(val)
		key.Checksum = checksum[:]
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
func populateShard(ctx context.Context, ps *badger.DB, pl *conn.Pool, group uint32) (int, error) {
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

// PredicateAndSchemaData can be used to return data corresponding to a predicate over
// a stream.
func (w *grpcWorker) PredicateAndSchemaData(stream protos.Worker_PredicateAndSchemaDataServer) error {
	gkeys := &protos.GroupKeys{}

	// Receive all keys from client first.
	// TODO: Weird that we batch keys in batches of 1000 on the client, but but not here.
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

	txn := pstore.NewTransaction(false)
	defer txn.Discard()
	it := txn.NewIterator(badger.DefaultIteratorOptions)
	defer it.Close()

	var count int
	var gidx int
	// Do NOT it.Next() by default. Be careful when you "continue" in loop!
	for it.Rewind(); it.Valid(); {
		iterItem := it.Item()
		k := iterItem.Key()
		pk := x.Parse(k)

		if pk == nil {
			it.Next()
			continue
		}

		// No checksum check for schema keys
		var v []byte
		val, err := iterItem.Value()
		if err != nil {
			return err
		}
		v = make([]byte, len(val))
		// Copy it as we have to access it outside.
		copy(v, val)

		if !pk.IsSchema() {
			var pl protos.PostingList
			posting.UnmarshalOrCopy(v, iterItem.UserMeta(), &pl)

			// If a key is present in follower but not in leader, send a kv with empty value
			// so that the follower can delete it
			for gidx < len(gkeys.Keys) && bytes.Compare(gkeys.Keys[gidx].Key, k) < 0 {
				kv := &protos.KV{Key: gkeys.Keys[gidx].Key}
				gidx++
				if err := stream.Send(kv); err != nil {
					return err
				}
			}

			// Found a match.
			if gidx < len(gkeys.Keys) && bytes.Compare(k, gkeys.Keys[gidx].Key) == 0 {
				t := gkeys.Keys[gidx]
				gidx++
				// Different keys would have the same prefix. So, check Checksum first,
				// it would be cheaper when there's no match.
				// TODO: What if checksum collides ?
				checksum := md5.Sum(v)
				if bytes.Equal(checksum[:], t.Checksum) {
					it.Next()
					continue
				}
			}
		}
		// TODO - Verify that schema key is deleted from follower after leader deletes it.

		// We just need to stream this kv. So, we can directly use the key
		// and val without any copying.
		kv := &protos.KV{
			Key:      k,
			Val:      v,
			UserMeta: []byte{iterItem.UserMeta()},
		}

		count++
		if err := stream.Send(kv); err != nil {
			return err
		}
		it.Next()
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
