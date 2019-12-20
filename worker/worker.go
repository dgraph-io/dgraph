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

	"github.com/dgraph-io/badger/v2"
	badgerpb "github.com/dgraph-io/badger/v2/pb"
	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"go.opencensus.io/plugin/ocgrpc"
	"golang.org/x/net/context"

	"github.com/golang/glog"
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

// Init initializes this package.
func Init(ps *badger.DB) {
	pstore = ps
	// needs to be initialized after group config
	pendingProposals = make(chan struct{}, x.WorkerConfig.NumPendingProposals)
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

func (w *grpcWorker) SubscribeForKV(
	req *pb.SubscriptionRequest, stream pb.Worker_SubscribeForKVServer) error {

	ctx, cancel := context.WithCancel(stream.Context())
	// Subscribe on given prefixes.
	var streamErr error
	err := pstore.Subscribe(ctx, func(kvs *badgerpb.KVList) {
		streamErr = stream.Send(kvs)
		if streamErr != nil {
			// Cancel the current subscription if not able to push to the stream.
			// This is stop the current subscription and end the stream.
			cancel()
		}
	}, req.GetPrefixes()...)
	// Return the err if there is an err returned by badger.
	if err != nil {
		return err
	}
	return streamErr
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

	glog.Infof("Stopping node...")
	groups().Node.closer.SignalAndWait()

	glog.Infof("Stopping worker server...")
	workerServer.Stop()
}
