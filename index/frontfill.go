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

package index

import (
	"context"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/x"
)

// FrontfillAdd inserts with overwrite (replace) key, value into our indices.
func FrontfillAdd(ctx context.Context, attr string, uid uint64, val string) {
	if err := globalIndices.FrontfillAdd(ctx, attr, uid, val); err != nil {
		x.TraceError(ctx, err)
	}
}

// FrontfillDel deletes a key, value from our indices.
func FrontfillDel(ctx context.Context, attr string, uid uint64) {
	if err := globalIndices.FrontfillDel(ctx, attr, uid); err != nil {
		x.TraceError(ctx, err)
	}
}

func (s *Indices) FrontfillAdd(ctx context.Context, attr string, uid uint64, val string) error {
	return s.Frontfill(ctx, &indexJob{
		attr:  attr,
		uid:   uid,
		value: val,
	})
}

func (s *Indices) FrontfillDel(ctx context.Context, attr string, uid uint64) error {
	return s.Frontfill(ctx, &indexJob{
		del:  true,
		attr: attr,
		uid:  uid,
	})
}

// Frontfill updates indices given mutation.
func (s *Indices) Frontfill(ctx context.Context, job *indexJob) error {
	index, found := s.idx[job.attr]
	if !found {
		return nil // This predicate is not indexed, which can be common.
	}
	return index.frontfill(ctx, job)
}

func (s *index) frontfill(ctx context.Context, job *indexJob) error {
	childID := job.uid % uint64(s.cfg.NumChild)
	child := s.child[childID]
	if child.jobC == nil {
		return x.Errorf("jobC nil for %s %d", child.cfg.Attr, childID)
	}
	child.jobC <- job
	return nil
}

func (s *Indices) initFrontfill() {
	for _, idx := range s.idx {
		for _, child := range idx.child {
			go child.handleFrontfill()
		}
	}
}

func (s *childIndex) handleFrontfill() {
	for m := range s.jobC {
		s.bleveLock.Lock()
		if !m.Delete {
			s.bleveIndex.Index(string(posting.UID(m.UID)), m.Value)
		} else {
			s.bleveIndex.Delete(string(posting.UID(m.UID)))
		}
		s.bleveLock.Unlock()
	}
}
