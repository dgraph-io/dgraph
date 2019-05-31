/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package bulk

import (
	"bytes"
	"sync/atomic"

	"github.com/dgraph-io/badger"

	"github.com/dgraph-io/dgraph/codec"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
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
	pl := new(pb.PostingList)
	txn := job.db.NewTransactionAt(r.state.writeTs, true)

	outputPostingList := func() {
		atomic.AddInt64(&r.prog.reduceKeyCount, 1)

		// For a UID-only posting list, the badger value is a delta packed UID
		// list. The UserMeta indicates to treat the value as a delta packed
		// list when the value is read by dgraph.  For a value posting list,
		// the full pb.Posting type is used (which pb.y contains the
		// delta packed UID list).
		meta := posting.BitCompletePosting
		pl.Pack = codec.Encode(uids, 256)
		val, err := pl.Marshal()
		x.Check(err)
		x.Check(txn.SetEntry(&badger.Entry{
			Key:      currentKey,
			Value:    val,
			UserMeta: meta,
		}))

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
