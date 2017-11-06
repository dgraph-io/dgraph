/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package worker

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"strconv"
	"sync"
	"sync/atomic"

	"golang.org/x/net/context"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/x"
)

var (
	errEmptyPredicate = x.Errorf("Predicate not specified")
	errNotLeader      = x.Errorf("Server is not leader of this group")
	emptyPayload      = protos.Payload{}
)

// size of kvs won't be too big, we would take care before proposing.
func populateKeyValues(ctx context.Context, kvs []*protos.KV) error {
	// No new deletion/background cleanup would start after we start streaming tablet,
	// so all the proposals for a particular tablet would atmost wait for deletion of
	// single tablet.
	groups().waitForBackgroundDeletion()
	x.Printf("Writing %d keys\n", len(kvs))

	var hasError uint32
	var wg sync.WaitGroup
	wg.Add(len(kvs))
	for _, kv := range kvs {
		txn := pstore.NewTransactionAt(math.MaxUint64, true)
		txn.Set(kv.Key, kv.Val, kv.UserMeta[0])
		txn.CommitAt(kv.Version, func(err error) {
			if err != nil {
				atomic.StoreUint32(&hasError, 1)
			}
			wg.Done()
		})
		txn.Discard()
	}
	if hasError > 0 {
		return x.Errorf("Error while writing to badger")
	}
	return nil
}

func movePredicateHelper(ctx context.Context, predicate string, gid uint32) error {
	pl := groups().Leader(gid)
	if pl == nil {
		return x.Errorf("Unable to find a connection for groupd: %d\n", gid)
	}
	c := protos.NewWorkerClient(pl.Get())
	stream, err := c.ReceivePredicate(ctx)
	if err != nil {
		return err
	}

	count := 0
	sendPl := func(stream protos.Worker_ReceivePredicateClient, l *posting.List) error {
		kv, err := l.MarshalToKv()
		if err != nil {
			return err
		}
		return stream.Send(kv)
	}

	// sends all data except schema, schema key has different prefix
	txn := pstore.NewTransactionAt(math.MaxUint64, false)
	defer txn.Discard()
	iterOpts := badger.DefaultIteratorOptions
	iterOpts.AllVersions = true
	it := txn.NewIterator(iterOpts)
	defer it.Close()

	prefix := x.PredicatePrefix(predicate)
	var prevKey []byte
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		item := it.Item()
		key := item.Key()
		if bytes.Equal(key, prevKey) {
			it.Next()
			continue
		}
		if cap(prevKey) < len(key) {
			prevKey = make([]byte, len(key))
		}
		copy(prevKey, key)
		l, err := posting.ReadPostingList(key, it)
		if err != nil {
			return err
		}
		count++
		if err := sendPl(stream, l); err != nil {
			return err
		}
	}

	// send schema
	schemaKey := x.SchemaKey(predicate)
	item, err := txn.Get(schemaKey)
	if err != nil && err != badger.ErrKeyNotFound {
		return err
	}
	val, err := item.Value()
	if err != nil {
		return err
	}
	kv := &protos.KV{}
	kv.Key = schemaKey
	kv.Val = val
	kv.Version = 1
	kv.UserMeta = []byte{item.UserMeta()}
	if err := stream.Send(kv); err != nil {
		return err
	}
	count++
	x.Printf("Sent %d number of keys for predicate %v\n", count, predicate)

	payload, err := stream.CloseAndRecv()
	if err != nil {
		return err
	}
	recvCount, err := strconv.Atoi(string(payload.Data))
	if err != nil {
		return err
	}
	if recvCount != count {
		return x.Errorf("Sent count %d doesn't match with received %d", count, recvCount)
	}
	return nil
}

func batchAndProposeKeyValues(ctx context.Context, kvs chan *protos.KV) error {
	n := groups().Node
	proposal := &protos.Proposal{}
	size := 0
	firstKV := true
	var predicate string

	for kv := range kvs {
		if size >= 32<<20 { // 32 MB
			if err := n.ProposeAndWait(ctx, proposal); err != nil {
				return err
			}
			proposal.Kv = proposal.Kv[:0]
			size = 0
			continue
		}

		if firstKV {
			firstKV = false
			pk := x.Parse(kv.Key)
			predicate = pk.Attr
			// Delete on all nodes.
			p := &protos.Proposal{CleanPredicate: pk.Attr}
			err := groups().Node.ProposeAndWait(ctx, p)
			if err != nil {
				x.Printf("Error while cleaning predicate %v %v\n", pk.Attr, err)
			}
		}
		proposal.Kv = append(proposal.Kv, kv)
		size = size + len(kv.Key) + len(kv.Val)
	}
	// Propose remaining keys.
	if err := n.ProposeAndWait(ctx, proposal); err != nil {
		return err
	}
	return schema.Load(predicate)
}

// Returns count which can be used to verify whether we have moved all keys
// for a predicate or not.
func (w *grpcWorker) ReceivePredicate(stream protos.Worker_ReceivePredicateServer) error {
	// Values can be pretty big so having less buffer is safer.
	kvs := make(chan *protos.KV, 10)
	che := make(chan error, 1)
	// We can use count to check the number of posting lists returned in tests.
	count := 0
	ctx := stream.Context()
	payload := &protos.Payload{}

	go func() {
		// Takes care of throttling and batching.
		che <- batchAndProposeKeyValues(ctx, kvs)
	}()
	for {
		kv, err := stream.Recv()
		if err == io.EOF {
			payload.Data = []byte(fmt.Sprintf("%d", count))
			stream.SendAndClose(payload)
			break
		}
		if err != nil {
			x.Printf("received %d number of keys, err %v\n", count, err)
			return err
		}
		count++

		select {
		case kvs <- kv:
		case <-ctx.Done():
			close(kvs)
			<-che
			x.Printf("received %d number of keys, context deadline\n", count)
			return ctx.Err()
		case err := <-che:
			x.Printf("received %d number of keys, error %v\n", count, err)
			return err
		}
	}
	close(kvs)
	err := <-che
	x.Printf("received %d number of keys, error %v\n", count, err)
	return err
}

func (w *grpcWorker) MovePredicate(ctx context.Context,
	in *protos.MovePredicatePayload) (*protos.Payload, error) {
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

	// Ensures that all future mtuations beyond this point are rejected
	if err := n.ProposeAndWait(ctx, &protos.Proposal{State: in.State}); err != nil {
		return &emptyPayload, err
	}
	tctxs := posting.Txns().Iterate(func(key []byte) bool {
		pk := x.Parse(key)
		return pk.Attr == in.Predicate
	})
	if len(tctxs) > 0 {
		go tryAbortTransactions(tctxs)
		return &emptyPayload, posting.ErrConflict
	}
	// We iterate over badger, so need to flush and wait for sync watermark to catch up.
	n.applyAllMarks(ctx)

	err := movePredicateHelper(ctx, in.Predicate, in.DestGroupId)
	return &emptyPayload, err
}
