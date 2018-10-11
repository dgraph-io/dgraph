/*
 * Copyright 2016-2018 Dgraph Labs, Inc. and Contributors
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

package worker

import (
	"errors"
	"io"
	"sync/atomic"

	"github.com/dgraph-io/badger"
	"github.com/golang/glog"
	"golang.org/x/net/trace"

	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
)

const (
	// MB represents a megabyte.
	MB = 1 << 20
)

// populateSnapshot gets data for a shard from the leader and writes it to BadgerDB on the follower.
func (n *node) populateSnapshot(ps *badger.DB, pl *conn.Pool) (int, error) {
	conn := pl.Get()
	c := pb.NewWorkerClient(conn)

	ctx := n.ctx
	snap, err := n.Snapshot()
	if err != nil {
		return 0, err
	}
	stream, err := c.StreamSnapshot(ctx, snap)
	if err != nil {
		return 0, err
	}
	// Before we write anything, we should drop all the data stored in ps.
	if err := ps.DropAll(); err != nil {
		return 0, err
	}

	// We can use count to check the number of posting lists returned in tests.
	count := 0
	writer := &x.TxnWriter{DB: ps, BlindWrite: true}
	for {
		kvs, err := stream.Recv()
		if err == io.EOF {
			glog.V(2).Infoln("EOF has been reached")
			break
		}
		if err != nil {
			return count, err
		}

		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}
		glog.V(2).Infof("Received a batch of %d keys. Total so far: %d\n", len(kvs.Kv), count)
		for _, kv := range kvs.Kv {
			count++
			var meta byte
			if len(kv.UserMeta) > 0 {
				meta = kv.UserMeta[0]
			}
			if err := writer.SetAt(kv.Key, kv.Val, meta, kv.Version); err != nil {
				return 0, err
			}
		}
	}
	if err := writer.Flush(); err != nil {
		return 0, err
	}
	glog.Infof("Populated snapshot with %d keys.\n", count)
	return count, nil
}

func (w *grpcWorker) StreamSnapshot(reqSnap *pb.Snapshot,
	stream pb.Worker_StreamSnapshotServer) error {
	n := groups().Node
	if n == nil {
		return conn.ErrNoNode
	}
	// Indicate that we're streaming right now. Used to cancel
	// calculateSnapshot.  However, this logic isn't foolproof. A leader might
	// have already proposed a snapshot, which it can apply while this streaming
	// is going on. That can happen after the reqSnap check we're doing below.
	// However, I don't think we need to tackle this edge case for now.
	atomic.AddInt32(&n.streaming, 1)
	defer atomic.AddInt32(&n.streaming, -1)

	if !x.IsTestRun() {
		if !n.AmLeader() {
			return errNotLeader
		}
	}
	snap, err := n.Snapshot()
	if err != nil {
		return err
	}
	x.Printf("Got StreamSnapshot request. Mine: %+v. Requested: %+v\n", snap, reqSnap)
	if snap.Index != reqSnap.Index || snap.ReadTs != reqSnap.ReadTs {
		return errors.New("Mismatching snapshot request")
	}

	// We have matched the requested snapshot with what this node, the leader of the group, has.
	// Now, we read all the posting lists stored below MinPendingStartTs, and stream them over the
	// wire. The MinPendingStartTs stored as part of creation of Snapshot, tells us the readTs to
	// use to read from the store. Anything below this timestamp, we should pick up.
	//
	// TODO: This would also pick up schema updates done "after" the snapshot index. Guess that
	// might be OK. Otherwise, we'd want to version the schemas as well. Currently, they're stored
	// at timestamp=1.

	// TODO: Confirm that the bug is now over.
	// BUG: There's a bug here due to which a node which doesn't see any transactions, but has real
	// data fails to send that over, because of min_ts.

	sl := streamLists{stream: stream, db: pstore}
	sl.chooseKey = func(_ *badger.Item) bool {
		// Pick all keys.
		return true
	}
	sl.itemToKv = func(key []byte, itr *badger.Iterator) (*pb.KV, error) {
		item := itr.Item()
		pk := x.Parse(key)
		if pk.IsSchema() {
			val, err := item.ValueCopy(nil)
			if err != nil {
				return nil, err
			}
			kv := &pb.KV{
				Key:      key,
				Val:      val,
				UserMeta: []byte{item.UserMeta()},
				Version:  item.Version(),
			}
			return kv, nil
		}
		// We should keep reading the posting list instead of copying the key-value pairs directly,
		// to consolidate the read logic in one place. This is more robust than trying to replicate
		// a simplified key-value copy logic here, which still understands the BitCompletePosting
		// and other bits about the way we store posting lists.
		l, err := posting.ReadPostingList(key, itr)
		if err != nil {
			return nil, err
		}
		return l.MarshalToKv()
	}

	if err := sl.orchestrate(stream.Context(), "Sending SNAPSHOT", snap.ReadTs); err != nil {
		return err
	}

	if tr, ok := trace.FromContext(stream.Context()); ok {
		tr.LazyPrintf("Sent keys. Done.\n")
	}
	return nil
}
