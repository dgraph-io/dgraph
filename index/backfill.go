/*
 * Copyright 2016 Dgraph Labs, Inc.
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

// Package index indexes values in database. This can be used for filtering.
package index

import (
	"context"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/posting/types"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"
)

// Backfill simply adds stuff from posting list into index.
func (s *Indices) Backfill(ctx context.Context, ps *store.Store) error {
	for _, index := range s.idx {
		go index.backfill(ctx, ps, s.errC)
	}
	for i := 0; i < len(s.idx); i++ {
		if err := <-s.errC; err != nil {
			return err
		}
	}
	return nil
}

func (s *predIndex) backfill(ctx context.Context, ps *store.Store, errC chan error) {
	x.Trace(ctx, "Backfilling attribute: %s\n", s.cfg.Attr)
	for _, child := range s.child {
		go child.backfill(ctx, ps, s.errC)
	}

	it := ps.NewIterator()
	defer it.Close()
	prefix := s.cfg.Attr + "|"
	for it.Seek([]byte(prefix)); it.Valid(); it.Next() {
		if !it.ValidForPrefix([]byte(s.cfg.Attr)) {
			// Keys are of the form attr|uid and sorted. Once we hit a attr that is
			// wrong, we are done.
			break
		}
		uid, attr := posting.DecodeKey(it.Key().Data())

		childID := uid % uint64(s.cfg.NumChild)
		pl := types.GetRootAsPostingList(it.Value().Data(), 0)
		var p types.Posting
		for i := 0; i < pl.PostingsLength(); i++ {
			x.Assertf(pl.Postings(&p, i), "Unable to get posting: %d %s", uid, attr)
			if p.ValueLength() == 0 {
				continue
			}
			value := string(p.ValueBytes())
			s.child[childID].backfillC <- &mutation{
				uid:   uid,
				value: value,
			}
		}
	}

	for i := 0; i < s.cfg.NumChild; i++ {
		close(s.child[i].backfillC)
	}
	for i := 0; i < s.cfg.NumChild; i++ {
		if err := <-s.errC; err != nil {
			errC <- err // Some child failed. Inform our parent and return.
			return
		}
	}
	errC <- nil
}

func (s *childIndex) backfill(ctx context.Context, ps *store.Store, errC chan error) {
	var count uint64
	for job := range s.backfillC {
		if !job.remove {
			//			s.batch.Index(string(posting.UID(job.uid)), job.value)
			s.batch.Insert(s.parent.cfg.Attr, string(posting.UID(job.uid)), job.value)
		} else {
			err := x.Errorf("Backfill does not support deletes %s %d", job.attr, job.uid)
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
func (s *childIndex) doIndex(ctx context.Context, count *uint64) error {
	if s.batch.Size() == 0 {
		return nil
	}
	newCount := *count + uint64(s.batch.Size())
	x.Trace(ctx, "Attr[%s] child %d batch[%d, %d]\n",
		s.parent.cfg.Attr, s.childID, count, newCount)
	err := x.Wrap(s.parent.indexer.Batch(s.batch))
	s.batch.Reset()
	*count = newCount
	return err
}
