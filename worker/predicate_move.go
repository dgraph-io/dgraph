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
	"sync"
	"sync/atomic"

	"github.com/golang/glog"
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
	errEmptyPredicate = x.Errorf("Predicate not specified")
	errNotLeader      = x.Errorf("Server is not leader of this group")
	errUnableToAbort  = x.Errorf("Unable to abort pending transactions")
	emptyPayload      = api.Payload{}
)

// size of kvs won't be too big, we would take care before proposing.
func populateKeyValues(ctx context.Context, kvs []*bpb.KV) error {
	// No new deletion/background cleanup would start after we start streaming tablet,
	// so all the proposals for a particular tablet would atmost wait for deletion of
	// single tablet.
	groups().waitForBackgroundDeletion()
	glog.Infof("Writing %d keys\n", len(kvs))

	var hasError uint32
	var wg sync.WaitGroup
	wg.Add(len(kvs))
	first := true
	var predicate string
	for _, kv := range kvs {
		if first {
			pk := x.Parse(kv.Key)
			predicate = pk.Attr
			first = false
		}
		txn := pstore.NewTransactionAt(math.MaxUint64, true)
		if err := txn.SetWithMeta(kv.Key, kv.Value, kv.UserMeta[0]); err != nil {
			return err
		}
		err := txn.CommitAt(kv.Version, func(err error) {
			if err != nil {
				atomic.StoreUint32(&hasError, 1)
			}
			wg.Done()
		})
		if err != nil {
			return err
		}
		txn.Discard()
	}
	if hasError > 0 {
		return x.Errorf("Error while writing to badger")
	}
	wg.Wait()
	return schema.Load(predicate)
}

func movePredicateHelper(ctx context.Context, predicate string, gid uint32) error {
	pl := groups().Leader(gid)
	if pl == nil {
		return x.Errorf("Unable to find a connection for group: %d\n", gid)
	}
	c := pb.NewWorkerClient(pl.Get())
	s, err := c.ReceivePredicate(ctx)
	if err != nil {
		return fmt.Errorf("While calling ReceivePredicate: %+v", err)
	}

	// sends all data except schema, schema key has different prefix
	// Read the predicate keys and stream to keysCh.
	//
	// TODO: We should use a particular read timestamp here.
	stream := pstore.NewStreamAt(math.MaxUint64)
	stream.LogPrefix = fmt.Sprintf("Sending predicate: [%s]", predicate)
	stream.Prefix = x.PredicatePrefix(predicate)
	stream.KeyToList = func(key []byte, itr *badger.Iterator) (*bpb.KVList, error) {
		// For now, just send out full posting lists, because we use delete markers to delete older
		// data in the prefix range. So, by sending only one version per key, and writing it at a
		// provided timestamp, we can ensure that these writes are above all the delete markers.
		l, err := posting.ReadPostingList(key, itr)
		if err != nil {
			return nil, err
		}
		kv, err := l.MarshalToKv()
		return listWrap(kv), err
	}
	stream.Send = func(list *bpb.KVList) error {
		return s.Send(&pb.KVS{Kv: list.Kv})
	}
	if err := stream.Orchestrate(ctx); err != nil {
		return err
	}

	txn := pstore.NewTransactionAt(math.MaxUint64, false)
	defer txn.Discard()
	// Send schema (if present) now after all keys have been transferred over.
	schemaKey := x.SchemaKey(predicate)
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
		kv := &bpb.KV{
			Key:      schemaKey,
			Value:    val,
			Version:  1,
			UserMeta: []byte{item.UserMeta()},
		}
		kvs.Kv = append(kvs.Kv, kv)
		if err := s.Send(kvs); err != nil {
			return err
		}
	}

	payload, err := s.CloseAndRecv()
	if err != nil {
		return err
	}
	recvCount, err := strconv.Atoi(string(payload.Data))
	if err != nil {
		return err
	}
	glog.Infof("Received %d keys\n", recvCount)
	return nil
}

func batchAndProposeKeyValues(ctx context.Context, kvs chan *pb.KVS) error {
	glog.Infoln("Receiving predicate. Batching and proposing key values")
	n := groups().Node
	proposal := &pb.Proposal{}
	size := 0
	var pk *x.ParsedKey

	for kvBatch := range kvs {
		for _, kv := range kvBatch.Kv {
			if size >= 32<<20 { // 32 MB
				if err := n.proposeAndWait(ctx, proposal); err != nil {
					return err
				}
				proposal.Kv = proposal.Kv[:0]
				size = 0
			}

			if pk == nil {
				pk = x.Parse(kv.Key)
				// Delete on all nodes.
				p := &pb.Proposal{CleanPredicate: pk.Attr}
				glog.Infof("Predicate being received: %v", pk.Attr)
				err := groups().Node.proposeAndWait(ctx, p)
				if err != nil {
					glog.Errorf("Error while cleaning predicate %v %v\n", pk.Attr, err)
					return err
				}
			}
			proposal.Kv = append(proposal.Kv, kv)
			size += len(kv.Key) + len(kv.Value)
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
	// Values can be pretty big so having less buffer is safer.
	kvs := make(chan *pb.KVS, 10)
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
	if groups().gid != in.SourceGroupId {
		return &emptyPayload,
			x.Errorf("Group id doesn't match, received request for %d, my gid: %d",
				in.SourceGroupId, groups().gid)
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

	glog.Infof("Move predicate request for pred: [%v], src: [%v], dst: [%v]\n", in.Predicate,
		in.SourceGroupId, in.DestGroupId)

	// Ensures that all future mutations beyond this point are rejected.
	if err := n.proposeAndWait(ctx, &pb.Proposal{State: in.State}); err != nil {
		return &emptyPayload, err
	}
	aborted := false
	for i := 0; i < 12; i++ {
		// Try a dozen times, then give up.
		glog.Infof("Trying to abort pending mutations. Loop: %d", i)
		tctxs := posting.Oracle().IterateTxns(func(key []byte) bool {
			pk := x.Parse(key)
			return pk.Attr == in.Predicate
		})
		if len(tctxs) == 0 {
			aborted = true
			break
		}
		tryAbortTransactions(tctxs)
	}
	if !aborted {
		return &emptyPayload, errUnableToAbort
	}
	// We iterate over badger, so need to flush and wait for sync watermark to catch up.
	n.applyAllMarks(ctx)

	err := movePredicateHelper(ctx, in.Predicate, in.DestGroupId)
	return &emptyPayload, err
}
