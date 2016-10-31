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

	"github.com/dgraph-io/dgraph/posting"
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
	err := posting.Backup(groups().num)
	if err != nil {
		return &Payload{Data: []byte("Backup Failed")}, err
	}
	return &Payload{Data: []byte("Backup Initiated")}, nil
}

// Creates backup of all nodes in cluster by initiaing backups in all the master
// nodes.
func BackupAll() error {
	allServers := groups().all
	errChan := make(chan error, 1000)
	count := 0
	for _, v := range allServers {
		for _, ser := range v.list {
			// Only send backup request to master nodes.
			if !ser.Leader || ser.Addr == groups().Leader(groups().num) {
				continue
			}
			count++
			addr := ser.Addr
			pl := pools().get(addr)
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
	}

	go func() {
		err := posting.Backup(groups().num)
		errChan <- err
	}()
	count++

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
