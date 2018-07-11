/*
 * Copyright 2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package posting

import (
	"bytes"
	"math"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/bp128"
	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/x"
)

// constructs the posting list from the disk using the passed iterator.
// Use forward iterator with allversions enabled in iter options.
//
// key would now be owned by the posting list. So, ensure that it isn't reused
// elsewhere.
func ReadPostingList(key []byte, it *badger.Iterator) (*List, error) {
	l := new(List)
	l.key = key
	l.mutationMap = make(map[uint64]*intern.PostingList)
	l.activeTxns = make(map[uint64]struct{})
	l.plist = new(intern.PostingList)

	// Iterates from highest Ts to lowest Ts
	for it.Valid() {
		item := it.Item()
		if item.IsDeletedOrExpired() {
			// Don't consider any more versions.
			break
		}
		if !bytes.Equal(item.Key(), l.key) {
			break
		}
		if l.commitTs == 0 {
			l.commitTs = item.Version()
		}

		val, err := item.Value()
		if err != nil {
			return nil, err
		}
		if item.UserMeta()&BitCompletePosting > 0 {
			if err := unmarshalOrCopy(l.plist, item); err != nil {
				return nil, err
			}
			l.minTs = item.Version()
			// No need to do Next here. The outer loop can take care of skipping more versions of
			// the same key.
			break
		}
		if item.UserMeta()&BitDeltaPosting > 0 {
			pl := &intern.PostingList{}
			x.Check(pl.Unmarshal(val))
			pl.Commit = item.Version()
			for _, mpost := range pl.Postings {
				// commitTs, startTs are meant to be only in memory, not
				// stored on disk.
				mpost.CommitTs = item.Version()
			}
			l.mutationMap[pl.Commit] = pl
		} else {
			x.Fatalf("unexpected meta: %d", item.UserMeta())
		}
		if item.DiscardEarlierVersions() {
			break
		}
		it.Next()
	}
	return l, nil
}

// Generate a new PostingList by merging the immutable layer with the deltas.
func (l *List) Rollup() (*intern.PostingList, error) {
	l.RLock()
	defer l.RUnlock()
	// l.AssertLock()
	final := new(intern.PostingList)
	var bp bp128.BPackEncoder
	buf := make([]uint64, 0, bp128.BlockSize)

	maxCommitTs := l.commitTs
	// Pick all committed entries
	// TODO: Do we need this assert here?
	// x.AssertTrue(l.minTs <= l.commitTs)
	err := l.iterate(math.MaxUint64, 0, func(p *intern.Posting) bool {
		// iterate already takes care of not returning entries whose commitTs is above l.commitTs.
		// So, we don't need to do any filtering here. In fact, doing filtering here could result
		// in a bug.
		buf = append(buf, p.Uid)
		if len(buf) == bp128.BlockSize {
			bp.PackAppend(buf)
			buf = buf[:0]
		}

		// We want to add the posting if it has facets or has a value.
		if p.Facets != nil || p.PostingType != intern.Posting_REF || len(p.Label) != 0 {
			// I think it's okay to take the pointer from the iterator, because we have a lock
			// over List; which won't be released until final has been marshalled. Thus, the
			// underlying data wouldn't be changed.
			final.Postings = append(final.Postings, p)
		}
		maxCommitTs = x.Max(maxCommitTs, p.CommitTs)
		return true
	})
	if err != nil {
		return nil, err
	}
	if len(buf) > 0 {
		bp.PackAppend(buf)
	}
	if sz := bp.Size(); sz > 0 {
		final.Uids = make([]byte, sz)
		// TODO: Add bytes method
		// What does this TODO above mean?
		bp.WriteTo(final.Uids)
	}
	final.Commit = maxCommitTs
	return final, nil
	// // Keep all uncommited Entries or postings with commitTs > l.commitTs
	// // in mutation map. Discard all else.
	// for startTs, plist := range l.mutationMap {
	// 	cl := plist.Commit
	// 	if cl == 0 || cl > l.commitTs {
	// 		// Keep this.
	// 	} else {
	// 		delete(l.mutationMap, startTs)
	// 	}
	// }

	// l.minTs = l.commitTs
	// l.plist = final
	// l.numCommits = 0
	// atomic.StoreInt32(&l.estimatedSize, l.calculateSize())
	// return nil
}

func scanKeysToRoll(snapshotTs uint64, keyChan chan *intern.KVS) error {
	txn := pstore.NewTransactionAt(snapshotTs, false)
	defer txn.Discard()

	opts := badger.DefaultIteratorOptions
	opts.AllVersions = false
	opts.PrefetchValues = false

	itr := txn.NewIterator(opts)
	defer itr.Close()

	kvs := new(intern.KVS)
	// We just pick up the first version of each key. If it is already complete, we skip.
	// If it is a delta, we add that to KVS, to be rolled up.
	for itr.Rewind(); itr.Valid(); itr.Next() {
		item := itr.Item()
		pk := x.Parse(item.Key())
		if pk.IsSchema() {
			// Skip schema.
			continue
		}
		if item.UserMeta()&BitCompletePosting > 0 {
			// First version is complete. Therefore, skip.

		} else if item.UserMeta()&BitDeltaPosting > 0 {
			kvs.Kv = append(kvs.Kv, &intern.KV{Key: item.KeyCopy(nil)})
			if len(kvs.Kv) >= 100 {
				keyChan <- kvs
				kvs = new(intern.KVS)
			}

		} else {
			x.Fatalf("unexpected meta: %d", item.UserMeta())
		}
	}
	if len(kvs.Kv) > 0 {
		keyChan <- kvs
	}
	return nil
}
