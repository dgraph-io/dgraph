package main

import (
	"bytes"
	"sync/atomic"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/bp128"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
)

type reducer struct {
	*state
	input     <-chan shuffleOutput
	writesThr *x.Throttle
}

func (r *reducer) run() {
	thr := x.NewThrottle(r.opt.NumGoroutines)
	for reduceJob := range r.input {
		thr.Start()
		NumReducers.Add(1)
		NumQueuedReduceJobs.Add(-1)
		r.writesThr.Start()
		go func(job shuffleOutput) {
			r.reduce(job)
			thr.Done()
			NumReducers.Add(-1)
		}(reduceJob)
	}
	thr.Wait()
	r.writesThr.Wait()
}

func (r *reducer) reduce(job shuffleOutput) {
	var currentKey []byte
	var uids []uint64
	pl := new(protos.PostingList)
	var entries []*badger.Entry

	outputPostingList := func() {
		atomic.AddInt64(&r.prog.reduceKeyCount, 1)

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
			pl.Uids = bp128.DeltaPack(uids)
			var err error
			e.Value, err = pl.Marshal()
			x.Check(err)
		}
		entries = append(entries, e)

		uids = uids[:0]
		pl.Reset()
	}

	for _, mapEntry := range job.mapEntries {
		atomic.AddInt64(&r.prog.reduceEdgeCount, 1)

		if bytes.Compare(mapEntry.Key, currentKey) != 0 && currentKey != nil {
			outputPostingList()
		}
		currentKey = mapEntry.Key

		uid := mapEntry.Uid
		if mapEntry.Posting != nil {
			uid = mapEntry.Posting.Uid
		}
		if len(uids) > 0 && uids[len(uids)-1] == uid {
			continue
		}
		uids = append(uids, uid)
		if mapEntry.Posting != nil {
			pl.Postings = append(pl.Postings, mapEntry.Posting)
		}
	}
	outputPostingList()

	NumBadgerWrites.Add(1)
	job.kv.BatchSetAsync(entries, func(err error) {
		x.Check(err)
		for _, e := range entries {
			x.Check(e.Error)
		}
		NumBadgerWrites.Add(-1)
		r.writesThr.Done()
	})
}
