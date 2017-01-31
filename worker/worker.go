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
	"flag"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

var (
	workerPort = flag.Int("workerport", 12345,
		"Port used by worker for internal communication.")
	backupPath = flag.String("backup", "backup",
		"Folder in which to store backups.")
	pstore       *store.Store
	workerServer *grpc.Server
)

func Init(ps *store.Store) {
	pstore = ps
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

// Hello rpc call is used to check connection with other workers after worker
// tcp server for this instance starts.
func (w *grpcWorker) Echo(ctx context.Context, in *Payload) (*Payload, error) {
	return &Payload{Data: in.Data}, nil
}

// RunServer initializes a tcp server on port which listens to requests from
// other workers for internal communication.
func RunServer(bindall bool) {
	laddr := "localhost"
	if bindall {
		laddr = "0.0.0.0"
	}
	var err error
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", laddr, *workerPort))
	if err != nil {
		log.Fatalf("While running server: %v", err)
		return
	}
	log.Printf("Worker listening at address: %v", ln.Addr())

	workerServer = grpc.NewServer()
	RegisterWorkerServer(workerServer, &grpcWorker{})
	workerServer.Serve(ln)
}

// StoreStats returns stats for data store.
func StoreStats() string {
	return pstore.GetStats()
}

// BlockingStop stops all the nodes, server between other workers and syncs all marks.
func BlockingStop() {
	stopAllNodes()              // blocking stop all nodes
	workerServer.GracefulStop() // blocking stop server
	// blocking sync all marks
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := syncAllMarks(ctx); err != nil {
		x.Printf("Error in sync watermarks : %s", err.Error())
	}
}
