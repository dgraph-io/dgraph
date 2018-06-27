/*
 * Copyright 2016-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package worker

import (
	"bytes"
	"context"
	"io"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/badger"
	"golang.org/x/net/trace"

	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/x"
	humanize "github.com/dustin/go-humanize"
)

const (
	// MB represents a megabyte.
	MB = 1 << 20
)

// writeBatch performs a batch write of key value pairs to BadgerDB.
func writeBatch(ctx context.Context, pstore *badger.ManagedDB, kvChan chan *intern.KVS, che chan error) {
	var bytesWritten uint64
	t := time.NewTicker(time.Second)
	defer t.Stop()
	go func() {
		now := time.Now()
		for range t.C {
			dur := time.Since(now)
			durSec := uint64(dur.Seconds())
			if durSec == 0 {
				continue
			}
			speed := bytesWritten / durSec
			x.Printf("Getting SNAPSHOT: Time elapsed: %v, bytes written: %s, %s/s\n",
				x.FixedDuration(dur), humanize.Bytes(bytesWritten), humanize.Bytes(speed))
		}
	}()

	var hasError int32
	var wg sync.WaitGroup // to wait for all callbacks to return
	for kvs := range kvChan {
		for _, kv := range kvs.Kv {
			if kv.Version == 0 {
				// Ignore this one. Otherwise, we'll get ErrManagedDB back, because every Commit in
				// managed DB must have a valid commit ts.
				continue
			}
			txn := pstore.NewTransactionAt(math.MaxUint64, true)
			bytesWritten += uint64(kv.Size())
			x.Check(txn.SetWithMeta(kv.Key, kv.Val, kv.UserMeta[0]))
			wg.Add(1)
			rerr := txn.CommitAt(kv.Version, func(err error) {
				// We don't care about exact error
				defer wg.Done()
				if err != nil {
					x.Printf("Error while committing kv to badger %v\n", err)
					atomic.StoreInt32(&hasError, 1)
				}
			})
			x.Check(rerr)
		}
	}
	wg.Wait()

	if atomic.LoadInt32(&hasError) == 0 {
		che <- nil
	} else {
		che <- x.Errorf("Error while writing to badger")
	}
}

// populateShard gets data for a shard from the leader and writes it to BadgerDB on the follower.
func (n *node) populateShard(ps *badger.ManagedDB, pl *conn.Pool) (int, error) {
	conn := pl.Get()
	c := intern.NewWorkerClient(conn)

	n.RLock()
	ctx := n.ctx
	group := n.gid
	stream, err := c.PredicateAndSchemaData(ctx,
		&intern.SnapshotMeta{
			ClientTs: n.RaftContext.SnapshotTs,
			GroupId:  group,
		})
	n.RUnlock()
	if err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf(err.Error())
		}
		return 0, err
	}
	if tr, ok := trace.FromContext(ctx); ok {
		tr.LazyPrintf("Streaming data for group: %v", group)
	}

	kvs, err := stream.Recv()
	if err != nil {
		return 0, err
	}

	x.AssertTrue(len(kvs.Kv) == 1)
	ikv := kvs.Kv[0]
	// First key has the snapshot ts from the leader.
	x.AssertTrue(bytes.Equal(ikv.Key, []byte("min_ts")))
	n.Lock()
	n.RaftContext.SnapshotTs = ikv.Version
	n.Unlock()

	kvChan := make(chan *intern.KVS, 1000)
	che := make(chan error, 1)
	go writeBatch(ctx, ps, kvChan, che)

	// We can use count to check the number of posting lists returned in tests.
	count := 0
	for {
		kvs, err = stream.Recv()
		if err == io.EOF {
			x.Printf("EOF has been reached\n")
			break
		}
		if err != nil {
			if tr, ok := trace.FromContext(ctx); ok {
				tr.LazyPrintf(err.Error())
			}
			close(kvChan)
			return count, err
		}
		// We check for errors, if there are no errors we send value to channel.
		select {
		case kvChan <- kvs:
			count += len(kvs.Kv)
			// OK
		case <-ctx.Done():
			if tr, ok := trace.FromContext(ctx); ok {
				tr.LazyPrintf("Context timed out while streaming group: %v", group)
			}
			close(kvChan)
			return 0, ctx.Err()
		case err := <-che:
			if tr, ok := trace.FromContext(ctx); ok {
				tr.LazyPrintf("Error while doing a batch write for group: %v", group)
			}
			close(kvChan)
			// Important: Don't put return count, err
			// There was a compiler bug which was fixed in 1.8.1
			// https://github.com/golang/go/issues/21722.
			// Probably should be ok to return count, err now
			return 0, err
		}
	}
	close(kvChan)

	if err := <-che; err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Error while doing a batch write for group: %v", group)
		}
		return count, err
	}
	if tr, ok := trace.FromContext(ctx); ok {
		tr.LazyPrintf("Streaming complete for group: %v", group)
	}
	x.Printf("Got %d keys. DONE.\n", count)
	return count, nil
}

func (w *grpcWorker) PredicateAndSchemaData(m *intern.SnapshotMeta, stream intern.Worker_PredicateAndSchemaDataServer) error {
	clientTs := m.ClientTs

	if !x.IsTestRun() {
		if !groups().ServesGroup(m.GroupId) {
			return x.Errorf("Group %d not served.", m.GroupId)
		}
		n := groups().Node
		if !n.AmLeader() {
			return x.Errorf("Not leader of group: %d", m.GroupId)
		}
	}

	// Any commit which happens in the future will have commitTs greater than
	// this.
	// TODO: Ensure all deltas have made to disk and read in memory before checking disk.
	// BUG: There's a bug here due to which a node which doesn't see any transactions, but has real
	// data fails to send that over, because of min_ts.
	min_ts := posting.Txns().MinTs() // Why are we not using last snapshot ts?
	x.Printf("Got min_ts: %d\n", min_ts)
	// snap, err := groups().Node.Snapshot()
	// if err != nil {
	// 	return err
	// }
	// index := snap.Metadata.Index

	// TODO: Why are we using MinTs() in the place when we should be using
	// snapshot index? This is wrong.

	// TODO: We are not using the snapshot time, because we don't store the
	// transaction timestamp in the snapshot. If we did, we'd just use that
	// instead of this. This causes issues if the server had received a snapshot
	// to begin with, but had no active transactions. Then mints is always zero,
	// hence nothing is read or sent in the stream.

	// UPDATE: This doesn't look too bad. So, we're keeping track of the
	// transaction timestamps on the side. And we're using those to figure out
	// what to stream here. The snapshot index is not really being used for
	// anything here.
	// This whole transaction tracking business is complex and must be
	// simplified to its essence.

	// Send ts as first KV.
	if err := stream.Send(&intern.KVS{
		Kv: []*intern.KV{&intern.KV{
			Key:     []byte("min_ts"),
			Version: min_ts,
		}},
	}); err != nil {
		return err
	}

	sl := streamLists{stream: stream, db: pstore}
	sl.chooseKey = func(key []byte, version uint64) bool {
		pk := x.Parse(key)
		return version > clientTs || pk.IsSchema()
	}
	sl.itemToKv = func(key []byte, itr *badger.Iterator) (*intern.KV, error) {
		item := itr.Item()
		pk := x.Parse(key)
		if pk.IsSchema() {
			val, err := item.ValueCopy(nil)
			if err != nil {
				return nil, err
			}
			kv := &intern.KV{
				Key:      key,
				Val:      val,
				UserMeta: []byte{item.UserMeta()},
				Version:  item.Version(),
			}
			return kv, nil
		}
		l, err := posting.ReadPostingList(key, itr)
		if err != nil {
			return nil, err
		}
		return l.MarshalToKv()
	}

	if err := sl.orchestrate(stream.Context(), "Sending SNAPSHOT", min_ts); err != nil {
		return err
	}

	if tr, ok := trace.FromContext(stream.Context()); ok {
		tr.LazyPrintf("Sent keys. Done.\n")
	}
	return nil
}
