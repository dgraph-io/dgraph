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

// Predicate gets data for a predicate p from another instance and writes it to RocksDB.
func Predicate(p string, idx int) error {
	var err error

	pool := pools[idx]
	query := new(Payload)
	query.Data = []byte(p)
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

	kvs := make(chan x.KV, 10000)
	che := make(chan error)
	go dataStore.WriteBatch(kvs, che)

	for {
		b, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			close(kvs)
			return err
		}

		uo := flatbuffers.GetUOffsetT(b.Data)
		kv := new(task.KV)
		kv.Init(b.Data, uo)
		kvs <- x.KV{Key: kv.KeyBytes(), Val: kv.ValBytes()}
	}
	close(kvs)
	if err := <-che; err != nil {
		return err
	}
	return nil
}
