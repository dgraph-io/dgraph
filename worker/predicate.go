/*
* Copyright 2016 DGraph Labs, Inc.
*
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You may obtain a copy of the License at
*
*         http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
 */

package worker

import (
	"bytes"
	"context"
	"io"
	"sort"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

const (
	// MB represents a megabyte.
	MB = 1 << 20
)

// writeBatch performs a batch write of key value pairs to RocksDB.
func writeBatch(ctx context.Context, kv chan *task.KV, che chan error) {
	wb := pstore.NewWriteBatch()
	defer wb.Destroy()

	batchSize := 0
	batchWriteNum := 1
	for i := range kv {
		wb.Put(i.Key, i.Val)
		batchSize += len(i.Key) + len(i.Val)
		// We write in batches of size 32MB.
		if batchSize >= 32*MB {
			x.Trace(ctx, "SNAPSHOT: Doing batch write num: %d", batchWriteNum)
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
		x.Trace(ctx, "Doing batch write %d.", batchWriteNum)
		che <- pstore.WriteBatch(wb)
		return
	}
	che <- nil
}

func streamKeys(stream Worker_PredicateAndSchemaDataClient, groupId uint32) error {
	it := pstore.NewIterator()
	defer it.Close()

	g := &task.GroupKeys{
		GroupId: groupId,
	}

	for it.SeekToFirst(); it.Valid(); it.Next() {
		k, v := it.Key(), it.Value()
		pk := x.Parse(k.Data())

		if pk == nil {
			continue
		}
		// No need to send KC for schema keys
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

		var pl types.PostingList
		x.Check(pl.Unmarshal(v.Data()))

		kdup := make([]byte, len(k.Data()))
		copy(kdup, k.Data())
		key := &task.KC{
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
	c := NewWorkerClient(conn)

	stream, err := c.PredicateAndSchemaData(context.Background())
	if err != nil {
		x.TraceError(ctx, err)
		return 0, err
	}
	x.Trace(ctx, "Streaming data for group: %v", group)

	if err := streamKeys(stream, group); err != nil {
		x.TraceError(ctx, err)
		return 0, x.Wrapf(err, "While streaming keys group")
	}

	kvs := make(chan *task.KV, 1000)
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
			x.TraceError(ctx, err)
			close(kvs)
			return count, err
		}
		count++

		// We check for errors, if there are no errors we send value to channel.
		select {
		case kvs <- kv:
			// OK
		case <-ctx.Done():
			x.TraceError(ctx, x.Errorf("Context timed out while streaming group: %v", group))
			close(kvs)
			return count, ctx.Err()
		case err := <-che:
			x.TraceError(ctx, x.Errorf("Error while doing a batch write for group: %v", group))
			close(kvs)
			return count, err
		}
	}
	close(kvs)

	if err := <-che; err != nil {
		x.TraceError(ctx, x.Errorf("Error while doing a batch write for group: %v", group))
		return count, err
	}
	x.Trace(ctx, "Streaming complete for group: %v", group)
	return count, nil
}

// PredicateAndSchemaData can be used to return data corresponding to a predicate over
// a stream.
func (w *grpcWorker) PredicateAndSchemaData(stream Worker_PredicateAndSchemaDataServer) error {
	gkeys := &task.GroupKeys{}

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
	x.Trace(stream.Context(), "Got %d keys from client\n", len(gkeys.Keys))

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
			var pl types.PostingList
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
		kv := &task.KV{
			Key: k.Data(),
			Val: v.Data(),
		}

		count++
		if err := stream.Send(kv); err != nil {
			return err
		}
	} // end of iterator
	x.Trace(stream.Context(), "Sent %d keys to client. Done.\n", count)

	if err := it.Err(); err != nil {
		return err
	}
	return nil
}
