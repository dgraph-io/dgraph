/*
 * Copyright 2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package worker

import (
	"bytes"
	"sync"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/x"
	humanize "github.com/dustin/go-humanize"
	"golang.org/x/net/context"
)

type kvStream interface {
	Send(*intern.KVS) error
}

type streamLists struct {
	stream    kvStream
	predicate string
	db        *badger.ManagedDB
	chooseKey func(key []byte, version uint64) bool
	itemToKv  func(key []byte, itr *badger.Iterator) (*intern.KV, error)
}

// keyRange is [start, end), including start, excluding end. Do ensure that the start,
// end byte slices are owned by keyRange struct.
type keyRange struct {
	start []byte
	end   []byte
}

func (sl *streamLists) orchestrate(ctx context.Context, prefix string, ts uint64) error {
	keyCh := make(chan keyRange, 100)     // Contains keys for posting lists.
	kvChan := make(chan *intern.KVS, 100) // Contains marshaled posting lists.
	errCh := make(chan error, 1)          // Stores error by consumeKeys.

	// Read the predicate keys and stream to keysCh.
	go sl.produceRanges(ctx, ts, keyCh)

	// Read the posting lists corresponding to keys and send to kvChan.
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := sl.produceKVs(ctx, ts, keyCh, kvChan); err != nil {
				select {
				case errCh <- err:
				default:
				}
			}
		}()
	}

	// Pick up key-values from kvChan and send to stream.
	kvErr := make(chan error, 1)
	go func() {
		kvErr <- sl.streamKVs(ctx, prefix, kvChan)
	}()
	wg.Wait()     // Wait for produceKVs to be over.
	close(kvChan) // Now we can close kvChan.

	select {
	case err := <-errCh: // Check error from produceKVs.
		return err
	default:
	}

	// Wait for key streaming to be over.
	if err := <-kvErr; err != nil {
		return err
	}
	return nil
}

func (sl *streamLists) produceRanges(ctx context.Context, ts uint64, keyCh chan keyRange) {
	var prefix []byte
	if len(sl.predicate) > 0 {
		prefix = x.PredicatePrefix(sl.predicate)
	}
	txn := sl.db.NewTransactionAt(ts, false)
	defer txn.Discard()
	iterOpts := badger.DefaultIteratorOptions
	iterOpts.PrefetchValues = false
	it := txn.NewIterator(iterOpts)
	defer it.Close()

	var start []byte
	var size int64
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		item := it.Item()
		if len(start) == 0 {
			start = item.KeyCopy(nil)
		}

		size += item.EstimatedSize()
		if size > 4*MB {
			kr := keyRange{start: start, end: item.KeyCopy(nil)}
			keyCh <- kr
			start = item.KeyCopy(nil)
			size = 0
		}
	}
	if len(start) > 0 {
		keyCh <- keyRange{start: start}
	}
	close(keyCh)
}

func (sl *streamLists) produceKVs(ctx context.Context, ts uint64,
	keyCh chan keyRange, kvChan chan *intern.KVS) error {
	var prefix []byte
	if len(sl.predicate) > 0 {
		prefix = x.PredicatePrefix(sl.predicate)
	}

	txn := sl.db.NewTransactionAt(ts, false)
	defer txn.Discard()
	iterate := func(kr keyRange) error {
		iterOpts := badger.DefaultIteratorOptions
		// iterOpts.PrefetchSize = 10
		iterOpts.AllVersions = true
		iterOpts.PrefetchValues = false
		it := txn.NewIterator(iterOpts)
		defer it.Close()

		kvs := new(intern.KVS)
		var prevKey []byte
		for it.Seek(kr.start); it.ValidForPrefix(prefix); {
			item := it.Item()
			if bytes.Equal(item.Key(), prevKey) {
				it.Next()
				continue
			}
			prevKey = append(prevKey[:0], item.Key()...)

			// Check if we reached the end of the key range.
			if len(kr.end) > 0 && bytes.Compare(item.Key(), kr.end) >= 0 {
				break
			}
			// Check if we should pick this key.
			if sl.chooseKey != nil && !sl.chooseKey(item.Key(), item.Version()) {
				continue
			}

			// Now convert to key value.
			kv, err := sl.itemToKv(item.KeyCopy(nil), it)
			if err != nil {
				return err
			}
			kvs.Kv = append(kvs.Kv, kv)
		}
		if len(kvs.Kv) > 0 {
			kvChan <- kvs
		}
		return nil
	}

	for {
		select {
		case kr, ok := <-keyCh:
			if !ok {
				// Done with the keys.
				return nil
			}
			if err := iterate(kr); err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (sl *streamLists) streamKVs(ctx context.Context, prefix string,
	kvChan chan *intern.KVS) error {
	var count int
	var bytesSent uint64
	t := time.NewTicker(time.Second)
	defer t.Stop()
	now := time.Now()

	slurp := func(batch *intern.KVS) error {
	loop:
		for {
			select {
			case kvs, ok := <-kvChan:
				if !ok {
					break loop
				}
				x.AssertTrue(kvs != nil)
				batch.Kv = append(batch.Kv, kvs.Kv...)
			default:
				break loop
			}
		}
		sz := uint64(batch.Size())
		bytesSent += sz
		count += len(batch.Kv)
		t := time.Now()
		if err := sl.stream.Send(batch); err != nil {
			return err
		}
		x.Printf("Sent batch of size: %s in %v.\n", humanize.Bytes(sz), time.Since(t))
		return nil
	}

outer:
	for {
		var batch *intern.KVS
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-t.C:
			dur := time.Since(now)
			durSec := uint64(dur.Seconds())
			if durSec == 0 {
				continue
			}
			speed := bytesSent / durSec
			x.Printf("%s Time elapsed: %v, bytes sent: %s, speed: %v/sec\n",
				prefix, x.FixedDuration(dur), humanize.Bytes(bytesSent), humanize.Bytes(speed))

		case kvs, ok := <-kvChan:
			if !ok {
				break outer
			}
			x.AssertTrue(kvs != nil)
			batch = kvs
			if err := slurp(batch); err != nil {
				return err
			}
		}
	}

	x.Printf("%s Sent %d (+1 maybe for schema) keys\n", prefix, count)
	return nil
}
