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
	"sort"

	"golang.org/x/net/trace"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
)

const (
	// MB represents a megabyte.
	MB = 1 << 20
)

// writeBatch performs a batch write of key value pairs to RocksDB.
func writeBatch(ctx context.Context, kv chan *protos.KV, che chan error) {
	wb := pstore.NewWriteBatch()
	defer wb.Destroy()

	batchSize := 0
	batchWriteNum := 1
	for i := range kv {
		wb.Put(i.Key, i.Val)
		batchSize += len(i.Key) + len(i.Val)
		// We write in batches of size 32MB.
		if batchSize >= 32*MB {
			if tr, ok := trace.FromContext(ctx); ok {
				tr.LazyPrintf("SNAPSHOT: Doing batch write num: %d", batchWriteNum)
			}
			if err := pstore.WriteBatch(wb); err != nil {
				che <- err
				return
			}

			batchWriteNum++
			// Resetting batch size after a batch write.
			batchSize = 0
			// Since we are writing data in batches, we need to clear up items enqueued
			// for batch write after every successful write.
			wb.Clear()
		}
	}
	// After channel is closed the above loop would exit, we write the data in
	// write batch here.
	if batchSize > 0 {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Doing batch write %d.", batchWriteNum)
		}
		che <- pstore.WriteBatch(wb) // Returning, wb will be destroyed. No need to clear.
		return
	}
	che <- nil
}

func streamKeys(stream protos.Worker_PredicateAndSchemaDataClient, groupId uint32) error {
	it := pstore.NewIterator()
	defer it.Close()

	g := &protos.GroupKeys{
		GroupId: groupId,
	}

	for it.SeekToFirst(); it.Valid(); it.Next() {
		k, v := it.Key(), it.Value()
		pk := x.Parse(k.Data())

		if pk == nil {
			continue
		}
		// No need to send KC for schema keys, since we won't save anything
		// by sending checksum of schema key
		if pk.IsSchema() {
			it.Seek(pk.SkipSchema())
			it.Prev()
			continue
		}
		if group.BelongsTo(pk.Attr) != g.GroupId {
			it.Seek(pk.SkipPredicate())
			it.Prev() // To tackle it.Next() called by default.
			continue
		}

		var pl protos.PostingList
		x.Check(pl.Unmarshal(v.Data()))

		kdup := make([]byte, len(k.Data()))
		copy(kdup, k.Data())
		key := &protos.KC{
			Key:      kdup,
			Checksum: pl.Checksum,
		}
		g.Keys = append(g.Keys, key)
		if len(g.Keys) >= 1000 {
			if err := stream.Send(g); err != nil {
				return x.Wrapf(err, "While sending group keys to server.")
			}
			g.Keys = g.Keys[:0]
		}
	}
	if err := stream.Send(g); err != nil {
		return x.Wrapf(err, "While sending group keys to server.")
	}
	return stream.CloseSend()
}

// PopulateShard gets data for predicate pred from server with id serverId and
// writes it to RocksDB.
func populateShard(ctx context.Context, pl *pool, group uint32) (int, error) {
	conn, err := pl.Get()
	if err != nil {
		return 0, err
	}
	defer pl.Put(conn)
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

	if err := streamKeys(stream, group); err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf(err.Error())
		}
		return 0, x.Wrapf(err, "While streaming keys group")
	}

	kvs := make(chan *protos.KV, 1000)
	che := make(chan error)
	go writeBatch(ctx, kvs, che)

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
				tr.LazyPrintf(fmt.Sprintf("Context timed out while streaming group: %v", group))
			}
			close(kvs)
			return count, ctx.Err()
		case err := <-che:
			if tr, ok := trace.FromContext(ctx); ok {
				tr.LazyPrintf(fmt.Sprintf("Error while doing a batch write for group: %v", group))
			}
			close(kvs)
			return count, err
		}
	}
	close(kvs)

	if err := <-che; err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf(fmt.Sprintf("Error while doing a batch write for group: %v", group))
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

	if !groups().ServesGroup(gkeys.GroupId) {
		return x.Errorf("Group %d not served.", gkeys.GroupId)
	}
	n := groups().Node(gkeys.GroupId)
	if !n.AmLeader() {
		return x.Errorf("Not leader of group: %d", gkeys.GroupId)
	}

	// TODO(pawan) - Shift to CheckPoints once we figure out how to add them to the
	// RocksDB library we are using.
	// http://rocksdb.org/blog/2609/use-checkpoints-for-efficient-snapshots/
	it := pstore.NewIterator()
	defer it.Close()

	var count int
	for it.SeekToFirst(); it.Valid(); it.Next() {
		k, v := it.Key(), it.Value()
		pk := x.Parse(k.Data())

		if pk == nil {
			continue
		}
		if group.BelongsTo(pk.Attr) != gkeys.GroupId && !pk.IsSchema() {
			it.Seek(pk.SkipPredicate())
			it.Prev() // To tackle it.Next() called by default.
			continue
		} else if group.BelongsTo(pk.Attr) != gkeys.GroupId {
			continue
		}

		// No checksum check for schema keys
		if !pk.IsSchema() {
			var pl protos.PostingList
			x.Check(pl.Unmarshal(v.Data()))

			idx := sort.Search(len(gkeys.Keys), func(i int) bool {
				t := gkeys.Keys[i]
				return bytes.Compare(k.Data(), t.Key) <= 0
			})

			if idx < len(gkeys.Keys) {
				// Found a match.
				t := gkeys.Keys[idx]
				// Different keys would have the same prefix. So, check Checksum first,
				// it would be cheaper when there's no match.
				if bytes.Equal(pl.Checksum, t.Checksum) && bytes.Equal(k.Data(), t.Key) {
					// No need to send this.
					continue
				}
			}
		}

		// We just need to stream this kv. So, we can directly use the key
		// and val without any copying.
		kv := &protos.KV{
			Key: k.Data(),
			Val: v.Data(),
		}

		count++
		if err := stream.Send(kv); err != nil {
			return err
		}
	} // end of iterator
	if tr, ok := trace.FromContext(stream.Context()); ok {
		tr.LazyPrintf("Sent %d keys to client. Done.\n", count)
	}

	if err := it.Err(); err != nil {
		return err
	}
	return nil
}
