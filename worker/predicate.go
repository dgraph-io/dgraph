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
	"sync/atomic"
	"time"

	"github.com/coreos/etcd/raft"
	"github.com/dgraph-io/badger"
	"github.com/golang/glog"

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
func (n *node) populateSnapshot(snap pb.Snapshot, ps *badger.DB, pl *conn.Pool) (int, error) {
	conn := pl.Get()
	c := pb.NewWorkerClient(conn)

	// Set my RaftContext on the snapshot, so it's easier to locate me.
	ctx := n.ctx
	snap.Context = n.RaftContext
	stream, err := c.StreamSnapshot(ctx)
	if err != nil {
		return 0, err
	}
	if err := stream.Send(&snap); err != nil {
		return 0, err
	}
	// Before we write anything, we should drop all the data stored in ps.
	if err := ps.DropAll(); err != nil {
		return 0, err
	}

	// TODO HACK
	// Let's add artificial delay, so we can test this failure.
	glog.Infof("Sleeping for 15s...")
	time.Sleep(15 * time.Second)
	glog.Infof("Woke up")

	// We can use count to check the number of posting lists returned in tests.
	count := 0
	writer := x.NewTxnWriter(ps)
	writer.BlindWrite = true // Do overwrite keys.
	for {
		kvs, err := stream.Recv()
		if err != nil {
			return count, err
		}
		if kvs.Done {
			glog.V(1).Infoln("All key-values have been received.")
			break
		}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}

		glog.V(1).Infof("Received a batch of %d keys. Total so far: %d\n", len(kvs.Kv), count)
		if err := writer.Send(kvs); err != nil {
			return 0, err
		}
		count += len(kvs.Kv)
	}
	if err := writer.Flush(); err != nil {
		return 0, err
	}
	glog.Infof("Snapshot writes DONE. Sending ACK")
	// Send an acknowledgement back to the leader.
	if err := stream.Send(&pb.Snapshot{Done: true}); err != nil {
		return 0, err
	}
	glog.Infof("Populated snapshot with %d keys.\n", count)
	return count, nil
}

func doStreamSnapshot(snap *pb.Snapshot, stream pb.Worker_StreamSnapshotServer) error {
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

	var numKeys uint64
	sl := streamLists{stream: stream, db: pstore}
	sl.chooseKey = func(_ *badger.Item) bool {
		// Pick all keys.
		return true
	}
	sl.itemToKv = func(key []byte, itr *badger.Iterator) (*pb.KV, error) {
		atomic.AddUint64(&numKeys, 1)
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
	// Indicate that sending is done.
	if err := stream.Send(&pb.KVS{Done: true}); err != nil {
		return err
	}
	glog.Infof("Key streaming done. Sent %d keys. Waiting for ACK...", numKeys)
	ack, err := stream.Recv()
	if err != nil {
		return err
	}
	glog.Infof("Received ACK with done: %v\n", ack.Done)
	return nil
}

func (w *grpcWorker) StreamSnapshot(stream pb.Worker_StreamSnapshotServer) error {
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
	snap, err := stream.Recv()
	if err != nil {
		return err
	}
	glog.Infof("Got StreamSnapshot request: %+v\n", snap)
	if err := doStreamSnapshot(snap, stream); err != nil {
		glog.Errorf("While streaming snapshot: %v. Reporting failure.", err)
		n.Raft().ReportSnapshot(snap.Context.GetId(), raft.SnapshotFailure)
		return err
	}
	glog.Infof("Stream snapshot: OK")
	return nil
}
