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
	"strconv"

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
	pstore *store.Store
)

func Init(ps *store.Store) {
	pstore = ps
}

// grpcWorker struct implements the gRPC server interface.
type grpcWorker struct{}

// Hello rpc call is used to check connection with other workers after worker
// tcp server for this instance starts.
func (w *grpcWorker) Echo(ctx context.Context, in *Payload) (*Payload, error) {
	return &Payload{Data: in.Data}, nil
}

// InitiateBackup inititates the backup at the machine.
func (w *grpcWorker) InitiateBackup(ctx context.Context, in *Payload) (*Payload, error) {
	// Iterate through the groups serverd by this server and backup groups
	// for which it is the leader.
	gid, err := strconv.Atoi(string(in.Data))
	x.Check(err)
	return &Payload{}, initiateBackup(uint32(gid))
}

// StartBackupProcess starts the backup process if it is master of group 0.
func (w *grpcWorker) StartBackupProcess(ctx context.Context, in *Payload) (*Payload, error) {
	cur, ok := groups().local[0]
	if ok && cur.AmLeader() {
		return &Payload{}, BackupAll()
	}
	return &Payload{}, fmt.Errorf("Backup request sent to wrong server")
}

func initiateBackup(gid uint32) error {
	if !groups().Node(gid).AmLeader() {
		return fmt.Errorf("Node not leader of group %d", gid)
	}
	return backup(gid, *backupPath)
}

// StartBackup starts the backup process if it is master of group 0
// else calls the master of group 0.
func StartBackup() error {
	cur, ok := groups().local[0]
	if ok && cur.AmLeader() {
		return BackupAll()
	}

	for _, node := range groups().all[0].list {
		if node.Leader {
			addr := node.Addr
			pl := pools().get(addr)
			conn, err := pl.Get()
			if err != nil {
				return err
			}
			ctx := context.Background()
			query := &Payload{}
			c := NewWorkerClient(conn)
			_, err = c.StartBackupProcess(ctx, query)
			return err
		}
	}
	return nil
}

// BackupAll Creates backup of all nodes in cluster by initiating backups in all the master
// nodes.
func BackupAll() error {
	errChan := make(chan error, 1000)
	count := 0
	// Send backup calls to other workers.
	doneMap := make(map[uint32]bool)

	groups().RLock()
	for gid, it := range groups().local {
		if it.AmLeader() {
			count++
			doneMap[gid] = true
			go func() {
				err := initiateBackup(gid)
				errChan <- err
			}()
		}
	}

	for gid, servers := range groups().all {
		if _, ok := doneMap[gid]; ok {
			continue
		}
		for _, it := range servers.list {
			if it.Leader {
				count++
				pl := pools().get(it.Addr)
				conn, err := pl.Get()
				if err != nil {
					return err
				}
				go func() {
					ctx := context.Background()
					query := &Payload{[]byte(strconv.Itoa(int(gid)))}
					c := NewWorkerClient(conn)
					_, err = c.InitiateBackup(ctx, query)
					errChan <- err
					pl.Put(conn)
				}()
				break
			}
		}
	}
	groups().RUnlock()

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
