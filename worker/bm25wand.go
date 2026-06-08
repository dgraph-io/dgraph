/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import (
	"container/heap"
	"math"
	"sort"

	"github.com/dgraph-io/dgraph/v25/posting"
)

// wandBlockSize is the number of postings grouped into one logical block for
// Block-Max WAND upper bounds. The postings come from a standard Dgraph posting
// list (already resident in memory once loaded); these blocks exist only to give
// WAND per-block score bounds for pruning — they are not a storage format.
const wandBlockSize = 128

// termCursor is an in-memory cursor over one query term's posting list,
// materialized from the standard posting List as UID-ascending (uid, tf, docLen)
// entries. Document length travels with each posting, so scoring needs no separate
// lookup. Per-block max bounds drive Block-Max WAND pruning.
type termCursor struct {
	postings []posting.BM25Posting
	idf      float64
	pos      int

	// blockUBPre[i] is the pre-IDF BM25 upper bound for block i (max term
	// frequency, min document length in the block). suffixUBPre[i] = max over
	// j >= i of blockUBPre[j], for the remaining-list upper bound.
	blockUBPre  []float64
	suffixUBPre []float64
}

// ubPre computes the pre-IDF BM25 contribution upper bound for a block, using the
// block's maximum term frequency and minimum document length (the score is
// increasing in tf and decreasing in dl, so this is a safe upper bound).
func ubPre(maxTF, minDL uint32, k, b, avgDL float64) float64 {
	if avgDL <= 0 {
		avgDL = 1
	}
	tf := float64(maxTF)
	dl := float64(minDL)
	denom := k*(1-b+b*dl/avgDL) + tf
	if denom <= 0 {
		return 0
	}
	return (k + 1) * tf / denom
}

// newTermCursor builds a cursor and precomputes its per-block upper bounds.
func newTermCursor(postings []posting.BM25Posting, idf, k, b, avgDL float64) *termCursor {
	c := &termCursor{postings: postings, idf: idf}
	numBlocks := (len(postings) + wandBlockSize - 1) / wandBlockSize
	c.blockUBPre = make([]float64, numBlocks)
	for blk := 0; blk < numBlocks; blk++ {
		start := blk * wandBlockSize
		end := start + wandBlockSize
		if end > len(postings) {
			end = len(postings)
		}
		var maxTF uint32
		minDL := uint32(math.MaxUint32)
		for i := start; i < end; i++ {
			if postings[i].TF > maxTF {
				maxTF = postings[i].TF
			}
			dl := postings[i].DocLen
			if dl == 0 {
				dl = 1
			}
			if dl < minDL {
				minDL = dl
			}
		}
		c.blockUBPre[blk] = ubPre(maxTF, minDL, k, b, avgDL)
	}
	c.suffixUBPre = make([]float64, numBlocks)
	var running float64
	for blk := numBlocks - 1; blk >= 0; blk-- {
		if c.blockUBPre[blk] > running {
			running = c.blockUBPre[blk]
		}
		c.suffixUBPre[blk] = running
	}
	return c
}

func (c *termCursor) exhausted() bool { return c.pos >= len(c.postings) }

func (c *termCursor) currentDoc() uint64 {
	if c.exhausted() {
		return math.MaxUint64
	}
	return c.postings[c.pos].Uid
}

func (c *termCursor) currentTF() uint32 {
	if c.exhausted() {
		return 0
	}
	return c.postings[c.pos].TF
}

func (c *termCursor) currentDocLen() uint32 {
	if c.exhausted() {
		return 0
	}
	return c.postings[c.pos].DocLen
}

// remainingUB returns the IDF-weighted upper-bound score over the remainder of the
// list from the current position.
func (c *termCursor) remainingUB() float64 {
	if c.exhausted() || len(c.suffixUBPre) == 0 {
		return 0
	}
	blk := c.pos / wandBlockSize
	if blk >= len(c.suffixUBPre) {
		return 0
	}
	return c.idf * c.suffixUBPre[blk]
}

// next advances by one posting.
func (c *termCursor) next() bool {
	c.pos++
	return !c.exhausted()
}

// skipTo advances to the first posting with UID >= target.
func (c *termCursor) skipTo(target uint64) bool {
	if c.exhausted() {
		return false
	}
	if c.postings[c.pos].Uid >= target {
		return true
	}
	rel := sort.Search(len(c.postings)-c.pos, func(i int) bool {
		return c.postings[c.pos+i].Uid >= target
	})
	c.pos += rel
	return !c.exhausted()
}

// skipToWithBMW is skipTo with Block-Max WAND pruning: blocks whose upper bound
// combined with otherUB cannot beat theta are skipped wholesale.
func (c *termCursor) skipToWithBMW(target uint64, theta, otherUB float64) bool {
	if !c.skipTo(target) {
		return false
	}
	for !c.exhausted() {
		blk := c.pos / wandBlockSize
		if c.idf*c.blockUBPre[blk]+otherUB > theta {
			return true
		}
		// This block can't produce a winner; jump to the start of the next block.
		c.pos = (blk + 1) * wandBlockSize
	}
	return false
}

// scoredDoc holds a UID and its BM25 score for the min-heap.
type scoredDoc struct {
	uid   uint64
	score float64
}

// topKHeap is a min-heap of scored documents for top-k tracking.
type topKHeap struct {
	docs []scoredDoc
	k    int
}

func (h *topKHeap) Len() int           { return len(h.docs) }
func (h *topKHeap) Less(i, j int) bool { return h.docs[i].score < h.docs[j].score }
func (h *topKHeap) Swap(i, j int)      { h.docs[i], h.docs[j] = h.docs[j], h.docs[i] }
func (h *topKHeap) Push(x interface{}) { h.docs = append(h.docs, x.(scoredDoc)) }
func (h *topKHeap) Pop() interface{} {
	old := h.docs
	n := len(old)
	item := old[n-1]
	h.docs = old[:n-1]
	return item
}

// threshold returns the minimum score in the heap (the score to beat).
func (h *topKHeap) threshold() float64 {
	if len(h.docs) < h.k {
		return 0
	}
	return h.docs[0].score
}

// tryPush adds a doc if it beats the current threshold.
func (h *topKHeap) tryPush(uid uint64, score float64) {
	if len(h.docs) < h.k {
		heap.Push(h, scoredDoc{uid: uid, score: score})
		return
	}
	if score > h.docs[0].score {
		h.docs[0] = scoredDoc{uid: uid, score: score}
		heap.Fix(h, 0)
	}
}

// sorted returns all docs sorted by score descending, then UID ascending.
func (h *topKHeap) sorted() []scoredDoc {
	result := make([]scoredDoc, len(h.docs))
	copy(result, h.docs)
	sort.Slice(result, func(i, j int) bool {
		if result[i].score != result[j].score {
			return result[i].score > result[j].score
		}
		return result[i].uid < result[j].uid
	})
	return result
}

// bm25TopK returns how many top-scored documents the WAND search should retain for a
// first/offset query window. With a first limit it is first+offset (so the offset can
// be dropped afterward while still bounding work and memory to the window); with no
// first limit it is 0, meaning every matching document is scored. Returning first+offset
// rather than 0 whenever an offset is present is what keeps a deep-paginated query from
// materializing and scoring the entire corpus.
func bm25TopK(first, offset int) int {
	if first <= 0 {
		return 0
	}
	return first + offset
}

// bm25PaginateScored slices score-descending results to the [offset, offset+first)
// window. first <= 0 means no upper bound. It clamps offset to the slice length so an
// offset past the end yields an empty result instead of panicking.
func bm25PaginateScored(results []scoredDoc, first, offset int) []scoredDoc {
	if offset > len(results) {
		offset = len(results)
	}
	results = results[offset:]
	if first > 0 && first < len(results) {
		results = results[:first]
	}
	return results
}

// bm25Score computes the BM25 contribution of a single term occurrence.
func bm25Score(idf, tf, dl, avgDL, k, b float64) float64 {
	if avgDL <= 0 {
		avgDL = 1
	}
	if dl <= 0 {
		dl = 1
	}
	return idf * (k + 1) * tf / (k*(1-b+b*dl/avgDL) + tf)
}

// wandSearch performs a WAND / Block-Max WAND top-k BM25 search over standard
// posting lists. queryTokens must already carry the BM25 tokenizer identifier
// byte. getList reads a posting list for a key. If topK <= 0, every matching
// document is scored (no early termination).
func wandSearch(getList func(key []byte) (*posting.List, error), attr string, readTs uint64,
	queryTokens []string, k, b, avgDL, N float64, topK int,
	filterSet map[uint64]struct{}, useBMW bool) ([]scoredDoc, error) {

	var cursors []*termCursor
	for _, token := range queryTokens {
		postings, err := posting.ReadBM25TermPostings(getList, attr, token, readTs)
		if err != nil {
			return nil, err
		}
		df := uint64(len(postings))
		if df == 0 {
			continue
		}
		// N comes from bucketed stats and df from the term's posting list; if stats
		// ever lag the postings, clamp N >= df for this term so the smoothed IDF
		// stays non-negative and finite instead of producing a negative/NaN score.
		dfN := float64(df)
		nDocs := N
		if nDocs < dfN {
			nDocs = dfN
		}
		idf := math.Log1p((nDocs - dfN + 0.5) / (dfN + 0.5))
		cursors = append(cursors, newTermCursor(postings, idf, k, b, avgDL))
	}

	if len(cursors) == 0 {
		return nil, nil
	}

	if topK <= 0 {
		return scoreAllDocs(cursors, k, b, avgDL, filterSet), nil
	}
	return wandTopK(cursors, k, b, avgDL, topK, filterSet, useBMW), nil
}

// wandTopK runs the WAND / Block-Max WAND main loop over prepared cursors and
// returns the top-k documents sorted by score descending. It is the core scoring
// loop, separated from posting-list I/O so it can be exercised directly.
func wandTopK(cursors []*termCursor, k, b, avgDL float64, topK int,
	filterSet map[uint64]struct{}, useBMW bool) []scoredDoc {

	h := &topKHeap{k: topK}
	heap.Init(h)

	for {
		// Drop exhausted cursors.
		active := cursors[:0]
		for _, c := range cursors {
			if !c.exhausted() {
				active = append(active, c)
			}
		}
		cursors = active
		if len(cursors) == 0 {
			break
		}

		// Sort cursors by current document ascending.
		sort.Slice(cursors, func(i, j int) bool {
			return cursors[i].currentDoc() < cursors[j].currentDoc()
		})

		theta := h.threshold()

		// Find pivot: accumulate upper bounds until they exceed theta.
		var sumUB float64
		pivot := -1
		var pivotDoc uint64
		for i, c := range cursors {
			sumUB += c.remainingUB()
			if sumUB > theta && pivot == -1 {
				pivot = i
				pivotDoc = c.currentDoc()
			}
		}
		if pivot == -1 {
			break // sum of all upper bounds can't beat theta
		}

		// Advance all cursors before the pivot up to pivotDoc.
		allAtPivot := true
		for i := 0; i < pivot; i++ {
			if cursors[i].currentDoc() < pivotDoc {
				var ok bool
				if useBMW {
					otherUB := sumUB - cursors[i].remainingUB()
					ok = cursors[i].skipToWithBMW(pivotDoc, theta, otherUB)
				} else {
					ok = cursors[i].skipTo(pivotDoc)
				}
				if !ok {
					allAtPivot = false
					break
				}
				if cursors[i].currentDoc() != pivotDoc {
					allAtPivot = false
				}
			}
		}
		if !allAtPivot {
			continue
		}

		// Score the pivot document.
		if filterSet != nil {
			if _, ok := filterSet[pivotDoc]; !ok {
				for _, c := range cursors {
					if c.currentDoc() == pivotDoc {
						c.next()
					}
				}
				continue
			}
		}

		var score float64
		for _, c := range cursors {
			if c.currentDoc() == pivotDoc {
				dl := float64(c.currentDocLen())
				score += bm25Score(c.idf, float64(c.currentTF()), dl, avgDL, k, b)
			}
		}
		h.tryPush(pivotDoc, score)

		for _, c := range cursors {
			if c.currentDoc() == pivotDoc {
				c.next()
			}
		}
	}

	return h.sorted()
}

// scoreAllDocs scores every matching document without early termination. Used when
// no top-k limit is specified.
func scoreAllDocs(cursors []*termCursor, k, b, avgDL float64,
	filterSet map[uint64]struct{}) []scoredDoc {

	scores := make(map[uint64]float64)

	for _, c := range cursors {
		for !c.exhausted() {
			uid := c.currentDoc()
			if filterSet != nil {
				if _, ok := filterSet[uid]; !ok {
					c.next()
					continue
				}
			}
			scores[uid] += bm25Score(c.idf, float64(c.currentTF()), float64(c.currentDocLen()),
				avgDL, k, b)
			c.next()
		}
	}

	results := make([]scoredDoc, 0, len(scores))
	for uid, s := range scores {
		results = append(results, scoredDoc{uid: uid, score: s})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].score != results[j].score {
			return results[i].score > results[j].score
		}
		return results[i].uid < results[j].uid
	})
	return results
}
