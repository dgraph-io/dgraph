/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
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

package zero

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/badger/v2/y"
	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"github.com/stretchr/testify/require"
	"go.opencensus.io/plugin/ocgrpc"
	"google.golang.org/grpc"
)

func TestRemoveNode(t *testing.T) {
	server := &Server{
		state: &pb.MembershipState{
			Groups: map[uint32]*pb.Group{1: {Members: map[uint64]*pb.Member{}}},
		},
	}
	err := server.removeNode(context.TODO(), 3, 1)
	require.Error(t, err)
	err = server.removeNode(context.TODO(), 1, 2)
	require.Error(t, err)
}

//func setState() state {
//	kvOpt := badger.LSMOnlyOptions("zw").WithSyncWrites(false).WithTruncate(true).
//		WithValueLogFileSize(64 << 20).WithMaxCacheSize(10 << 20)
//	kv, err := badger.Open(kvOpt)
//	x.Checkf(err, "Error while opening WAL store")
//	defer kv.Close()
//
//	opts.bindall = true
//	opts.myAddr = "0.0.0.0:5180"
//	opts.portOffset = 100
//	opts.nodeId = 1
//	opts.numReplicas = 1
//	opts.w = "zw"
//	opts.rebalanceInterval = 48000000
//
//	// zero out from memory
//	kvOpt.EncryptionKey = nil
//
//	store := raftwal.Init(kv, 1, 0)
//
//	// Initialize the servers.
//	var st state
//	s := grpc.NewServer(
//		grpc.MaxRecvMsgSize(x.GrpcMaxSize),
//		grpc.MaxSendMsgSize(x.GrpcMaxSize),
//		grpc.MaxConcurrentStreams(1000),
//		grpc.StatsHandler(&ocgrpc.ServerHandler{}))
//
//	rc := pb.RaftContext{Id: 1, Addr: "0.0.0.0:5180", Group: 0}
//	m := conn.NewNode(&rc, store)
//
//	// Zero followers should not be forwarding proposals to the leader, to avoid txn commits which
//	// were calculated in a previous Zero leader.
//	m.Cfg.DisableProposalForwarding = true
//	st.rs = conn.NewRaftServer(m)
//
//	st.node = &node{Node: m, ctx: context.TODO(), closer: y.NewCloser(1)}
//	st.zero = &Server{NumReplicas: 1, Node: st.node}
//	st.zero.Init()
//	st.node.server = st.zero
//
//	x.Check(st.node.initAndStartNode())
//
//	pb.RegisterZeroServer(s, st.zero)
//	pb.RegisterRaftServer(s, st.rs)
//
//	return st
//}

func setState() state {

	opts.bindall = true
	opts.myAddr = "0.0.0.0:5180"
	opts.portOffset = 100
	opts.nodeId = 1
	opts.numReplicas = 1
	opts.w = "zw"
	opts.rebalanceInterval = 48000000
	addr := "0.0.0.0"

	grpcListener, err := setupListener(addr, x.PortZeroGrpc+opts.portOffset, "grpc")
	if err != nil {
		log.Fatal(err)
	}

	// Open raft write-ahead log and initialize raft node.
	x.Checkf(os.MkdirAll(opts.w, 0700), "Error while creating WAL dir.")
	kvOpt := badger.LSMOnlyOptions(opts.w).WithSyncWrites(false).WithTruncate(true).
		WithValueLogFileSize(64 << 20).WithMaxCacheSize(10 << 20)

	// zero out from memory
	//kvOpt.EncryptionKey = nil
	//kvOpt.InMemory = true
	//kvOpt.ValueDir = ""
	//kvOpt.Dir = ""
	//kvOpt.EventLogging = false
	//kvOpt.Compression = 0

	kv, err := badger.Open(kvOpt)
	x.Checkf(err, "Error while opening WAL store")

	fmt.Printf("Here %#v\n", opts)
	store := raftwal.Init(kv, opts.nodeId, 0)

	// Initialize the servers.
	var st state

	s := grpc.NewServer(
		grpc.MaxRecvMsgSize(x.GrpcMaxSize),
		grpc.MaxSendMsgSize(x.GrpcMaxSize),
		grpc.MaxConcurrentStreams(1000),
		grpc.StatsHandler(&ocgrpc.ServerHandler{}))

	rc := pb.RaftContext{Id: opts.nodeId, Addr: opts.myAddr, Group: 0}
	m := conn.NewNode(&rc, store)

	// Zero followers should not be forwarding proposals to the leader, to avoid txn commits which
	// were calculated in a previous Zero leader.
	m.Cfg.DisableProposalForwarding = true
	st.rs = conn.NewRaftServer(m)

	st.node = &node{Node: m, ctx: context.Background(), closer: y.NewCloser(1)}
	st.zero = &Server{NumReplicas: opts.numReplicas, Node: st.node}
	st.zero.Init()
	st.node.server = st.zero

	pb.RegisterZeroServer(s, st.zero)
	pb.RegisterRaftServer(s, st.rs)

	go func() {
		defer st.zero.closer.Done()
		err := s.Serve(grpcListener)
		glog.Infof("gRPC server stopped : %v", err)

		// Attempt graceful stop (waits for pending RPCs), but force a stop if
		// it doesn't happen in a reasonable amount of time.
		done := make(chan struct{})
		const timeout = 5 * time.Second
		go func() {
			s.GracefulStop()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(timeout):
			glog.Infof("Stopping grpc gracefully is taking longer than %v."+
				" Force stopping now. Pending RPCs will be abandoned.", timeout)
			s.Stop()
		}
	}()

	x.Check(st.node.initAndStartNode())

	for !st.node.AmLeader() {
		time.Sleep(time.Second)

	}

	return st
}

func TestLease(b *testing.T) {
	num := &pb.Num{Val: uint64(10)}
	st := setState()
	st.node.server.AssignUids(context.TODO(), num)
}

func TestEmptyProposal(t *testing.T) {
	var proposal pb.ZeroProposal
	st := setState()

	err := st.node.proposeAndWait(context.TODO(), &proposal)
	require.Nil(t, err)
}

func BenchmarkEmptyProposal(b *testing.B) {
	var proposal pb.ZeroProposal
	st := setState()
	defer os.RemoveAll("zw")

	b.Run("Leasing", func(b *testing.B) {
		for i := 0; i <= b.N; i++ {
			err := st.node.proposeAndWait(context.TODO(), &proposal)
			if err != nil {
				log.Fatal(err)
			}
		}
	})
}

func BenchmarkLease(b *testing.B) {
	num := &pb.Num{Val: uint64(1)}
	st := setState()
	defer os.RemoveAll("zw")

	b.Run("Leasing", func(b *testing.B) {
		for i := 0; i <= b.N; i++ {
			_, err := st.node.server.lease(context.TODO(), num, false)
			if err != nil {
				log.Fatal(err)
			}
		}
	})
}
