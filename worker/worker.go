/*
 * Copyright 2016 DGraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Package worker contains code for internal worker communication to perform
// queries and mutations.
package worker

import (
	"bytes"
	"log"
	"net"
	"sort"
	"sync"

	"github.com/coreos/etcd/raft/raftpb"
	"github.com/google/flatbuffers/go"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/posting/types"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/x"
)

// State stores the worker state.
type State struct {
	dataStore *store.Store

	// TODO: Remove this code once RAFT groups are in place.
	// pools stores the pool for all the instances which is then used to send queries
	// and mutations to the appropriate instance.
	pools      []*pool
	poolsMutex sync.RWMutex
}

// Stores the worker state.
var ws *State

// NewState initializes the state on an instance with data,uid store and other meta.
func SetState(ps *store.Store) *State {
	ws = &State{
		dataStore: ps,
	}
	return ws
}

// NewQuery creates a Query flatbuffer table, serializes and returns it.
func NewQuery(attr string, uids []uint64, terms []string) []byte {
	b := flatbuffers.NewBuilder(0)

	x.Assert(uids == nil || terms == nil)

	var vend flatbuffers.UOffsetT
	if uids != nil {
		task.QueryStartUidsVector(b, len(uids))
		for i := len(uids) - 1; i >= 0; i-- {
			b.PrependUint64(uids[i])
		}
		vend = b.EndVector(len(uids))
	} else {
		offsets := make([]flatbuffers.UOffsetT, 0, len(terms))
		for _, term := range terms {
			uo := b.CreateString(term)
			offsets = append(offsets, uo)
		}
		task.QueryStartTermsVector(b, len(terms))
		for i := len(terms) - 1; i >= 0; i-- {
			b.PrependUOffsetT(offsets[i])
		}
		vend = b.EndVector(len(terms))
	}

	ao := b.CreateString(attr)
	task.QueryStart(b)
	task.QueryAddAttr(b, ao)
	if uids != nil {
		task.QueryAddUids(b, vend)
	} else {
		task.QueryAddTerms(b, vend)
	}
	qend := task.QueryEnd(b)
	b.Finish(qend)
	return b.Bytes[b.Head():]
}

// grpcWorker struct implements the gRPC server interface.
type grpcWorker struct{}

// Hello rpc call is used to check connection with other workers after worker
// tcp server for this instance starts.
func (w *grpcWorker) Hello(ctx context.Context, in *Payload) (*Payload, error) {
	out := new(Payload)
	if string(in.Data) == "hello" {
		out.Data = []byte("Oh hello there!")
	} else {
		out.Data = []byte("Hey stranger!")
	}

	return out, nil
}

func (w *grpcWorker) RaftMessage(ctx context.Context, query *Payload) (*Payload, error) {
	reply := &Payload{}
	reply.Data = []byte("ok")
	msg := raftpb.Message{}
	if err := msg.Unmarshal(query.Data); err != nil {
		return reply, err
	}

	rc := task.GetRootAsRaftContext(msg.Context, 0)
	node := groups().Node(rc.Group())
	node.Connect(msg.From, string(rc.Addr()))
	node.Step(ctx, msg)

	return reply, nil
}

func (w *grpcWorker) JoinCluster(ctx context.Context, query *Payload) (*Payload, error) {
	reply := &Payload{}
	reply.Data = []byte("ok")
	if len(query.Data) == 0 {
		return reply, x.Errorf("JoinCluster: No data provided")
	}
	rc := task.GetRootAsRaftContext(query.Data, 0)
	node := groups().Node(rc.Group())
	node.Connect(rc.Id(), string(rc.Addr()))
	node.AddToCluster(rc.Id())
	return reply, nil
}

// PredicateData can be used to return data corresponding to a predicate over
// a stream.
func (w *grpcWorker) PredicateData(query *Payload, stream Worker_PredicateDataServer) error {
	var group task.GroupKeys
	uo := flatbuffers.GetUOffsetT(query.Data)
	group.Init(query.Data, uo)
	_ = group.Groupid()

	// TODO(pawan) - Shift to CheckPoints once we figure out how to add them to the
	// RocksDB library we are using.
	// http://rocksdb.org/blog/2609/use-checkpoints-for-efficient-snapshots/
	it := ws.dataStore.NewIterator()
	defer it.Close()

	for it.SeekToFirst(); it.Valid(); it.Next() {
		k, v := it.Key(), it.Value()
		pl := types.GetRootAsPostingList(v.Data(), 0)

		// TODO: Check that key is part of the specified group id.
		i := sort.Search(group.KeysLength(), func(i int) bool {
			var t task.KC
			x.Assertf(group.Keys(&t, i), "Unable to parse task.KC")
			return bytes.Compare(k.Data(), t.KeyBytes()) <= 0
		})

		if i < group.KeysLength() {
			// Found a match.
			var t task.KC
			x.Assertf(group.Keys(&t, i), "Unable to parse task.KC")

			if bytes.Equal(k.Data(), t.KeyBytes()) && bytes.Equal(pl.Checksum(), t.ChecksumBytes()) {
				// No need to send this.
				continue
			}
		}

		b := flatbuffers.NewBuilder(0)
		bko := b.CreateByteVector(k.Data())
		bvo := b.CreateByteVector(v.Data())
		task.KVStart(b)
		task.KVAddKey(b, bko)
		task.KVAddVal(b, bvo)
		kvoffset := task.KVEnd(b)
		b.Finish(kvoffset)

		p := Payload{Data: b.Bytes[b.Head():]}
		if err := stream.Send(&p); err != nil {
			return err
		}
		k.Free()
		v.Free()
	}
	if err := it.Err(); err != nil {
		return err
	}
	return nil
}

// runServer initializes a tcp server on port which listens to requests from
// other workers for internal communication.
func RunServer(port string) {
	ln, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("While running server: %v", err)
		return
	}
	log.Printf("Worker listening at address: %v", ln.Addr())

	s := grpc.NewServer(grpc.CustomCodec(&PayloadCodec{}))
	RegisterWorkerServer(s, &grpcWorker{})
	s.Serve(ln)
}
