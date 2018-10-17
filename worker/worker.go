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

// Package worker contains code for pb.worker communication to perform
// queries and mutations.
package worker

import (
	"fmt"
	"log"
	"math"
	"net"
	"sync"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/dgraphee/backup"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

var (
	pstore           *badger.DB
	workerServer     *grpc.Server
	raftServer       conn.RaftServer
	pendingProposals chan struct{}
	// In case of flaky network connectivity we would try to keep upto maxPendingEntries in wal
	// so that the nodes which have lagged behind leader can just replay entries instead of
	// fetching snapshot if network disconnectivity is greater than the interval at which snapshots
	// are taken
)

func workerPort() int {
	return x.Config.PortOffset + x.PortInternal
}

func Init(ps *badger.DB) {
	pstore = ps
	// needs to be initialized after group config
	pendingProposals = make(chan struct{}, Config.NumPendingProposals)
	workerServer = grpc.NewServer(
		grpc.MaxRecvMsgSize(x.GrpcMaxSize),
		grpc.MaxSendMsgSize(x.GrpcMaxSize),
		grpc.MaxConcurrentStreams(math.MaxInt32))
}

// grpcWorker struct implements the gRPC server interface.
type grpcWorker struct {
	sync.Mutex
	reqids map[uint64]bool
}

// addIfNotPresent returns false if it finds the reqid already present.
// Otherwise, adds the reqid in the list, and returns true.
func (w *grpcWorker) addIfNotPresent(reqid uint64) bool {
	w.Lock()
	defer w.Unlock()
	if w.reqids == nil {
		w.reqids = make(map[uint64]bool)
	} else if _, has := w.reqids[reqid]; has {
		return false
	}
	w.reqids[reqid] = true
	return true
}

// RunServer initializes a tcp server on port which listens to requests from
// other workers for pb.communication.
func RunServer(bindall bool) {
	laddr := "localhost"
	if bindall {
		laddr = "0.0.0.0"
	}
	var err error
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", laddr, workerPort()))
	if err != nil {
		log.Fatalf("While running server: %v", err)
	}
	x.Printf("Worker listening at address: %v", ln.Addr())

	pb.RegisterWorkerServer(workerServer, &grpcWorker{})
	pb.RegisterRaftServer(workerServer, &raftServer)
	workerServer.Serve(ln)
}

// StoreStats returns stats for data store.
func StoreStats() string {
	return "Currently no stats for badger"
}

// BlockingStop stops all the nodes, server between other workers and syncs all marks.
func BlockingStop() {
	log.Println("Stopping group...")
	groups().closer.SignalAndWait()

	log.Println("Stopping node...")
	groups().Node.closer.SignalAndWait()

	log.Printf("Stopping worker server...")
	workerServer.Stop()

	// TODO: What is this for?
	posting.StopLRUEviction()
}

// Backup ...
func (s *Server) Backup(ctx context.Context, req *pb.BackupRequest) (*pb.BackupResponse, error) {
	if req.StartTs == 0 {
		req.StartTs = State.getTimestamp(true)
	}
	// resp.Txn = &api.TxnContext{
	// 	StartTs: req.StartTs,
	// }
	backupRequest = &backup.Request{
		ReadTs: req.StartTs,
	}
	if err := backupRequest.Process(ctx); err != nil {
		return nil, err
	}
	return &pb.BackupResponse{}, nil
}
