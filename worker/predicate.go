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
	"bytes"
	"context"
	"io"
	"sort"

	"github.com/dgraph-io/dgraph/posting/types"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/x"
	flatbuffers "github.com/google/flatbuffers/go"
)

const (
	// MB represents a megabyte.
	MB = 1 << 20
)

// writeBatch performs a batch write of key value pairs to RocksDB.
func writeBatch(ctx context.Context, kv chan *task.KV, che chan error) {
	wb := pstore.NewWriteBatch()
	batchSize := 0
	batchWriteNum := 1
	for i := range kv {
		wb.Put(i.KeyBytes(), i.ValBytes())
		batchSize += len(i.KeyBytes()) + len(i.ValBytes())
		// We write in batches of size 32MB.
		if batchSize >= 32*MB {
			x.Trace(ctx, "Doing batch write %d.", batchWriteNum)
			if err := pstore.WriteBatch(wb); err != nil {
				che <- err
				return
			}

			batchWriteNum++
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
		x.Trace(ctx, "Doing batch write %d.", batchWriteNum)
		che <- pstore.WriteBatch(wb)
		return
	}
	che <- nil
}

func generateGroup(group uint64) ([]byte, error) {
	it := pstore.NewIterator()
	defer it.Close()

	b := flatbuffers.NewBuilder(0)
	uoffsets := make([]flatbuffers.UOffsetT, 0, 100)

	for it.SeekToFirst(); it.Valid(); it.Next() {
		// TODO: Check if this key belongs to the group.

		k, v := it.Key(), it.Value()
		pl := types.GetRootAsPostingList(v.Data(), 0)

		ko := b.CreateByteVector(k.Data())
		co := b.CreateByteVector(pl.Checksum())

		task.KCStart(b)
		task.KCAddKey(b, ko)
		task.KCAddChecksum(b, co)
		uo := task.KCEnd(b)

		uoffsets = append(uoffsets, uo)
	}
	if err := it.Err(); err != nil {
		return []byte{}, nil
	}

	task.GroupKeysStartKeysVector(b, len(uoffsets))
	for i := len(uoffsets) - 1; i >= 0; i-- {
		b.PrependUOffsetT(uoffsets[i])
	}
	keysOffset := b.EndVector(len(uoffsets))

	task.GroupKeysStart(b)
	task.GroupKeysAddGroupid(b, group)
	task.GroupKeysAddKeys(b, keysOffset)
	rend := task.GroupKeysEnd(b)
	b.Finish(rend)
	return b.Bytes[b.Head():], nil
}

// PopulateShard gets data for predicate pred from server with id serverId and
// writes it to RocksDB.
func populateShard(ctx context.Context, pl *pool, group uint64) (int, error) {
	query := new(Payload)
	data, err := generateGroup(group)
	if err != nil {
		return 0, x.Wrapf(err, "While generating keys group")
	}
	query.Data = data

	conn, err := pl.Get()
	if err != nil {
		return 0, err
	}
	defer pl.Put(conn)
	c := NewWorkerClient(conn)

	stream, err := c.PredicateData(context.Background(), query)
	if err != nil {
		return 0, err
	}
	x.Trace(ctx, "Streaming data for group: %v", group)

	kvs := make(chan *task.KV, 1000)
	che := make(chan error)
	go writeBatch(ctx, kvs, che)

	// We can use count to check the number of posting lists returned in tests.
	count := 0
	for {
		payload, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			close(kvs)
			return count, err
		}
		count++
		uo := flatbuffers.GetUOffsetT(payload.Data)
		kv := new(task.KV)
		kv.Init(payload.Data, uo)

		// We check for errors, if there are no errors we send value to channel.
		select {
		case <-ctx.Done():
			x.TraceError(ctx, x.Errorf("Context timed out while streaming group: %v", group))
			close(kvs)
			return count, ctx.Err()

		case err := <-che:
			x.TraceError(ctx, x.Errorf("Error while doing a batch write for group: %v", group))
			close(kvs)
			return count, err

		case kvs <- kv:
		}
	}
	close(kvs)

	if err := <-che; err != nil {
		x.TraceError(ctx, x.Errorf("Error while doing a batch write for group: %v", group))
		return count, err
	}
	x.Trace(ctx, "Streaming complete for group: %v", group)
	return count, nil
}

// PredicateData can be used to return data corresponding to a predicate over
// a stream.
func (w *grpcWorker) PredicateData(query *Payload, stream Worker_PredicateDataServer) error {
	var group task.GroupKeys
	uo := flatbuffers.GetUOffsetT(query.Data)
	group.Init(query.Data, uo)
	_ = group.Groupid()

	// TODO(pawan) - Shift to CheckPoints once we figure out how to add them to the
	// RocksDB library we are using.
	// http://rocksdb.org/blog/2609/use-checkpoints-for-efficient-snapshots/
	it := pstore.NewIterator()
	defer it.Close()

	for it.SeekToFirst(); it.Valid(); it.Next() {
		k, v := it.Key(), it.Value()
		pl := types.GetRootAsPostingList(v.Data(), 0)

		// TODO: Check that key is part of the specified group id.
		i := sort.Search(group.KeysLength(), func(i int) bool {
			var t task.KC
			x.AssertTruef(group.Keys(&t, i), "Unable to parse task.KC")
			return bytes.Compare(k.Data(), t.KeyBytes()) <= 0
		})

		if i < group.KeysLength() {
			// Found a match.
			var t task.KC
			x.AssertTruef(group.Keys(&t, i), "Unable to parse task.KC")

			if bytes.Equal(k.Data(), t.KeyBytes()) && bytes.Equal(pl.Checksum(), t.ChecksumBytes()) {
				// No need to send this.
				continue
			}
		}

		b := flatbuffers.NewBuilder(0)
		bko := b.CreateByteVector(k.Data())
		bvo := b.CreateByteVector(v.Data())
		task.KVStart(b)
		task.KVAddKey(b, bko)
		task.KVAddVal(b, bvo)
		kvoffset := task.KVEnd(b)
		b.Finish(kvoffset)

		p := Payload{Data: b.Bytes[b.Head():]}
		if err := stream.Send(&p); err != nil {
			return err
		}
		k.Free()
		v.Free()
	}
	if err := it.Err(); err != nil {
		return err
	}
	return nil
}
