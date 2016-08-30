/*
 * Copyright 2016 DGraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package bidx

import (
	"context"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/posting/types"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"
)

// Backfill simply adds stuff from posting list into index.
func (s *Indices) Backfill(ctx context.Context, ps *store.Store) error {
	for _, index := range s.pred {
		go index.backfill(ctx, ps, s.errC)
	}
	for i := 0; i < len(s.pred); i++ {
		if err := <-s.errC; err != nil {
			return err
		}
	}
	return nil
}

func (s *predIndex) backfill(ctx context.Context, ps *store.Store, errC chan error) {
	x.Trace(ctx, "Backfilling attribute: %s\n", s.config.Attribute)
	for _, child := range s.child {
		go child.backfill(ctx, ps, s.errC)
	}

	it := ps.NewIterator()
	defer it.Close()
	prefix := s.config.Attribute + "|"
	for it.Seek([]byte(prefix)); it.Valid(); it.Next() {
		if !it.ValidForPrefix([]byte(s.config.Attribute)) {
			// Keys are of the form attr|uid and sorted. Once we hit a attr that is
			// wrong, we are done.
			break
		}
		uid, attr := posting.DecodeKey(it.Key().Data())

		childID := uid % uint64(s.config.NumChild)
		pl := types.GetRootAsPostingList(it.Value().Data(), 0)
		var p types.Posting
		for i := 0; i < pl.PostingsLength(); i++ {
			x.Assertf(pl.Postings(&p, i), "Unable to get posting: %d %s", uid, attr)
			if p.ValueLength() == 0 {
				continue
			}
			value := string(p.ValueBytes())
			s.child[childID].jobC <- indexJob{
				op:    jobOpAdd,
				uid:   uid,
				value: value,
			}
		}
	}

	for i := 0; i < s.config.NumChild; i++ {
		close(s.child[i].jobC)
	}
	for i := 0; i < s.config.NumChild; i++ {
		if err := <-s.errC; err != nil {
			errC <- err // Some child failed. Inform our parent and return.
			return
		}
	}
	errC <- nil
}

func (s *indexChild) backfill(ctx context.Context, ps *store.Store, errC chan error) {
	var count uint64
	for job := range s.jobC {
		if job.op == jobOpAdd {
			s.batch.Index(string(posting.UID(job.uid)), job.value)
		} else {
			err := x.Errorf("Unknown job operation for backfill: %d", job.op)
			// We don't x.TraceError here. Let whoever wants to print the error call
			// x.TraceError. Otherwise, we print the error twice.
			errC <- err
			return
		}
		if s.batch.Size() >= batchSize {
			if err := s.doIndex(ctx, &count); err != nil {
				errC <- err
				return
			}
		}
	}
	errC <- s.doIndex(ctx, &count)
}

// doIndex apply the batch. count is incremented by batch size. Returns any error.
func (s *indexChild) doIndex(ctx context.Context, count *uint64) error {
	if s.batch.Size() == 0 {
		return nil
	}
	newCount := *count + uint64(s.batch.Size())
	x.Trace(ctx, "Attr[%s] child %d batch[%d, %d]\n",
		s.config.Attribute, s.childID, count, newCount)
	err := s.bleveIndex.Batch(s.batch)
	s.batch.Reset()
	*count = newCount
	return x.Wrap(err)
}
