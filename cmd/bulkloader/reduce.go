package main

import (
	"bytes"
	"sync/atomic"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/bp128"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
)

func reduce(batch []*protos.FlatPosting, kv *badger.KV, prog *progress) {
	var currentKey []byte
	var uids []uint64
	pl := new(protos.PostingList)
	var entries []*badger.Entry

	outputPostingList := func() {
		atomic.AddInt64(&prog.reduceKeyCount, 1)

		// For a UID-only posting list, the badger value is a delta packed UID
		// list. The UserMeta indicates to treat the value as a delta packed
		// list when the value is read by dgraph.  For a value posting list,
		// the full protos.Posting type is used (which internally contains the
		// delta packed UID list).
		e := &badger.Entry{Key: currentKey}
		if len(pl.Postings) == 0 {
			e.Value = bp128.DeltaPack(uids)
			e.UserMeta = 0x01
		} else {
			var err error
			pl.Uids = bp128.DeltaPack(uids)
			e.Value, err = pl.Marshal()
			x.Check(err)
		}
		entries = append(entries, e)

		uids = uids[:0]
		pl.Reset()
	}

	for _, posting := range batch {
		atomic.AddInt64(&prog.reduceEdgeCount, 1)

		if bytes.Compare(posting.Key, currentKey) != 0 && currentKey != nil {
			outputPostingList()
		}
		currentKey = posting.Key

		if posting.Full == nil {
			uids = append(uids, posting.UidOnly)
		} else {
			uids = append(uids, posting.Full.Uid)
			pl.Postings = append(pl.Postings, posting.Full)
		}
	}
	outputPostingList()

	err := kv.BatchSet(entries)
	x.Check(err)
	for _, e := range entries {
		x.Check(e.Error)
	}
}
