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
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/dustin/go-humanize"
	"github.com/golang/glog"
	"github.com/pkg/errors"
	otrace "go.opencensus.io/trace"

	"github.com/dgraph-io/badger/v3"
	bpb "github.com/dgraph-io/badger/v3/pb"
	"github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
)

var (
	errEmptyPredicate = errors.Errorf("Predicate not specified")
	errNotLeader      = errors.Errorf("Server is not leader of this group")
	emptyPayload      = api.Payload{}
)

const (
	// NoCleanPredicate is used to indicate that we are in phase 2 of predicate move, so we should
	// not clean the predicate.
	NoCleanPredicate = iota
	// CleanPredicate is used to indicate that we need to clean the predicate on receiver.
	CleanPredicate
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
	pk, err := x.Parse(kvs[0].Key)
	if err != nil {
		return errors.Errorf("while parsing KV: %+v, got error: %v", kvs[0], err)
	}
	return schema.Load(pk.Attr)
}

func batchAndProposeKeyValues(ctx context.Context, kvs chan *pb.KVS) error {
	glog.Infoln("Receiving predicate. Batching and proposing key values")
	n := groups().Node
	proposal := &pb.Proposal{}
	size := 0
	var pk x.ParsedKey

	for kvPayload := range kvs {
		buf := z.NewBufferSlice(kvPayload.GetData())
		err := buf.SliceIterate(func(s []byte) error {
			kv := &bpb.KV{}
			x.Check(kv.Unmarshal(s))
			if len(pk.Attr) == 0 {
				// This only happens once.
				var err error
				pk, err = x.Parse(kv.Key)
				if err != nil {
					return errors.Errorf("while parsing kv: %+v, got error: %v", kv, err)
				}

				if !pk.IsSchema() {
					return errors.Errorf("Expecting first key to be schema key: %+v", kv)
				}

				glog.Infof("Predicate being received: %v", pk.Attr)
				if kv.StreamId == CleanPredicate {
					// Delete on all nodes. Remove the schema at timestamp kv.Version-1 and set it at
					// kv.Version. kv.Version will be the TxnTs of the predicate move.
					p := &pb.Proposal{CleanPredicate: pk.Attr, StartTs: kv.Version - 1}
					if err := n.proposeAndWait(ctx, p); err != nil {
						glog.Errorf("Error while cleaning predicate %v %v\n", pk.Attr, err)
						return err
					}
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
			return nil
		})
		if err != nil {
			return err
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
		kvBuf, err := stream.Recv()
		if err == io.EOF {
			payload.Data = []byte(fmt.Sprintf("%d", count))
			if err := stream.SendAndClose(payload); err != nil {
				glog.Errorf("Received %d keys. Error in loop: %v\n", count, err)
				return err
			}
			break
		}
		if err != nil {
			glog.Errorf("Received %d keys. Error in loop: %v\n", count, err)
			return err
		}
		glog.V(2).Infof("Received batch of size: %s\n", humanize.IBytes(uint64(len(kvBuf.Data))))

		buf := z.NewBufferSlice(kvBuf.Data)
		buf.SliceIterate(func(_ []byte) error {
			count++
			return nil
		})

		select {
		case kvs <- kvBuf:
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
		// Expected Checksum ensures that all the members of this group would block until they get
		// the latest membership status where this predicate now belongs to another group. So they
		// know that they are no longer serving this predicate, before they delete it from their
		// state. Without this checksum, the members could end up deleting the predicate and then
		// serve a request asking for that predicate, causing Jepsen failures.
		p := &pb.Proposal{
			CleanPredicate:   in.Predicate,
			ExpectedChecksum: in.ExpectedChecksum,
			StartTs:          in.ReadTs,
		}
		return &emptyPayload, groups().Node.proposeAndWait(ctx, p)
	}
	if err := posting.Oracle().WaitForTs(ctx, in.ReadTs); err != nil {
		return &emptyPayload,
			errors.Errorf("While waiting for read ts: %d. Error: %v", in.ReadTs, err)
	}

	gid, err := groups().BelongsTo(in.Predicate)
	switch {
	case err != nil:
		return &emptyPayload, err
	case gid == 0:
		return &emptyPayload, errNonExistentTablet
	case gid != groups().groupId():
		return &emptyPayload, errUnservedTablet
	}

	msg := fmt.Sprintf("Move predicate request: %+v", in)
	glog.Info(msg)
	span.Annotate(nil, msg)

	err = movePredicateHelper(ctx, in)
	if err != nil {
		span.Annotatef(nil, "Error while movePredicateHelper: %v", err)
	}
	return &emptyPayload, err
}

func movePredicateHelper(ctx context.Context, in *pb.MovePredicatePayload) error {
	// Note: Manish thinks it *should* be OK for a predicate receiver to not have to stop other
	// operations like snapshots and rollups. Note that this is the sender. This should stop other
	// operations.
	closer, err := groups().Node.startTask(opPredMove)
	if err != nil {
		return errors.Wrapf(err, "unable to start task opPredMove")
	}
	defer closer.Done()

	span := otrace.FromContext(ctx)

	pl := groups().Leader(in.DestGid)
	if pl == nil {
		return errors.Errorf("Unable to find a connection for group: %d\n", in.DestGid)
	}
	c := pb.NewWorkerClient(pl.Get())
	out, err := c.ReceivePredicate(ctx)
	if err != nil {
		return errors.Wrapf(err, "while calling ReceivePredicate")
	}

	txn := pstore.NewTransactionAt(in.ReadTs, false)
	defer txn.Discard()

	// Send schema first.
	schemaKey := x.SchemaKey(in.Predicate)
	item, err := txn.Get(schemaKey)
	switch {
	case err == badger.ErrKeyNotFound:
		// The predicate along with the schema could have been deleted. In that case badger would
		// return ErrKeyNotFound. We don't want to try and access item.Value() in that case.
	case err != nil:
		return err
	default:
		val, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}
		buf := z.NewBuffer(1024, "PredicateMove.MovePredicateHelper")
		defer buf.Release()

		kv := &bpb.KV{}
		kv.Key = schemaKey
		kv.Value = val
		kv.Version = in.ReadTs
		kv.UserMeta = []byte{item.UserMeta()}
		if in.SinceTs == 0 {
			// When doing Phase I of predicate move, receiver should clean the predicate.
			kv.StreamId = CleanPredicate
		}
		badger.KVToBuffer(kv, buf)

		kvs := &pb.KVS{
			Data: buf.Bytes(),
		}
		if err := out.Send(kvs); err != nil {
			return errors.Errorf("while sending: %v", err)
		}
	}

	itrs := make([]*badger.Iterator, x.WorkerConfig.Badger.NumGoroutines)
	if in.SinceTs > 0 {
		iopt := badger.DefaultIteratorOptions
		iopt.AllVersions = true
		for i := range itrs {
			itrs[i] = txn.NewIterator(iopt)
			defer itrs[i].Close()
		}
	}

	// sends all data except schema, schema key has different prefix
	// Read the predicate keys and stream to keysCh.
	stream := pstore.NewStreamAt(in.ReadTs)
	stream.LogPrefix = fmt.Sprintf("Sending predicate: [%s]", in.Predicate)
	stream.Prefix = x.PredicatePrefix(in.Predicate)
	stream.SinceTs = in.SinceTs
	stream.KeyToList = func(key []byte, itr *badger.Iterator) (*bpb.KVList, error) {
		bitr := itr
		// Use the threadlocal iterator because "itr" has the sinceTs set and
		// it will not be able to read all the data.
		if itrs[itr.ThreadId] != nil {
			bitr = itrs[itr.ThreadId]
			bitr.Seek(key)
		}

		// For now, just send out full posting lists, because we use delete markers to delete older
		// data in the prefix range. So, by sending only one version per key, and writing it at a
		// provided timestamp, we can ensure that these writes are above all the delete markers.
		l, err := posting.ReadPostingList(key, bitr)
		if err != nil {
			return nil, err
		}
		kvs, err := l.Rollup(itr.Alloc)
		for _, kv := range kvs {
			// Let's set all of them at this move timestamp.
			kv.Version = in.ReadTs
		}
		return &bpb.KVList{Kv: kvs}, err
	}
	stream.Send = func(buf *z.Buffer) error {
		kvs := &pb.KVS{
			Data: buf.Bytes(),
		}
		return out.Send(kvs)
	}
	span.Annotatef(nil, "Starting stream list orchestrate")
	if err := stream.Orchestrate(out.Context()); err != nil {
		return err
	}

	payload, err := out.CloseAndRecv()
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
