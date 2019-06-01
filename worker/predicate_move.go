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
	"strconv"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	otrace "go.opencensus.io/trace"
	"golang.org/x/net/context"

	"github.com/dgraph-io/badger"
	bpb "github.com/dgraph-io/badger/pb"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/x"
)

var (
	errEmptyPredicate = errors.Errorf("Predicate not specified")
	errNotLeader      = errors.Errorf("Server is not leader of this group")
	emptyPayload      = api.Payload{}
)

// size of kvs won't be too big, we would take care before proposing.
func populateKeyValues(ctx context.Context, kvs []*bpb.KV) error {
	glog.Infof("Writing %d keys\n", len(kvs))
	if len(kvs) == 0 {
		return nil
	}
	writer := posting.NewTxnWriter(pstore)
	if err := writer.Write(&bpb.KVList{Kv: kvs}); err != nil {
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
					return errors.Errorf("Expecting first key to be schema key: %+v", kv)
				}
				// Delete on all nodes.
				p := &pb.Proposal{CleanPredicate: pk.Attr}
				glog.Infof("Predicate being received: %v", pk.Attr)
				if err := n.proposeAndWait(ctx, p); err != nil {
					glog.Errorf("Error while cleaning predicate %v %v\n", pk.Attr, err)
					return err
				}
			}

			proposal.Kv = append(proposal.Kv, kv)
			size += len(kv.Key) + len(kv.Value)
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
		return errors.Errorf("ReceivePredicate failed: Not the leader of group")
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

	n := groups().Node
	if !n.AmLeader() {
		return &emptyPayload, errNotLeader
	}
	if groups().groupId() != in.SourceGid {
		return &emptyPayload,
			errors.Errorf("Group id doesn't match, received request for %d, my gid: %d",
				in.SourceGid, groups().groupId())
	}
	if len(in.Predicate) == 0 {
		return &emptyPayload, errEmptyPredicate
	}
	if in.DestGid == 0 {
		glog.Infof("Was instructed to delete tablet: %v", in.Predicate)
		p := &pb.Proposal{CleanPredicate: in.Predicate}
		return &emptyPayload, groups().Node.proposeAndWait(ctx, p)
	}
	if err := posting.Oracle().WaitForTs(ctx, in.TxnTs); err != nil {
		return &emptyPayload, errors.Errorf("While waiting for txn ts: %d. Error: %v", in.TxnTs, err)
	}
	if gid, err := groups().BelongsTo(in.Predicate); err != nil {
		return &emptyPayload, err
	} else if gid == 0 {
		return &emptyPayload, errNonExistentTablet
	} else if gid != groups().groupId() {
		return &emptyPayload, errUnservedTablet
	}

	msg := fmt.Sprintf("Move predicate request: %+v", in)
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
		return errors.Errorf("Unable to find a connection for group: %d\n", in.DestGid)
	}
	c := pb.NewWorkerClient(pl.Get())
	s, err := c.ReceivePredicate(ctx)
	if err != nil {
		return fmt.Errorf("While calling ReceivePredicate: %+v", err)
	}

	// This txn is only reading the schema. Doesn't really matter what read timestamp we use,
	// because schema keys are always set at ts=1.
	txn := pstore.NewTransactionAt(in.TxnTs, false)
	defer txn.Discard()

	// Send schema first.
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
		kv := &bpb.KV{}
		kv.Key = schemaKey
		kv.Value = val
		kv.Version = 1
		kv.UserMeta = []byte{item.UserMeta()}
		kvs.Kv = append(kvs.Kv, kv)
		if err := s.Send(kvs); err != nil {
			return err
		}
	}

	// sends all data except schema, schema key has different prefix
	// Read the predicate keys and stream to keysCh.
	stream := pstore.NewStreamAt(in.TxnTs)
	stream.LogPrefix = fmt.Sprintf("Sending predicate: [%s]", in.Predicate)
	stream.Prefix = x.PredicatePrefix(in.Predicate)
	stream.KeyToList = func(key []byte, itr *badger.Iterator) (*bpb.KVList, error) {
		// For now, just send out full posting lists, because we use delete markers to delete older
		// data in the prefix range. So, by sending only one version per key, and writing it at a
		// provided timestamp, we can ensure that these writes are above all the delete markers.
		l, err := posting.ReadPostingList(key, itr)
		if err != nil {
			return nil, err
		}
		kvs, err := l.Rollup()
		for _, kv := range kvs {
			// Let's set all of them at this move timestamp.
			kv.Version = in.TxnTs
		}
		return &bpb.KVList{Kv: kvs}, err
	}
	stream.Send = func(list *bpb.KVList) error {
		return s.Send(&pb.KVS{Kv: list.Kv})
	}
	span.Annotatef(nil, "Starting stream list orchestrate")
	if err := stream.Orchestrate(ctx); err != nil {
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

	msg := fmt.Sprintf("Receiver %s says it got %d keys.\n", pl.Addr, recvCount)
	span.Annotate(nil, msg)
	glog.Infof(msg)
	return nil
}
