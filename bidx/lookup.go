// Given attribute and value, look up index.
package bidx

import (
	"container/heap"
	"log"

	"github.com/blevesearch/bleve"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/x"
)

type LookupCategory int

const (
	LookupTerm        = iota
	LookupMatch       = iota
	LookupMatchPhrase = iota
	LookupPrefix      = iota
	// More to come. See http://www.blevesearch.com/docs/Query/
)

type LookupSpec struct {
	Attr     string
	Param    []string
	Category LookupCategory
}

type LookupResult struct {
	UID []uint64
	Err error
}

func (s *Indices) Lookup(li *LookupSpec) *LookupResult {
	if s == nil {
		return &LookupResult{
			Err: x.Errorf("Indices is nil"),
		}
	}
	index := s.index[li.Attr]
	if index == nil {
		return &LookupResult{
			Err: x.Errorf("Attribute missing: %s", li.Attr),
		}
	}
	return index.lookup(li)
}

func (s *Index) lookup(li *LookupSpec) *LookupResult {
	results := make(chan *LookupResult)
	for _, ss := range s.shard {
		go ss.lookup(li, results)
	}

	var lr []*LookupResult
	for i := 0; i < len(s.shard); i++ {
		r := <-results
		if r.Err != nil {
			return r
		}
		lr = append(lr, r)
	}
	// Merge lr into one LookupResult.
	return mergeResults(lr)
}

func (s *IndexShard) lookup(li *LookupSpec, results chan *LookupResult) {
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
	searchResults, err := s.bindex.Search(search)
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
