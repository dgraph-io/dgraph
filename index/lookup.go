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
	"container/heap"
	"log"

	"github.com/blevesearch/bleve"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/x"
)

// LookupCategory describes type of lookup query. For now, we default to
// LookupMatch. It is not so clear how GraphQL can be modified to do more
// elaborate filtering.
type LookupCategory int

const (
	lookupUnknown = iota
	// LookupTerm is exact match.
	LookupTerm
	// LookupMatch allows a partial match. Default.
	LookupMatch
	// More to come. See http://www.blevesearch.com/docs/Query/
)

// LookupSpec defines a index lookup query.
type LookupSpec struct {
	Attr     string
	Param    []string
	Category LookupCategory
}

// LookupResult defines a index lookup result.
type LookupResult struct {
	UID []uint64
	Err error
}

// Lookup does a lookup into Bleve indices.
func (s *Indices) Lookup(li *LookupSpec) *LookupResult {
	if s == nil {
		return &LookupResult{
			Err: x.Errorf("Indices is nil"),
		}
	}
	index := s.idx[li.Attr]
	if index == nil {
		return &LookupResult{
			Err: x.Errorf("Attribute missing: %s", li.Attr),
		}
	}
	return index.lookup(li)
}

func (s *predIndex) lookup(li *LookupSpec) *LookupResult {
	results := make(chan *LookupResult)
	for _, ss := range s.child {
		go ss.lookup(li, results)
	}

	var lr []*LookupResult
	for i := 0; i < len(s.child); i++ {
		r := <-results
		if r.Err != nil {
			return r
		}
		lr = append(lr, r)
	}
	// Merge lr into one LookupResult.
	return mergeResults(lr)
}

func (s *childIndex) lookup(li *LookupSpec, results chan *LookupResult) {
	var query bleve.Query
	switch li.Category {
	case LookupTerm:
		if len(li.Param) != 1 {
			log.Fatalf("LookupTerm: expected 1 param, got %d", len(li.Param))
		}
		query = bleve.NewTermQuery(li.Param[0])
	case LookupMatch:
		if len(li.Param) != 1 {
			log.Fatalf("LookupTerm: expected 1 param, got %d", len(li.Param))
		}
		query = bleve.NewMatchQuery(li.Param[0])
	default:
		log.Fatalf("Lookup category not handled: %d", li.Category)
	}
	search := bleve.NewSearchRequest(query)
	s.bleveLock.RLock() // Read block might suffice? Index stats might be off.
	searchResults, err := s.bleveIndex.Search(search)
	s.bleveLock.RUnlock()
	if err != nil {
		results <- &LookupResult{Err: err}
		return
	}
	results <- &LookupResult{UID: extractUIDs(searchResults)}
}

func extractUIDs(r *bleve.SearchResult) []uint64 {
	var out []uint64
	for _, h := range r.Hits {
		out = append(out, posting.DecodeUID([]byte(h.ID)))
	}
	return out
}

func mergeResults(lr []*LookupResult) *LookupResult {
	// Similar to sortedUniqueUids in query.go. Do merge of sorted lists.
	h := &x.Uint64Heap{}
	heap.Init(h)

	for i, r := range lr {
		if len(r.UID) == 0 {
			continue
		}
		e := x.Elem{
			Uid: r.UID[0],
			Idx: i,
		}
		heap.Push(h, e)
	}

	// The resulting list of uids will be stored here.
	sorted := make([]uint64, 0, 100)

	// Which element are we looking at per LookupResult?
	ptr := make([]int, len(lr))

	var last uint64
	for h.Len() > 0 { // While heap is not empty.
		me := (*h)[0] // Peek at the top element in heap.
		if me.Uid != last {
			sorted = append(sorted, me.Uid) // Add if unique.
			last = me.Uid
		}
		uidList := lr[me.Idx].UID
		if ptr[me.Idx] >= len(uidList)-1 {
			heap.Pop(h)

		} else {
			ptr[me.Idx]++
			uid := uidList[ptr[me.Idx]]
			(*h)[0].Uid = uid
			heap.Fix(h, 0) // Faster than Pop() followed by Push().
		}
	}
	return &LookupResult{UID: sorted}
}
