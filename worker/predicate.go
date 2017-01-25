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

	"github.com/boltdb/bolt"
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
	if err := pstore.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("data"))
		for i := range kv {
			if err := b.Put(i.Key, i.Val); err != nil {
				return err
			}
		}
		x.Trace(ctx, "Doing batch write.")
		return nil
	}); err != nil {
		che <- err
		return
	}
	che <- nil
}

func streamKeys(stream Worker_PredicateDataClient, groupId uint32) error {
	g := &task.GroupKeys{
		GroupId: groupId,
	}

	pstore.View(func(tx *bolt.Tx) error {
		c := tx.Bucket([]byte("data")).Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			pk := x.Parse(k)

			if pk == nil {
				continue
			}
			if group.BelongsTo(pk.Attr) != g.GroupId {
				c.Seek(pk.SkipPredicate())
				c.Prev() // To tackle it.Next() called by default.
				continue
			}

			var pl types.PostingList
			x.Check(pl.Unmarshal(v))

			kdup := make([]byte, len(k))
			copy(kdup, k)
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
		return nil
	})
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

	stream, err := c.PredicateData(context.Background())
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

// PredicateData can be used to return data corresponding to a predicate over
// a stream.
func (w *grpcWorker) PredicateData(stream Worker_PredicateDataServer) error {
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

	var count int
	if err := pstore.View(func(tx *bolt.Tx) error {
		c := tx.Bucket([]byte("data")).Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			pk := x.Parse(k)

			if pk == nil {
				continue
			}
			if group.BelongsTo(pk.Attr) != gkeys.GroupId {
				c.Seek(pk.SkipPredicate())
				c.Prev() // To tackle it.Next() called by default.
				continue
			}

			var pl types.PostingList
			x.Check(pl.Unmarshal(v))

			idx := sort.Search(len(gkeys.Keys), func(i int) bool {
				t := gkeys.Keys[i]
				return bytes.Compare(k, t.Key) <= 0
			})

			if idx < len(gkeys.Keys) {
				// Found a match.
				t := gkeys.Keys[idx]
				// Different keys would have the same prefix. So, check Checksum first,
				// it would be cheaper when there's no match.
				if bytes.Equal(pl.Checksum, t.Checksum) && bytes.Equal(k, t.Key) {
					// No need to send this.
					continue
				}
			}

			// We just need to stream this kv. So, we can directly use the key
			// and val without any copying.
			kv := &task.KV{
				Key: k,
				Val: v,
			}

			count++
			if err := stream.Send(kv); err != nil {
				return err
			}
		} // end of iterator
		return nil
	}); err != nil {
		return err
	}
	x.Trace(stream.Context(), "Sent %d keys to client. Done.\n", count)

	return nil
}
