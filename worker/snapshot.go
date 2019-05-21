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

	bpb "github.com/dgraph-io/badger/pb"
	"github.com/golang/glog"
	"go.etcd.io/etcd/raft"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
)

const (
	// MB represents a megabyte.
	MB = 1 << 20
)

type badgerWriter interface {
	Write(kvs *bpb.KVList) error
	Flush() error
}

// populateSnapshot gets data for a shard from the leader and writes it to BadgerDB on the follower.
func (n *node) populateSnapshot(snap pb.Snapshot, pl *conn.Pool) (int, error) {
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

	var writer badgerWriter
	if snap.SinceTs == 0 {
		sw := pstore.NewStreamWriter()
		if err := sw.Prepare(); err != nil {
			return 0, err
		}

		writer = sw
	} else {
		writer = posting.NewTxnWriter(pstore)
	}

	// We can use count to check the number of posting lists returned in tests.
	count := 0
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
		if err := writer.Write(&bpb.KVList{Kv: kvs.Kv}); err != nil {
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

func doStreamSnapshot(snap *pb.Snapshot, out pb.Worker_StreamSnapshotServer) error {
	// We choose not to try and match the requested snapshot from the latest snapshot at the leader.
	// This is the job of the Raft library. At the leader end, we service whatever is asked of us.
	// If this snapshot is old, Raft should cause the follower to request another one, to overwrite
	// the data from this one.
	//
	// Snapshot request contains the txn read timestamp to be used to get a consistent snapshot of
	// the data. This is what we use in orchestrate.
	//
	// Note: This would also pick up schema updates done "after" the snapshot index. Guess that
	// might be OK. Otherwise, we'd want to version the schemas as well. Currently, they're stored
	// at timestamp=1.

	// We no longer check if this node is the leader, because the leader can switch between snapshot
	// requests. Therefore, we wait until this node has reached snap.ReadTs, before servicing the
	// request. Any other node in the group should have the same data as the leader, once it is past
	// the read timestamp.
	glog.Infof("Waiting to reach timestamp: %d", snap.ReadTs)
	if err := posting.Oracle().WaitForTs(out.Context(), snap.ReadTs); err != nil {
		return err
	}

	var num int
	stream := pstore.NewStreamAt(snap.ReadTs)
	stream.LogPrefix = "Sending Snapshot"
	// Use the default implementation. We no longer try to generate a rolled up posting list here.
	// Instead, we just stream out all the versions as they are.
	stream.KeyToList = nil
	stream.Send = func(list *bpb.KVList) error {
		kvs := &pb.KVS{Kv: list.Kv}
		num += len(kvs.Kv)
		return out.Send(kvs)
	}
	stream.ChooseKey = func(item *badger.Item) bool {
		return item.Version() >= snap.SinceTs
	}

	if err := stream.Orchestrate(out.Context()); err != nil {
		return err
	}

	// Indicate that sending is done.
	if err := out.Send(&pb.KVS{Done: true}); err != nil {
		return err
	}

	glog.Infof("Streaming done. Sent %d entries. Waiting for ACK...", num)
	ack, err := out.Recv()
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

	snap, err := stream.Recv()
	if err != nil {
		// If we don't even receive a request (here or if no StreamSnapshot is called), we can't
		// report the snapshot to be a failure, because we don't know which follower is asking for
		// one. Technically, I (the leader) can figure out from my Raft state, but I (Manish) think
		// this is such a rare scenario, that we don't need to build a mechanism to track that. If
		// we see it in the wild, we could add a timeout based mechanism to receive this request,
		// but timeouts are always hard to get right.
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
