/*
* Copyright 2016 DGraph Labs, Inc.
*
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You may obtain a copy of the License at
*
*         http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
 */

package worker

import (
	"context"
	"io"

	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/x"
	flatbuffers "github.com/google/flatbuffers/go"
)

const (
	// MB represents a megabyte.
	MB = 1 << 20
)

// writeBatch performs a batch write of key value pairs to RocksDB.
func writeBatch(kv chan *task.KV, che chan error) {
	wb := dataStore.NewWriteBatch()
	batchSize := 0
	for i := range kv {
		wb.Put(i.KeyBytes(), i.ValBytes())
		batchSize += len(i.KeyBytes()) + len(i.ValBytes())
		// We write in batches of size 32MB.
		if batchSize >= 32*MB {
			if err := dataStore.WriteBatch(wb); err != nil {
				che <- err
				return
			}
			// Resetting batch size after a batch write.
			batchSize = 0
			// Since we are writing data in batches, we need to clear up items enqueued
			// for batch write after every successful write.
			wb.Clear()
		}
	}
	// After channel is closed the above loop would exit, we write the data in
	// write batch here.
	if batchSize > 0 {
		if err := dataStore.WriteBatch(wb); err != nil {
			che <- err
			return
		}
	}
	che <- nil
}

// PopulateShard gets data for predicate pred from server with id serverId and
// writes it to RocksDB.
func PopulateShard(ctx context.Context, pred string, serverId int) error {
	var err error

	pool := pools[serverId]
	query := new(Payload)
	query.Data = []byte(pred)
	if err != nil {
		return err
	}

	conn, err := pool.Get()
	if err != nil {
		return err
	}
	defer pool.Put(conn)
	c := NewWorkerClient(conn)

	stream, err := c.PredicateData(context.Background(), query)
	if err != nil {
		return err
	}
	x.Trace(ctx, "Streaming data for pred: %v from server with id: %v", pred, serverId)

	kvs := make(chan *task.KV, 1000)
	che := make(chan error)
	go writeBatch(kvs, che)

	for {
		payload, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			close(kvs)
			return err
		}

		uo := flatbuffers.GetUOffsetT(payload.Data)
		kv := new(task.KV)
		kv.Init(payload.Data, uo)

		// We check for errors, if there are no errors we send value to channel.
		select {
		case <-ctx.Done():
			close(kvs)
			return ctx.Err()
		case err := <-che:
			close(kvs)
			return err
		case kvs <- kv:
		}
	}
	close(kvs)

	if err := <-che; err != nil {
		return err
	}
	x.Trace(ctx, "Streaming complete for pred: %v from server with id: %v", pred, serverId)
	return nil
}
