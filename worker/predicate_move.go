/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
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

package worker

import (
	"fmt"
	"io"
	"math"
	"strconv"

	"github.com/golang/glog"
	otrace "go.opencensus.io/trace"
	"golang.org/x/net/context"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/stream"
	"github.com/dgraph-io/dgraph/x"
)

var (
	errEmptyPredicate = x.Errorf("Predicate not specified")
	errNotLeader      = x.Errorf("Server is not leader of this group")
	errUnableToAbort  = x.Errorf("Unable to abort pending transactions")
	emptyPayload      = api.Payload{}
)

// size of kvs won't be too big, we would take care before proposing.
func populateKeyValues(ctx context.Context, kvs []*pb.KV) error {
	glog.Infof("Writing %d keys\n", len(kvs))
	if len(kvs) == 0 {
		return nil
	}
	writer := x.NewTxnWriter(pstore)
	writer.BlindWrite = true
	if err := writer.Send(&pb.KVS{Kv: kvs}); err != nil {
		return err
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	pk := x.Parse(kvs[0].Key)
	return schema.Load(pk.Attr)
}

func batchAndProposeKeyValues(ctx context.Context, kvs chan *pb.KVS) error {
	glog.Infoln("Receiving predicate. Batching and proposing key values")
	n := groups().Node
	proposal := &pb.Proposal{}
	size := 0
	var pk *x.ParsedKey

	for kvBatch := range kvs {
		for _, kv := range kvBatch.Kv {
			if pk == nil {
				// This only happens once.
				pk = x.Parse(kv.Key)
				if !pk.IsSchema() {
					return x.Errorf("Expecting first key to be schema key: %+v", kv)
				}
				// Delete on all nodes.
				p := &pb.Proposal{CleanPredicate: pk.Attr}
				glog.Infof("Predicate being received: %v", pk.Attr)
				if err := groups().Node.proposeAndWait(ctx, p); err != nil {
					glog.Errorf("Error while cleaning predicate %v %v\n", pk.Attr, err)
					return err
				}
			}

			proposal.Kv = append(proposal.Kv, kv)
			size += len(kv.Key) + len(kv.Val)
			if size >= 32<<20 { // 32 MB
				if err := n.proposeAndWait(ctx, proposal); err != nil {
					return err
				}
				proposal = &pb.Proposal{}
				size = 0
			}
		}
	}
	if size > 0 {
		// Propose remaining keys.
		if err := n.proposeAndWait(ctx, proposal); err != nil {
			return err
		}
	}
	return nil
}

// Returns count which can be used to verify whether we have moved all keys
// for a predicate or not.
func (w *grpcWorker) ReceivePredicate(stream pb.Worker_ReceivePredicateServer) error {
	if !groups().Node.AmLeader() {
		return x.Errorf("ReceivePredicate failed: Not the leader of group")
	}
	// No new deletion/background cleanup would start after we start streaming tablet,
	// so all the proposals for a particular tablet would atmost wait for deletion of
	// single tablet. Only leader needs to do this.
	mu := groups().blockDeletes
	mu.Lock()
	defer mu.Unlock()

	// Values can be pretty big so having less buffer is safer.
	kvs := make(chan *pb.KVS, 3)
	che := make(chan error, 1)
	// We can use count to check the number of posting lists returned in tests.
	count := 0
	ctx := stream.Context()
	payload := &api.Payload{}

	glog.Infof("Got ReceivePredicate. Group: %d. Am leader: %v",
		groups().groupId(), groups().Node.AmLeader())

	go func() {
		// Takes care of throttling and batching.
		che <- batchAndProposeKeyValues(ctx, kvs)
	}()
	for {
		kvBatch, err := stream.Recv()
		if err == io.EOF {
			payload.Data = []byte(fmt.Sprintf("%d", count))
			stream.SendAndClose(payload)
			break
		}
		if err != nil {
			glog.Errorf("Received %d keys. Error in loop: %v\n", count, err)
			return err
		}
		count += len(kvBatch.Kv)

		select {
		case kvs <- kvBatch:
		case <-ctx.Done():
			close(kvs)
			<-che
			glog.Infof("Received %d keys. Context deadline\n", count)
			return ctx.Err()
		case err := <-che:
			glog.Infof("Received %d keys. Error via channel: %v\n", count, err)
			return err
		}
	}
	close(kvs)
	err := <-che
	glog.Infof("Proposed %d keys. Error: %v\n", count, err)
	return err
}

func (w *grpcWorker) MovePredicate(ctx context.Context,
	in *pb.MovePredicatePayload) (*api.Payload, error) {
	ctx, span := otrace.StartSpan(ctx, "worker.MovePredicate")
	defer span.End()

	if groups().gid != in.SourceGid {
		return &emptyPayload,
			x.Errorf("Group id doesn't match, received request for %d, my gid: %d",
				in.SourceGid, groups().gid)
	}
	if len(in.Predicate) == 0 {
		return &emptyPayload, errEmptyPredicate
	}
	if !groups().ServesTablet(in.Predicate) {
		return &emptyPayload, errUnservedTablet
	}
	n := groups().Node
	if !n.AmLeader() {
		return &emptyPayload, errNotLeader
	}
	if err := posting.Oracle().WaitForTs(ctx, in.TxnTs); err != nil {
		return &emptyPayload, x.Errorf("While waiting for txn ts: %d. Error: %v", in.TxnTs, err)
	}

	msg := fmt.Sprintf("Move predicate request for pred: [%v], src: [%v], dst: [%v]\n",
		in.Predicate, in.SourceGid, in.DestGid)
	glog.Info(msg)
	span.Annotate(nil, msg)

	err := movePredicateHelper(ctx, in)
	if err != nil {
		span.Annotatef(nil, "Error while movePredicateHelper: %v", err)
	}
	return &emptyPayload, err
}

func movePredicateHelper(ctx context.Context, in *pb.MovePredicatePayload) error {
	span := otrace.FromContext(ctx)

	pl := groups().Leader(in.DestGid)
	if pl == nil {
		return x.Errorf("Unable to find a connection for group: %d\n", in.DestGid)
	}
	c := pb.NewWorkerClient(pl.Get())
	s, err := c.ReceivePredicate(ctx)
	if err != nil {
		return fmt.Errorf("While calling ReceivePredicate: %+v", err)
	}

	txn := pstore.NewTransactionAt(math.MaxUint64, false)
	defer txn.Discard()
	// Send schema (if present) now after all keys have been transferred over.
	schemaKey := x.SchemaKey(in.Predicate)
	item, err := txn.Get(schemaKey)
	if err == badger.ErrKeyNotFound {
		// The predicate along with the schema could have been deleted. In that case badger would
		// return ErrKeyNotFound. We don't want to try and access item.Value() in that case.
	} else if err != nil {
		return err
	} else {
		val, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}
		kvs := &pb.KVS{}
		kv := &pb.KV{}
		kv.Key = schemaKey
		kv.Val = val
		kv.Version = 1
		kv.UserMeta = []byte{item.UserMeta()}
		kvs.Kv = append(kvs.Kv, kv)
		if err := s.Send(kvs); err != nil {
			return err
		}
	}

	// sends all data except schema, schema key has different prefix
	// Read the predicate keys and stream to keysCh.
	sl := stream.Lists{Stream: s, Predicate: in.Predicate, DB: pstore}
	sl.ItemToKVFunc = func(key []byte, itr *badger.Iterator) (*pb.KV, error) {
		l, err := posting.ReadPostingList(key, itr)
		if err != nil {
			return nil, err
		}
		return l.MarshalToKv()
	}

	span.Annotatef(nil, "Starting stream list orchestrate")
	prefix := fmt.Sprintf("Sending predicate: [%s]", in.Predicate)
	if err := sl.Orchestrate(ctx, prefix, math.MaxUint64); err != nil {
		return err
	}
	payload, err := s.CloseAndRecv()
	if err != nil {
		return err
	}
	recvCount, err := strconv.Atoi(string(payload.Data))
	if err != nil {
		return err
	}

	msg := fmt.Sprintf("Receiver says it got %d keys.\n", recvCount)
	span.Annotate(nil, msg)
	glog.Infof(msg)
	return nil
}
