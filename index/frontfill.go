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
	"github.com/dgraph-io/dgraph/x"
)

// FrontfillAdd inserts with overwrite (replace) key, value into our indices.
func (s *Indices) FrontfillAdd(ctx context.Context, attr string, uid uint64, val string) error {
	return s.Frontfill(ctx, &mutation{
		remove: false,
		attr:   attr,
		uid:    uid,
		value:  val,
	})
}

// FrontfillDel deletes a key, value from our indices.
func (s *Indices) FrontfillDel(ctx context.Context, attr string, uid uint64) error {
	return s.Frontfill(ctx, &mutation{
		remove: true,
		attr:   attr,
		uid:    uid,
	})
}

// Frontfill updates indices given mutation.
func (s *Indices) Frontfill(ctx context.Context, job *mutation) error {
	index, found := s.idx[job.attr]
	if !found {
		return nil // This predicate is not indexed, which can be common.
	}
	childID := job.uid % uint64(index.cfg.NumChild)
	child := index.child[childID]
	if child.frontfillC == nil {
		return x.Errorf("Channel nil for %s %d", child.parent.cfg.Attr, childID)
	}
	child.frontfillC <- job
	return nil
}

func (s *childIndex) handleFrontfill() {
	for m := range s.frontfillC {
		uid := string(posting.UID(m.uid))
		if !m.remove {
			s.parent.indexer.Insert(s.parent.cfg.Attr, uid, m.value)
		} else {
			s.parent.indexer.Remove(s.parent.cfg.Attr, uid)
		}
	}
}
