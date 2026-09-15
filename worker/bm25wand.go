/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import (
	"container/heap"
	"context"
	"encoding/binary"
	"math"
	"sort"
	"strconv"

	"github.com/pkg/errors"

	"github.com/dgraph-io/dgraph/v25/posting"
	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/tok"
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

func (h *topKHeap) Len() int { return len(h.docs) }
func (h *topKHeap) Less(i, j int) bool {
	if h.docs[i].score != h.docs[j].score {
		return h.docs[i].score < h.docs[j].score
	}
	// Among equal scores, treat the higher UID as "smaller" so it sits at the root and
	// is the eviction victim first. This keeps the lowest UID on score ties, matching
	// the (score desc, UID asc) order that sorted() and scoreAllDocs produce, so a
	// first:k WAND query returns the same set as the exhaustive path. Primary ordering
	// stays score-ascending, so threshold() still reports the true minimum score and
	// WAND pivot pruning is unaffected.
	return h.docs[i].uid > h.docs[j].uid
}
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
	if offset < 0 {
		offset = 0
	}
	return first + offset
}

// bm25PaginateScored slices score-descending results to the [offset, offset+first)
// window. first <= 0 means no upper bound. It clamps offset to [0, len(results)] —
// mirroring x.PageRange — so a negative offset or one past the end yields a full or
// empty result instead of panicking.
func bm25PaginateScored(results []scoredDoc, first, offset int) []scoredDoc {
	if offset < 0 {
		offset = 0
	}
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

// handleBM25Search executes the bm25() root/filter function: it parses the (query
// [, k, b]) arguments, reads the corpus statistics, runs WAND / Block-Max WAND over
// the term posting lists, and emits UID-ascending results with positionally-aligned
// scores in ValueMatrix.
func (qs *queryState) handleBM25Search(ctx context.Context, args funcArgs) error {
	q := args.q
	attr := q.Attr

	// 1. Parse args: query text, optional k (default 1.2), b (default 0.75).
	if len(q.SrcFunc.Args) < 1 {
		return errors.Errorf("bm25 requires at least 1 argument (query text)")
	}
	queryText := q.SrcFunc.Args[0]
	k := 1.2
	b := 0.75
	if len(q.SrcFunc.Args) >= 2 {
		var err error
		k, err = strconv.ParseFloat(q.SrcFunc.Args[1], 64)
		if err != nil {
			return errors.Errorf("bm25: invalid k parameter: %s", q.SrcFunc.Args[1])
		}
	}
	if len(q.SrcFunc.Args) >= 3 {
		var err error
		b, err = strconv.ParseFloat(q.SrcFunc.Args[2], 64)
		if err != nil {
			return errors.Errorf("bm25: invalid b parameter: %s", q.SrcFunc.Args[2])
		}
	}
	if math.IsNaN(k) || math.IsInf(k, 0) || k <= 0 {
		return errors.Errorf("bm25: k must be a positive finite number, got %v", k)
	}
	if math.IsNaN(b) || math.IsInf(b, 0) || b < 0 || b > 1 {
		return errors.Errorf("bm25: b must be between 0 and 1, got %v", b)
	}
	// Negative first means "last N" elsewhere (x.PageRange), but bm25 results are
	// score-ordered and worker-paginated, so "last N by score" has no defined use;
	// reject it rather than silently returning everything.
	if q.First < 0 {
		return errors.Errorf("bm25: negative first (%d) is not supported", q.First)
	}

	// 2. Tokenize query (deduplicated) using the fulltext pipeline. The returned
	// tokens already carry the BM25 tokenizer identifier byte.
	lang := langForFunc(q.Langs)
	queryTokens, err := tok.GetBM25QueryTokens([]string{queryText}, lang)
	if err != nil {
		return err
	}
	if len(queryTokens) == 0 {
		args.out.UidMatrix = append(args.out.UidMatrix, &pb.List{})
		return nil
	}

	// 3. Read bucketed corpus stats and derive N and the average document length.
	docCount, totalTerms, err := posting.ReadBM25Stats(qs.cache.Get, attr, q.ReadTs)
	if err != nil {
		return err
	}
	if docCount == 0 || totalTerms == 0 {
		args.out.UidMatrix = append(args.out.UidMatrix, &pb.List{})
		return nil
	}
	avgDL := float64(totalTerms) / float64(docCount)
	N := float64(docCount)

	// Build a filter set if bm25 is used as a filter (@filter(bm25(...))).
	var filterSet map[uint64]struct{}
	if q.UidList != nil && len(q.UidList.Uids) > 0 {
		filterSet = make(map[uint64]struct{}, len(q.UidList.Uids))
		for _, uid := range q.UidList.Uids {
			filterSet[uid] = struct{}{}
		}
	}

	// 4. Use WAND top-k early termination whenever a first limit is set, retaining
	// first+offset documents so the offset can be dropped afterward. This bounds work
	// and memory to the requested window instead of scoring (and materializing) every
	// matching document just because an offset was supplied. With no first limit,
	// topK is 0 and every match is scored (the caller explicitly asked for all results).
	topK := bm25TopK(int(q.First), int(q.Offset))

	// 5. Run WAND / Block-Max WAND over the standard posting lists.
	results, err := wandSearch(qs.cache.Get, attr, q.ReadTs, queryTokens, k, b, avgDL, N,
		topK, filterSet, true)
	if err != nil {
		return err
	}

	// 6. Apply the first/offset window over the score-descending results. wandSearch
	// returns results sorted by score (descending), whether or not it top-k'd them, so
	// the same slice is correct for both the top-k and score-all paths.
	results = bm25PaginateScored(results, int(q.First), int(q.Offset))

	// 7. Emit UIDs ascending (required by the query pipeline) with positionally-
	// aligned scores in ValueMatrix; the query layer binds these to a value
	// variable so callers order by and project the score via val().
	sort.Slice(results, func(i, j int) bool { return results[i].uid < results[j].uid })
	uids := make([]uint64, len(results))
	for i, r := range results {
		uids[i] = r.uid
	}
	args.out.UidMatrix = append(args.out.UidMatrix, &pb.List{Uids: uids})

	scoreBuf := make([]byte, len(results)*8)
	scoreValues := make([]*pb.ValueList, len(results))
	for i, r := range results {
		off := i * 8
		binary.LittleEndian.PutUint64(scoreBuf[off:off+8], math.Float64bits(r.score))
		// Three-index slice caps capacity at 8 so a downstream append can't corrupt
		// adjacent scores in the shared backing array.
		scoreValues[i] = &pb.ValueList{
			Values: []*pb.TaskValue{{Val: scoreBuf[off : off+8 : off+8], ValType: pb.Posting_FLOAT}},
		}
	}
	args.out.ValueMatrix = append(args.out.ValueMatrix, scoreValues...)
	return nil
}
