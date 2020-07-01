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
	"sync/atomic"

	"github.com/dgraph-io/badger/v2"
	badgerpb "github.com/dgraph-io/badger/v2/pb"
	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
	"go.opencensus.io/plugin/ocgrpc"

	"github.com/golang/glog"
	"google.golang.org/grpc"
)

var (
	pstore       *badger.DB
	workerServer *grpc.Server
	raftServer   conn.RaftServer

	// In case of flaky network connectivity we would try to keep upto maxPendingEntries in wal
	// so that the nodes which have lagged behind leader can just replay entries instead of
	// fetching snapshot if network disconnectivity is greater than the interval at which snapshots
	// are taken
)

func workerPort() int {
	return x.Config.PortOffset + x.PortInternal
}

// Init initializes this package.
func Init(ps *badger.DB) {
	pstore = ps
	// needs to be initialized after group config
	limiter = rateLimiter{c: sync.NewCond(&sync.Mutex{}), max: x.WorkerConfig.NumPendingProposals}
	go limiter.bleed()

	workerServer = grpc.NewServer(
		grpc.MaxRecvMsgSize(x.GrpcMaxSize),
		grpc.MaxSendMsgSize(x.GrpcMaxSize),
		grpc.MaxConcurrentStreams(math.MaxInt32),
		grpc.StatsHandler(&ocgrpc.ServerHandler{}))
}

// grpcWorker struct implements the gRPC server interface.
type grpcWorker struct {
	sync.Mutex
}

func (w *grpcWorker) Subscribe(
	req *pb.SubscriptionRequest, stream pb.Worker_SubscribeServer) error {
	// Subscribe on given prefixes.
	return pstore.Subscribe(stream.Context(), func(kvs *badgerpb.KVList) error {
		return stream.Send(kvs)
	}, req.GetPrefixes()...)
}

// RunServer initializes a tcp server on port which listens to requests from
// other workers for pb.communication.
func RunServer(bindall bool) {
	laddr := "localhost"
	if bindall {
		laddr = "0.0.0.0"
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", laddr, workerPort()))
	if err != nil {
		log.Fatalf("While running server: %v", err)
	}
	glog.Infof("Worker listening at address: %v", ln.Addr())

	pb.RegisterWorkerServer(workerServer, &grpcWorker{})
	pb.RegisterRaftServer(workerServer, &raftServer)
	if err := workerServer.Serve(ln); err != nil {
		glog.Errorf("Error while calling Serve: %+v", err)
	}
}

// StoreStats returns stats for data store.
func StoreStats() string {
	return "Currently no stats for badger"
}

// BlockingStop stops all the nodes, server between other workers and syncs all marks.
func BlockingStop() {
	glog.Infof("Stopping group...")
	groups().closer.SignalAndWait()

	// Update checkpoint so that proposals are not replayed after the server restarts.
	glog.Infof("Updating RAFT state before shutting down...")
	if err := groups().Node.updateRaftProgress(); err != nil {
		glog.Warningf("Error while updating RAFT progress before shutdown: %v", err)
	}

	glog.Infof("Stopping node...")
	groups().Node.closer.SignalAndWait()

	glog.Infof("Stopping raftwal store...")
	groups().Node.Store.Closer.SignalAndWait()

	glog.Infof("Stopping worker server...")
	workerServer.Stop()
}

// UpdateLruMb updates the value of lru_mb
func UpdateLruMb(memoryMB float64) error {
	if memoryMB < MinAllottedMemory {
		return errors.Errorf("lru_mb must be at least %.0f\n", MinAllottedMemory)
	}

	posting.Config.Lock()
	posting.Config.AllottedMemory = memoryMB
	posting.Config.Unlock()
	return nil
}

// UpdateLogRequest updates value of x.WorkerConfig.LogRequest.
func UpdateLogRequest(val bool) {
	if val {
		atomic.StoreInt32(&x.WorkerConfig.LogRequest, 1)
		return
	}

	atomic.StoreInt32(&x.WorkerConfig.LogRequest, 0)
}

// LogRequestEnabled returns true if logging of requests is enabled otherwise false.
func LogRequestEnabled() bool {
	return atomic.LoadInt32(&x.WorkerConfig.LogRequest) > 0
}
