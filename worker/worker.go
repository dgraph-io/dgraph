/*
 * Copyright 2016 Dgraph Labs, Inc.
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

package worker

import (
	"flag"
	"log"
	"net"

	"google.golang.org/grpc"

	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgryski/go-farm"
	"github.com/google/flatbuffers/go"
)

var workerPort = flag.String("workerport", ":12345",
	"Port used by worker for internal communication.")

var dataStore, uidStore *store.Store
var pools []*Pool
var numInstances, instanceIdx uint64

func Init(ps, uStore *store.Store, idx, numInst uint64) {
	dataStore = ps
	uidStore = uStore
	instanceIdx = idx
	numInstances = numInst
}

func NewQuery(attr string, uids []uint64) []byte {
	b := flatbuffers.NewBuilder(0)
	task.QueryStartUidsVector(b, len(uids))
	for i := len(uids) - 1; i >= 0; i-- {
		b.PrependUint64(uids[i])
	}
	vend := b.EndVector(len(uids))

	ao := b.CreateString(attr)
	task.QueryStart(b)
	task.QueryAddAttr(b, ao)
	task.QueryAddUids(b, vend)
	qend := task.QueryEnd(b)
	b.Finish(qend)
	return b.Bytes[b.Head():]
}

type worker struct{}

func (w *worker) Hello(ctx context.Context, in *Payload) (*Payload, error) {
	out := new(Payload)
	if string(in.Data) == "hello" {
		out.Data = []byte("Oh hello there!")
	} else {
		out.Data = []byte("Hey stranger!")
	}

	return out, nil
}

func (w *worker) GetOrAssign(ctx context.Context, query *Payload) (*Payload, error) {
	uo := flatbuffers.GetUOffsetT(query.Data)
	xids := new(task.XidList)
	xids.Init(query.Data, uo)

	if instanceIdx != 0 {
		log.Fatalf("instanceIdx: %v. GetOrAssign. We shouldn't be getting this req", instanceIdx)
	}

	reply := new(Payload)
	var rerr error
	reply.Data, rerr = getOrAssignUids(ctx, xids)
	return reply, rerr
}

func (w *worker) Mutate(ctx context.Context, query *Payload) (*Payload, error) {
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

func (w *worker) ServeTask(ctx context.Context, query *Payload) (*Payload, error) {
	uo := flatbuffers.GetUOffsetT(query.Data)
	q := new(task.Query)
	q.Init(query.Data, uo)
	attr := string(q.Attr())
	x.Trace(ctx, "Attribute: %v NumUids: %v InstanceIdx: %v ServeTask",
		attr, q.UidsLength(), instanceIdx)

	reply := new(Payload)
	var rerr error
	if (instanceIdx == 0 && attr == "_xid_") ||
		farm.Fingerprint64([]byte(attr))%numInstances == instanceIdx {

		reply.Data, rerr = processTask(query.Data)

	} else {
		log.Fatalf("attr: %v instanceIdx: %v Request sent to wrong server.", attr, instanceIdx)
	}
	return reply, rerr
}

func runServer(port string) {
	ln, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("While running server: %v", err)
		return
	}
	log.Printf("Worker listening at address: %v", ln.Addr())

	s := grpc.NewServer(grpc.CustomCodec(&PayloadCodec{}))
	RegisterWorkerServer(s, &worker{})
	s.Serve(ln)
}

func Connect(workerList []string) {
	go runServer(*workerPort)

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

		pools = append(pools, pool)
	}

	log.Print("Server started. Clients connected.")
}
