/*
 * Copyright 2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package worker

import (
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
	chooseKey func(key []byte, version uint64) bool
	itemToKv  func(key string, itr *badger.Iterator) (*intern.KV, error)
}

func (sl *streamLists) orchestrate(ctx context.Context, prefix string, txn *badger.Txn) error {
	keysCh := make(chan string, 1000)     // Contains keys for posting lists.
	kvChan := make(chan *intern.KV, 1000) // Contains marshaled posting lists.
	errCh := make(chan error, 1)          // Stores error by consumeKeys.

	// Read the predicate keys and stream to keysCh.
	go sl.produceKeys(ctx, txn, keysCh)

	// Read the posting lists corresponding to keys and send to kvChan.
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := sl.produceKVs(ctx, txn, keysCh, kvChan); err != nil {
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

// TODO: Change this so the first phase generates a key range, and passes it onto the second phase.
// The 2nd phase would then use the iterator to iterate over that key range, generating the PLs, and
// sending them over to kvChan. That way we reduce the Iterator::Seeks significantly, and can use
// prefetch as well.
func (sl *streamLists) produceKeys(ctx context.Context, txn *badger.Txn, keys chan string) {
	var prefix []byte
	if len(sl.predicate) > 0 {
		prefix = x.PredicatePrefix(sl.predicate)
	}
	iterOpts := badger.DefaultIteratorOptions
	iterOpts.PrefetchValues = false
	it := txn.NewIterator(iterOpts)
	defer it.Close()

	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		item := it.Item()
		key := item.Key()

		if sl.chooseKey == nil || sl.chooseKey(key, item.Version()) {
			keys <- string(key)
		}
	}
	close(keys)
}

func (sl *streamLists) produceKVs(ctx context.Context, txn *badger.Txn,
	keys chan string, kvChan chan *intern.KV) error {
	for {
		select {
		case key, ok := <-keys:
			if !ok {
				// Done with the keys.
				return nil
			}
			iterOpts := badger.DefaultIteratorOptions
			// We don't know how many values do we really need to read this PL. We could stop at
			// just one. So, let's not get more than necessary.
			iterOpts.PrefetchValues = false
			iterOpts.AllVersions = true
			it := txn.NewIterator(iterOpts)
			it.Seek([]byte(key))
			if it.Valid() {
				kv, err := sl.itemToKv(key, it)
				it.Close()
				if err != nil {
					return err
				}
				kvChan <- kv
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (sl *streamLists) streamKVs(ctx context.Context, prefix string, kvChan chan *intern.KV) error {
	var count, batchSize int
	var bytesSent uint64
	kvs := &intern.KVS{}
	t := time.NewTicker(time.Second)
	defer t.Stop()
	now := time.Now()

outer:
	for {
		select {
		case kv, ok := <-kvChan:
			if !ok {
				break outer
			}
			kvs.Kv = append(kvs.Kv, kv)
			batchSize += kv.Size()
			bytesSent += uint64(kv.Size())
			count++
			if batchSize < 16*MB {
				continue
			}
			if err := sl.stream.Send(kvs); err != nil {
				return err
			}
			kvs = &intern.KVS{}
			batchSize = 0

		case <-t.C:
			dur := time.Since(now)
			speed := bytesSent / uint64(dur.Seconds())
			x.Printf("%s Time elapsed: %v, bytes sent: %s, speed: %v/sec\n",
				prefix, x.FixedDuration(dur), humanize.Bytes(bytesSent), humanize.Bytes(speed))

		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if len(kvs.Kv) > 0 {
		if err := sl.stream.Send(kvs); err != nil {
			return err
		}
	}
	x.Printf("%s Sent %d (+1 maybe for schema) keys\n", prefix, count)
	return nil
}
