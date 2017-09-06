package main

import (
	"bytes"
	"sync/atomic"

	"github.com/dgraph-io/dgraph/bp128"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
)

func reduce(batch []*protos.FlatPosting, prog *progress) {

	var currentKey []byte

	var uids []uint64
	pl := new(protos.PostingList)

	set := func(k, v []byte, m uint8) {
		// TODO: Placeholder
	}

	outputPostingList := func() {
		if len(pl.Postings) == 0 {
			set(currentKey, bp128.DeltaPack(uids), 0x01)
		} else {
			pl.Uids = bp128.DeltaPack(uids)
			plBuf, err := pl.Marshal()
			x.Check(err)
			set(currentKey, plBuf, 0x00)
		}

		uids = uids[:0]
		pl.Reset()
	}

	for _, posting := range batch {
		atomic.AddInt64(&prog.reduceEdgeCount, 1)

		if bytes.Compare(posting.Key, currentKey) != 0 && currentKey != nil {
			outputPostingList()
		}
		currentKey = posting.Key

		switch p := posting.Posting.(type) {
		case *protos.FlatPosting_UidPosting:
			uids = append(uids, p.UidPosting)
		case *protos.FlatPosting_FullPosting:
			uids = append(uids, p.FullPosting.Uid)
			pl.Postings = append(pl.Postings, p.FullPosting)
		default:
			x.AssertTruef(false, "unhandled posting type: %T", p)
		}

	}
	outputPostingList()
}
