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
	"github.com/dgraph-io/dgraph/x"
)

type mutation struct {
	Delete bool
	Attr   string
	UID    uint64
	Value  string
}

// FrontfillAdd inserts with overwrite (replace) key, value into our indices.
func FrontfillAdd(ctx context.Context, attr string, uid uint64, val string) {
	if err := globalIndices.Frontfill(ctx, newFrontfillAdd(attr, uid, val)); err != nil {
		x.TraceError(ctx, err)
	}
}

// FrontfillDel deletes a key, value from our indices.
func FrontfillDel(ctx context.Context, attr string, uid uint64) {
	if err := globalIndices.Frontfill(ctx, newFrontfillDel(attr, uid)); err != nil {
		x.TraceError(ctx, err)
	}
}

func newFrontfillAdd(attr string, uid uint64, val string) *mutation {
	return &mutation{
		Attr:  attr,
		UID:   uid,
		Value: val,
	}
}

func newFrontfillDel(attr string, uid uint64) *mutation {
	return &mutation{
		Delete: true,
		Attr:   attr,
		UID:    uid,
	}
}

// Frontfill updates indices given mutation.
func (s *Indices) Frontfill(ctx context.Context, m *mutation) error {
	index, found := s.pred[m.Attr]
	if !found {
		return nil // This predicate is not indexed, which can be common.
	}
	return index.frontfill(ctx, m)
}

func (s *predIndex) frontfill(ctx context.Context, m *mutation) error {
	childID := m.UID % uint64(s.config.NumChild)
	child := s.child[childID]
	if child.mutationC == nil {
		return x.Errorf("mutationC nil for %s %d", child.config.Attribute, childID)
	}
	child.mutationC <- m
	return nil
}

func (s *Indices) initFrontfill() {
	for _, pi := range s.pred {
		for _, child := range pi.child {
			child.mutationC = make(chan *mutation, 100)
			go child.handleFrontfill()
		}
	}
}

func (s *indexChild) handleFrontfill() {
	for m := range s.mutationC {
		s.bleveLock.Lock()
		if !m.Delete {
			s.bleveIndex.Index(string(posting.UID(m.UID)), m.Value)
		} else {
			s.bleveIndex.Delete(string(posting.UID(m.UID)))
		}
		s.bleveLock.Unlock()
	}
}
