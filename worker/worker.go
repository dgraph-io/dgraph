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

	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

var (
	workerPort = flag.Int("workerport", 12345,
		"Port used by worker for internal communication.")
	pstore *store.Store
)

func Init(ps *store.Store) {
	pstore = ps
}

// grpcWorker struct implements the gRPC server interface.
type grpcWorker struct{}

// Hello rpc call is used to check connection with other workers after worker
// tcp server for this instance starts.
func (w *grpcWorker) Hello(ctx context.Context, in *Payload) (*Payload, error) {
	return &Payload{Data: []byte("world")}, nil
}

// InitiateBackup inititates the backup at the machine.
func (w *grpcWorker) InitiateBackup(ctx context.Context, in *Payload) (*Payload, error) {
	// Iterate through the groups serverd by this server and backup groups
	// for which it is the leader.
	return &Payload{}, initiateBackup()
}

func initiateBackup() error {
	for gid, node := range groups().local {
		if node.AmLeader() {
			err := Backup(gid)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// BackupAll Creates backup of all nodes in cluster by initiaing backups in all the master
// nodes.
func BackupAll() error {
	errChan := make(chan error, 1000)
	count := 0
	// Send backup calls to other workers.
	for _, pl := range pools().all {
		count++
		conn, err := pl.Get()
		x.Check(err)

		go func() {
			ctx := context.Background()
			query := &Payload{}
			c := NewWorkerClient(conn)
			_, err = c.InitiateBackup(ctx, query)
			errChan <- err
			pl.Put(conn)
		}()

	}
	// Backup this worker.
	count++
	go func() {
		err := initiateBackup()
		errChan <- err
	}()

	for i := 0; i < count; i++ {
		select {
		case err := <-errChan:
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// runServer initializes a tcp server on port which listens to requests from
// other workers for internal communication.
func RunServer() {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", *workerPort))
	if err != nil {
		log.Fatalf("While running server: %v", err)
		return
	}
	log.Printf("Worker listening at address: %v", ln.Addr())

	s := grpc.NewServer(grpc.CustomCodec(&PayloadCodec{}))
	RegisterWorkerServer(s, &grpcWorker{})
	s.Serve(ln)
}

// StoreStats returns stats for data store.
func StoreStats() string {
	return pstore.GetStats()
}
