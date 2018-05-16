/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
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
	"time"

	"golang.org/x/net/context"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/x"
	humanize "github.com/dustin/go-humanize"
)

var (
	errEmptyPredicate = x.Errorf("Predicate not specified")
	errNotLeader      = x.Errorf("Server is not leader of this group")
	emptyPayload      = api.Payload{}
)

// size of kvs won't be too big, we would take care before proposing.
func populateKeyValues(ctx context.Context, kvs []*intern.KV) error {
	// No new deletion/background cleanup would start after we start streaming tablet,
	// so all the proposals for a particular tablet would atmost wait for deletion of
	// single tablet.
	groups().waitForBackgroundDeletion()
	x.Printf("Writing %d keys\n", len(kvs))

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
		if err := txn.SetWithMeta(kv.Key, kv.Val, kv.UserMeta[0]); err != nil {
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
	c := intern.NewWorkerClient(pl.Get())
	stream, err := c.ReceivePredicate(ctx)
	if err != nil {
		return fmt.Errorf("While calling ReceivePredicate: %+v", err)
	}

	var bytesSent uint64
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	go func() {
		now := time.Now()
		for range t.C {
			dur := time.Since(now)
			speed := bytesSent / uint64(dur.Seconds())
			x.Printf("Sending predicate: [%v] Time elapsed: %v, bytes sent: %s, speed: %v/sec\n",
				predicate, x.FixedDuration(dur), humanize.Bytes(bytesSent), humanize.Bytes(speed))
		}
	}()

	count := 0
	batchSize := 0
	kvs := &intern.KVS{}
	// sends all data except schema, schema key has different prefix
	prefix := x.PredicatePrefix(predicate)
	var prevKey []byte
	txn := pstore.NewTransactionAt(math.MaxUint64, false)
	defer txn.Discard()
	iterOpts := badger.DefaultIteratorOptions
	iterOpts.AllVersions = true
	it := txn.NewIterator(iterOpts)
	defer it.Close()
	for it.Seek(prefix); it.ValidForPrefix(prefix); {
		item := it.Item()
		key := item.Key()

		if bytes.Equal(key, prevKey) {
			it.Next()
			continue
		}
		prevKey = append(prevKey[:0], key...)

		l, err := posting.ReadPostingList(item.KeyCopy(nil), it)
		if err != nil {
			return err
		}

		kv, err := l.MarshalToKv()
		if err != nil {
			return err
		}
		kvs.Kv = append(kvs.Kv, kv)
		batchSize += kv.Size()
		bytesSent += uint64(kv.Size())
		count++
		if batchSize < 4*MB {
			continue
		}
		if err := stream.Send(kvs); err != nil {
			return err
		}
		batchSize = 0
		kvs = &intern.KVS{}
	}

	// Send schema if present.
	schemaKey := x.SchemaKey(predicate)
	item, err := txn.Get(schemaKey)
	if err == badger.ErrKeyNotFound {
		// The predicate along with the schema could have been deleted. In that case badger would
		// return ErrKeyNotFound. We don't want to try and access item.Value() in that case.
	} else if err != nil {
		return err
	} else {
		val, err := item.Value()
		if err != nil {
			return err
		}
		kv := &intern.KV{}
		kv.Key = schemaKey
		kv.Val = val
		kv.Version = 1
		kv.UserMeta = []byte{item.UserMeta()}
		kvs.Kv = append(kvs.Kv, kv)
		batchSize += kv.Size()
		bytesSent += uint64(kv.Size())
		count++
	}

	if batchSize > 0 {
		if err := stream.Send(kvs); err != nil {
			return err
		}
	}
	x.Printf("Sent %d keys for predicate %v\n", count, predicate)

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

func batchAndProposeKeyValues(ctx context.Context, kvs chan *intern.KVS) error {
	x.Println("Receiving predicate. Batching and proposing key values")
	n := groups().Node
	proposal := &intern.Proposal{}
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
				p := &intern.Proposal{CleanPredicate: pk.Attr}
				x.Printf("Predicate being received: %v", pk.Attr)
				err := groups().Node.proposeAndWait(ctx, p)
				if err != nil {
					x.Printf("Error while cleaning predicate %v %v\n", pk.Attr, err)
					return err
				}
			}
			proposal.Kv = append(proposal.Kv, kv)
			size += len(kv.Key) + len(kv.Val)
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
func (w *grpcWorker) ReceivePredicate(stream intern.Worker_ReceivePredicateServer) error {
	// Values can be pretty big so having less buffer is safer.
	kvs := make(chan *intern.KVS, 10)
	che := make(chan error, 1)
	// We can use count to check the number of posting lists returned in tests.
	count := 0
	ctx := stream.Context()
	payload := &api.Payload{}

	x.Printf("Got ReceivePredicate. Group: %d. Am leader: %v",
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
			x.Printf("Received %d keys. Error in loop: %v\n", count, err)
			return err
		}
		count += len(kvBatch.Kv)

		select {
		case kvs <- kvBatch:
		case <-ctx.Done():
			close(kvs)
			<-che
			x.Printf("Received %d keys. Context deadline\n", count)
			return ctx.Err()
		case err := <-che:
			x.Printf("Received %d keys. Error via channel: %v\n", count, err)
			return err
		}
	}
	close(kvs)
	err := <-che
	x.Printf("Proposed %d keys. Error: %v\n", count, err)
	return err
}

func (w *grpcWorker) MovePredicate(ctx context.Context,
	in *intern.MovePredicatePayload) (*api.Payload, error) {
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

	x.Printf("Move predicate request for pred: [%v], src: [%v], dst: [%v]\n", in.Predicate,
		in.SourceGroupId, in.DestGroupId)

	// Ensures that all future mutations beyond this point are rejected.
	if err := n.proposeAndWait(ctx, &intern.Proposal{State: in.State}); err != nil {
		return &emptyPayload, err
	}
	for i := 0; ; i++ {
		x.Printf("Trying to abort pending mutations. Loop: %d", i)
		tctxs := posting.Txns().Iterate(func(key []byte) bool {
			pk := x.Parse(key)
			return pk.Attr == in.Predicate
		})
		if len(tctxs) == 0 {
			break
		}
		tryAbortTransactions(tctxs)
	}
	// We iterate over badger, so need to flush and wait for sync watermark to catch up.
	n.applyAllMarks(ctx)

	err := movePredicateHelper(ctx, in.Predicate, in.DestGroupId)
	return &emptyPayload, err
}
