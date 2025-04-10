/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

// Package worker contains code for pb.worker communication to perform
// queries and mutations.
package worker

import (
	"context"
	"fmt"
	"log"
	"math"
	"net"
	"sync"
	"sync/atomic"

	"github.com/dustin/go-humanize"
	"github.com/golang/glog"
	"github.com/pkg/errors"
	"go.opencensus.io/plugin/ocgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/dgraph-io/badger/v4"
	badgerpb "github.com/dgraph-io/badger/v4/pb"
	"github.com/dgraph-io/ristretto/v2/z"
	"github.com/hypermodeinc/dgraph/v24/conn"
	"github.com/hypermodeinc/dgraph/v24/posting"
	"github.com/hypermodeinc/dgraph/v24/protos/pb"
	"github.com/hypermodeinc/dgraph/v24/schema"
	"github.com/hypermodeinc/dgraph/v24/x"
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
	limiter = rateLimiter{c: sync.NewCond(&sync.Mutex{}), max: int(x.WorkerConfig.Raft.GetInt64("pending-proposals"))}
	go limiter.bleed()

	grpcOpts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(x.GrpcMaxSize),
		grpc.MaxSendMsgSize(x.GrpcMaxSize),
		grpc.MaxConcurrentStreams(math.MaxInt32),
		grpc.StatsHandler(&ocgrpc.ServerHandler{}),
	}

	if x.WorkerConfig.TLSServerConfig != nil {
		grpcOpts = append(grpcOpts, grpc.Creds(credentials.NewTLS(x.WorkerConfig.TLSServerConfig)))
	}
	workerServer = grpc.NewServer(grpcOpts...)
}

// grpcWorker struct implements the gRPC server interface.
type grpcWorker struct {
	sync.Mutex
	pb.WorkerServer
}

// grpcWorker implements pb.WorkerServer.
var _ pb.WorkerServer = (*grpcWorker)(nil)

func (w *grpcWorker) Subscribe(
	req *pb.SubscriptionRequest, stream pb.Worker_SubscribeServer) error {
	// Subscribe on given prefixes.
	var matches []badgerpb.Match
	for _, p := range req.GetPrefixes() {
		matches = append(matches, badgerpb.Match{
			Prefix: p,
		})
	}
	for _, m := range req.GetMatches() {
		matches = append(matches, *m)
	}
	return pstore.Subscribe(stream.Context(), func(kvs *badgerpb.KVList) error {
		return stream.Send(kvs)
	}, matches)
}

func (w *grpcWorker) ApplyDrainmode(ctx context.Context, req *pb.Drainmode) (*pb.Status, error) {
	drainMode := &pb.Drainmode{State: req.State}
	node := groups().Node
	err := node.proposeAndWait(ctx, &pb.Proposal{Drainmode: drainMode}) // Subscribe on given prefixes.

	return nil, err
}

// StreamPt handles the stream of key-value pairs sent from proxy alpha.
// It writes the data to BadgerDB, sends an acknowledgment once all data is received,
// and proposes to accept the newly added data to other group nodes.
func (w *grpcWorker) StreamPt(stream pb.Worker_StreamPtServer) error {
	n := groups().Node
	if n == nil || n.Raft() == nil {
		return conn.ErrNoNode
	}

	closer, err := n.startTask(opStreamPDirs)
	if err != nil {
	}
	defer closer.Done()

	// time.Sleep(time.Minute * 3)

	var writer badgerWriter
	sw := pstore.NewStreamWriter()
	defer sw.Cancel()

	// Prepare the stream writer, which involves deleting existing data.
	if err := sw.Prepare(); err != nil {
		return err
	}

	writer = sw

	// Track the total size of key-value data received.
	size := 0
	for {
		// Receive a batch of key-value pairs from the stream.
		kvs, err := stream.Recv()
		if err != nil {
			return err
		}

		// Check if all key-value pairs have been received.
		if kvs != nil && kvs.Done {
			glog.Infoln("All key-values have been received.")
			break
		}

		// Increment the total size and log the batch size received.
		size += len(kvs.Data)
		glog.Infof("Received batch of size: %s. Total so far: %s\n",
			humanize.IBytes(uint64(len(kvs.Data))), humanize.IBytes(uint64(size)))

		// Write the received data to BadgerDB.
		buf := z.NewBufferSlice(kvs.Data)
		if err := writer.Write(buf); err != nil {
			return err
		}
	}

	// Flush any remaining data to ensure it is written to BadgerDB.
	if err := writer.Flush(); err != nil {
		return err
	}

	glog.Infof("P dir writes DONE. Sending ACK")

	// Send an acknowledgment to the leader indicating completion.
	if err := stream.SendAndClose(&pb.ReceiveSnapshotKVRequest{Done: true}); err != nil {
		return err
	}

	// Propose a stream operation to the raft node.
	currentNode := groups().Node
	streamProposal := &pb.ReqPStream{Addr: currentNode.MyAddr}
	err = currentNode.proposeAndWait(stream.Context(), &pb.Proposal{Reqpstream: streamProposal})
	if err != nil {
		return err
	}

	// Reload the schema from the database after restoring data.
	if err := schema.LoadFromDb(stream.Context()); err != nil {
		return errors.Wrapf(err, "cannot load schema after restore")
	}

	// Inform Zero (the management node) about the updated tablets.
	gr.informZeroAboutTablets()
	return nil
}

// this fucntion will called by leader of group or the node of group which got data from
// proxy alpha or import client
func (w *grpcWorker) StreamInGroup(out pb.Worker_StreamInGroupServer) error {
	stream := pstore.NewStreamAt(math.MaxUint64)
	// Use the default implementation. We no longer try to generate a rolled up posting list here.
	// Instead, we just stream out all the versions as they are.
	stream.KeyToList = nil
	stream.Send = func(buf *z.Buffer) error {
		kvs := &pb.PKVS{Data: buf.Bytes()}
		return out.Send(kvs)
	}

	// Orchestrate the stream. This will block until the context is cancelled or the stream is closed.
	if err := stream.Orchestrate(out.Context()); err != nil {
		return err
	}

	// Indicate that we are done sending data.
	done := &pb.PKVS{
		Done: true,
	}
	if err := out.Send(done); err != nil {
		return err
	}

	glog.Infof("Streaming done. Waiting for ACK...")
	ack, err := out.Recv()
	if err != nil {
		return err
	}
	glog.Infof("Received ACK with done: %v\n", ack.Done)

	return nil
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

	glog.Infof("Stopping worker server...")
	workerServer.Stop()

	groups().Node.cdcTracker.Close()
}

// UpdateCacheMb updates the value of cache_mb and updates the corresponding cache sizes.
func UpdateCacheMb(memoryMB int64) error {
	glog.Infof("Updating cacheMb to %d", memoryMB)
	if memoryMB < 0 {
		return errors.Errorf("cache_mb must be non-negative")
	}

	cachePercent, err := x.GetCachePercentages(Config.CachePercentage, 3)
	if err != nil {
		return err
	}
	plCacheSize := (cachePercent[0] * (memoryMB << 20)) / 100
	blockCacheSize := (cachePercent[1] * (memoryMB << 20)) / 100
	indexCacheSize := (cachePercent[2] * (memoryMB << 20)) / 100

	posting.UpdateMaxCost(plCacheSize)
	if _, err := pstore.CacheMaxCost(badger.BlockCache, blockCacheSize); err != nil {
		return errors.Wrapf(err, "cannot update block cache size")
	}
	if _, err := pstore.CacheMaxCost(badger.IndexCache, indexCacheSize); err != nil {
		return errors.Wrapf(err, "cannot update index cache size")
	}

	Config.CacheMb = memoryMB
	return nil
}

// UpdateLogDQLRequest updates value of x.WorkerConfig.LogDQLRequest.
func UpdateLogDQLRequest(val bool) {
	if val {
		atomic.StoreInt32(&x.WorkerConfig.LogDQLRequest, 1)
		return
	}

	atomic.StoreInt32(&x.WorkerConfig.LogDQLRequest, 0)
}

// LogDQLRequestEnabled returns true if logging of requests is enabled otherwise false.
func LogDQLRequestEnabled() bool {
	return atomic.LoadInt32(&x.WorkerConfig.LogDQLRequest) > 0
}
