// Given attribute and value, look up index.
package bidx

import (
	"container/heap"
	"fmt"
	"log"

	"github.com/blevesearch/bleve"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/x"
)

type LookupCategory int

const (
	lookupTerm        = iota
	lookupMatch       = iota
	lookupMatchPhrase = iota
	lookupPrefix      = iota
	// More to come. See http://www.blevesearch.com/docs/Query/
)

type LookupInfo struct {
	attr     string
	param    []string
	category LookupCategory
}

type LookupResult struct {
	uid []uint64
	err error
}

func (s *Indices) Lookup(li *LookupInfo) *LookupResult {
	index := s.Index[li.attr]
	if index == nil {
		return &LookupResult{
			err: fmt.Errorf("Attribute missing: %s", li.attr),
		}
	}
	return index.Lookup(li)
}

func (s *Index) Lookup(li *LookupInfo) *LookupResult {
	results := make(chan *LookupResult)
	for _, ss := range s.Shard {
		go ss.Lookup(li, results)
	}

	var lr []*LookupResult
	for i := 0; i < len(s.Shard); i++ {
		r := <-results
		if r.err != nil {
			return r
		}
		lr = append(lr, r)
	}
	// Merge lr into one LookupResult.
	return mergeResults(lr)
}

func (s *IndexShard) Lookup(li *LookupInfo, results chan *LookupResult) {
	var query bleve.Query
	switch li.category {
	case lookupTerm:
		if len(li.param) != 1 {
			log.Fatalf("LookupTerm: expected 1 param, got %d", len(li.param))
		}
		query = bleve.NewTermQuery(li.param[0])
	case lookupMatch:
		if len(li.param) != 1 {
			log.Fatalf("LookupTerm: expected 1 param, got %d", len(li.param))
		}
		query = bleve.NewMatchQuery(li.param[0])
	default:
		log.Fatalf("Lookup category not handled: %d", li.category)
	}
	search := bleve.NewSearchRequest(query)
	searchResults, err := s.Bindex.Search(search)
	if err != nil {
		results <- &LookupResult{err: err}
		return
	}
	results <- &LookupResult{uid: extractUIDs(searchResults)}
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
		if len(r.uid) == 0 {
			continue
		}
		e := x.Elem{
			Uid: r.uid[0],
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
		uidList := lr[me.Idx].uid
		if ptr[me.Idx] >= len(uidList)-1 {
			heap.Pop(h)

		} else {
			ptr[me.Idx]++
			uid := uidList[ptr[me.Idx]]
			(*h)[0].Uid = uid
			heap.Fix(h, 0) // Faster than Pop() followed by Push().
		}
	}
	return &LookupResult{uid: sorted}
}
