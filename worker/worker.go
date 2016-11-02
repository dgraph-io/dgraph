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

	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

var (
	workerPort = flag.Int("workerport", 12345,
		"Port used by worker for internal communication.")
	backupPath = flag.String("backup", "backup",
		"Folder in which to store backups.")
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
	var grpList []uint32

	groups().RLock()
	for gid, node := range groups().local {
		if node.AmLeader() {
			grpList = append(grpList, gid)
		}
	}
	groups().RUnlock()

	for _, gid := range grpList {
		err := Backup(gid, *backupPath)
		if err != nil {
			return err
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
	addrMap := make(map[string]bool)
	groups().RLock()
	for _, servers := range groups().all {
		for _, it := range servers.list {
			// Don't add the current nodes address in this.
			if it.Leader {
				addrMap[it.Addr] = true
			}
		}
	}
	groups().RUnlock()

	for addr := range addrMap {
		count++
		pl := pools().get(addr)
		conn, err := pl.Get()
		if err != nil {
			return err
		}
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
