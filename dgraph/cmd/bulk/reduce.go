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

package bulk

import (
	"bytes"
	"sync/atomic"

	"github.com/dgraph-io/dgraph/bp128"
	"github.com/dgraph-io/dgraph/posting"
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
	txn := job.db.NewTransactionAt(r.state.writeTs, true)

	outputPostingList := func() {
		atomic.AddInt64(&r.prog.reduceKeyCount, 1)

		// For a UID-only posting list, the badger value is a delta packed UID
		// list. The UserMeta indicates to treat the value as a delta packed
		// list when the value is read by dgraph.  For a value posting list,
		// the full protos.Posting type is used (which internally contains the
		// delta packed UID list).
		meta := posting.BitCompletePosting
		if len(pl.Postings) == 0 {
			meta |= posting.BitUidPosting
			txn.SetWithMeta(currentKey, bp128.DeltaPack(uids), meta)
		} else {
			pl.Uids = bp128.DeltaPack(uids)
			val, err := pl.Marshal()
			x.Check(err)
			txn.SetWithMeta(currentKey, val, meta)
		}

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
	x.Check(txn.CommitAt(r.state.writeTs, func(err error) {
		x.Check(err)
		NumBadgerWrites.Add(-1)
		r.writesThr.Done()
	}))
}
