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
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/coreos/etcd/raft/raftpb"
	"github.com/dgryski/go-farm"
	"github.com/google/flatbuffers/go"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"

	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/x"
)

// State stores the worker state.
type State struct {
	dataStore    *store.Store
	uidStore     *store.Store
	instanceIdx  uint64
	numInstances uint64
	// pools stores the pool for all the instances which is then used to send queries
	// and mutations to the appropriate instance.
	pools      []*Pool
	poolsMutex sync.RWMutex
}

// Stores the worker state.
var ws *State

// SetWorkerState sets the global worker state to the given state.
func SetWorkerState(s *State) {
	x.Assert(s != nil)
	ws = s
}

// NewState initializes the state on an instance with data,uid store and other meta.
func NewState(ps, uStore *store.Store, idx, numInst uint64) *State {
	return &State{
		dataStore:    ps,
		uidStore:     uStore,
		instanceIdx:  idx,
		numInstances: numInst,
	}
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

// GetOrAssign is used to get uids for a set of xids by communicating with other workers.
func (w *grpcWorker) GetOrAssign(ctx context.Context, query *Payload) (*Payload, error) {
	uo := flatbuffers.GetUOffsetT(query.Data)
	xids := new(task.XidList)
	xids.Init(query.Data, uo)

	if ws.instanceIdx != 0 {
		log.Fatalf("instanceIdx: %v. GetOrAssign. We shouldn't be getting this req", ws.instanceIdx)
	}

	reply := new(Payload)
	var rerr error
	reply.Data, rerr = getOrAssignUids(ctx, xids)
	return reply, rerr
}

// Mutate is used to apply mutations over the network on other instances.
func (w *grpcWorker) Mutate(ctx context.Context, query *Payload) (*Payload, error) {
	m := new(Mutations)
	if err := m.Decode(query.Data); err != nil {
		return nil, err
	}

	left := new(Mutations)
	if err := mutate(ctx, m, left); err != nil {
		return nil, err
	}

	reply := new(Payload)
	var rerr error
	reply.Data, rerr = left.Encode()
	return reply, rerr
}

// ServeTask is used to respond to a query.
func (w *grpcWorker) ServeTask(ctx context.Context, query *Payload) (*Payload, error) {
	q := new(task.Query)
	x.ParseTaskQuery(q, query.Data)
	attr := string(q.Attr())
	x.Trace(ctx, "Attribute: %v NumUids: %v InstanceIdx: %v ServeTask",
		attr, q.UidsLength(), ws.instanceIdx)

	reply := new(Payload)
	var rerr error
	// Request for xid <-> uid conversion should be handled by instance 0 and all
	// other requests should be handled according to modulo sharding of the predicate.
	chosenInstance := farm.Fingerprint64([]byte(attr)) % ws.numInstances
	if (ws.instanceIdx == 0 && attr == _xid_) || chosenInstance == ws.instanceIdx {
		reply.Data, rerr = processTask(query.Data)
	} else {
		log.Fatalf("attr: %v instanceIdx: %v Request sent to wrong server.", attr, ws.instanceIdx)
	}
	return reply, rerr
}

func (w *grpcWorker) RaftMessage(ctx context.Context, query *Payload) (*Payload, error) {
	p, ok := peer.FromContext(ctx)
	fmt.Printf("PEER for RAFTMESSAGE: %v %v\n", p.Addr.String(), ok)

	reply := &Payload{}
	reply.Data = []byte("ok")
	msg := raftpb.Message{}
	if err := msg.Unmarshal(query.Data); err != nil {
		return reply, err
	}
	fmt.Printf("Received RAFT Message: %+v\n", msg)
	GetNode().Step(ctx, msg)
	return reply, nil
}

// PredicateData can be used to return data corresponding to a predicate over
// a stream.
func (w *grpcWorker) PredicateData(query *Payload, stream Worker_PredicateDataServer) error {
	qp := query.Data
	prefix := append(qp, '|')
	// TODO(pawan) - Shift to CheckPoints once we figure out how to add them to the
	// RocksDB library we are using.
	// http://rocksdb.org/blog/2609/use-checkpoints-for-efficient-snapshots/
	it := ws.dataStore.NewIterator()
	defer it.Close()

	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		k, v := it.Key(), it.Value()

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
func runServer(port string) {
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

// Connect establishes a connection with other workers and sends the Hello rpc
// call to them.
func (s *State) Connect(workerList []string, workerPort string) {
	go runServer(workerPort)

	for _, addr := range workerList {
		if len(addr) == 0 {
			continue
		}

		pool := NewPool(addr, 5)
		query := new(Payload)
		query.Data = []byte("hello")

		conn, err := pool.Get()
		if err != nil {
			log.Fatalf("Unable to connect: %v", err)
		}

		c := NewWorkerClient(conn)
		_, err = c.Hello(context.Background(), query)
		if err != nil {
			log.Fatalf("Unable to connect: %v", err)
		}
		_ = pool.Put(conn)

		s.poolsMutex.Lock()
		s.pools = append(s.pools, pool)
		s.poolsMutex.Unlock()
	}

	log.Print("Server started. Clients connected.")
}

// GetPool returns pool for given index.
func (s *State) GetPool(k int) *Pool {
	s.poolsMutex.RLock()
	defer s.poolsMutex.RUnlock()
	return s.pools[k]
}
