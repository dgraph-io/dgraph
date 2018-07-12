/*
 * Copyright 2016-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package worker

import (
	"context"
	"errors"
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

	ctx := n.ctx
	snap, err := n.Snapshot()
	if err != nil {
		return 0, err
	}

	// TODO: Before we write anything, we should drop all the data stored in ps.
	stream, err := c.StreamSnapshot(ctx, snap)
	if err != nil {
		return 0, err
	}

	kvs, err := stream.Recv()
	if err != nil {
		return 0, err
	}

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
			close(kvChan)
			return count, err
		}
		// We check for errors, if there are no errors we send value to channel.
		select {
		case kvChan <- kvs:
			count += len(kvs.Kv)
			// OK
		case <-ctx.Done():
			close(kvChan)
			return 0, ctx.Err()
		case err := <-che:
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
		return count, err
	}
	x.Printf("Got %d keys. DONE.\n", count)
	return count, nil
}

func (w *grpcWorker) StreamSnapshot(reqSnap *intern.Snapshot, stream intern.Worker_StreamSnapshotServer) error {
	n := groups().Node
	if !x.IsTestRun() {
		if !n.AmLeader() {
			return errNotLeader
		}
	}
	snap, err := n.Snapshot()
	if err != nil {
		return err
	}
	x.Printf("Got StreamSnapshot request. Mine: %+v. Requested: %+v\n", snap, reqSnap)
	if snap.Index != reqSnap.Index || snap.MinPendingStartTs != reqSnap.MinPendingStartTs {
		return errors.New("Mismatching snapshot request")
	}

	// We have matched the requested snapshot with what this node, the leader of the group, has.
	// Now, we read all the posting lists stored below MinPendingStartTs, and stream them over the
	// wire. The MinPendingStartTs stored as part of creation of Snapshot, tells us the readTs to
	// use to read from the store. Anything below this timestamp, we should pick up.
	//
	// TODO: This would also pick up schema updates done "after" the snapshot index. Guess that
	// might be OK. Otherwise, we'd want to version the schemas as well. Currently, they're stored
	// at timestamp=1.

	// TODO: Confirm that the bug is now over.
	// BUG: There's a bug here due to which a node which doesn't see any transactions, but has real
	// data fails to send that over, because of min_ts.

	sl := streamLists{stream: stream, db: pstore}
	sl.chooseKey = func(key []byte, version uint64) bool {
		// Pick all keys.
		return true
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
		// We should keep reading the posting list instead of copying the key-value pairs directly,
		// to consolidate the read logic in one place. This is more robust than trying to replicate
		// a simplified key-value copy logic here, which still understands the BitCompletePosting
		// and other bits about the way we store posting lists.
		l, err := posting.ReadPostingList(key, itr)
		if err != nil {
			return nil, err
		}
		return l.MarshalToKv()
	}

	readTs := snap.MinPendingStartTs - 1
	if err := sl.orchestrate(stream.Context(), "Sending SNAPSHOT", readTs); err != nil {
		return err
	}

	if tr, ok := trace.FromContext(stream.Context()); ok {
		tr.LazyPrintf("Sent keys. Done.\n")
	}
	return nil
}
