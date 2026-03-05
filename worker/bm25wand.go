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
	"github.com/dgraph-io/dgraph/v25/posting/bm25block"
	"github.com/dgraph-io/dgraph/v25/posting/bm25enc"
	"github.com/dgraph-io/dgraph/v25/x"
)

// listIter iterates over a term's block-based posting list for WAND scoring.
type listIter struct {
	attr        string
	encodedTerm string
	readTs      uint64
	idf         float64
	k, b        float64

	dir        *bm25block.Dir
	ubPreSuf   []float64       // suffix max of UBPre values
	blockIdx   int             // current block index in dir.Blocks
	block      []bm25enc.Entry // decoded current block
	inBlockPos int             // position within current block

	exhausted bool
	legacy    bool // true if using legacy monolithic blob (migration fallback)
}

// newListIter creates a new iterator for a term's block-based posting list.
// Falls back to the legacy monolithic blob format if no block directory exists.
// If dir is non-nil, it is used directly (avoids re-reading from Badger).
func newListIter(attr, encodedTerm string, readTs uint64, idf, k, b float64, dir *bm25block.Dir) *listIter {
	if dir == nil {
		dirKey := x.BM25TermDirKey(attr, encodedTerm)
		dirBlob := posting.ReadBM25BlobAt(dirKey, readTs)
		dir = bm25block.DecodeDir(dirBlob)
	}

	if len(dir.Blocks) == 0 {
		// Fallback: try reading the legacy monolithic blob and wrap it as a single block.
		legacyKey := x.BM25IndexKey(attr, encodedTerm)
		legacyBlob := posting.ReadBM25BlobAt(legacyKey, readTs)
		legacyEntries := bm25enc.Decode(legacyBlob)
		if len(legacyEntries) == 0 {
			return &listIter{exhausted: true}
		}
		// Build a synthetic single-block directory from the legacy data.
		var maxTF uint32
		for _, e := range legacyEntries {
			if e.Value > maxTF {
				maxTF = e.Value
			}
		}
		dir = &bm25block.Dir{
			NextID: 1,
			Blocks: []bm25block.BlockMeta{{
				FirstUID: legacyEntries[0].UID,
				BlockID:  0,
				Count:    uint32(len(legacyEntries)),
				MaxTF:    maxTF,
			}},
		}
		it := &listIter{
			attr:        attr,
			encodedTerm: encodedTerm,
			readTs:      readTs,
			idf:         idf,
			k:           k,
			b:           b,
			dir:         dir,
			ubPreSuf:    bm25block.SuffixMaxUBPre(dir, k, b),
			blockIdx:    0,
			block:       legacyEntries, // pre-loaded
			inBlockPos:  -1,            // will advance on first next()
			legacy:      true,
		}
		return it
	}

	it := &listIter{
		attr:        attr,
		encodedTerm: encodedTerm,
		readTs:      readTs,
		idf:         idf,
		k:           k,
		b:           b,
		dir:         dir,
		ubPreSuf:    bm25block.SuffixMaxUBPre(dir, k, b),
		blockIdx:    -1, // will be advanced on first Next()
	}
	return it
}

// currentDoc returns the UID at the current position.
func (it *listIter) currentDoc() uint64 {
	if it.exhausted || it.block == nil || it.inBlockPos < 0 || it.inBlockPos >= len(it.block) {
		return math.MaxUint64
	}
	return it.block[it.inBlockPos].UID
}

// currentTF returns the term frequency at the current position.
func (it *listIter) currentTF() uint32 {
	if it.exhausted || it.block == nil || it.inBlockPos < 0 || it.inBlockPos >= len(it.block) {
		return 0
	}
	return it.block[it.inBlockPos].Value
}

// remainingUB returns the IDF-weighted upper-bound score for the remaining postings.
func (it *listIter) remainingUB() float64 {
	if it.exhausted || len(it.ubPreSuf) == 0 {
		return 0
	}
	idx := it.blockIdx
	if idx < 0 {
		idx = 0
	}
	if idx >= len(it.ubPreSuf) {
		return 0
	}
	return it.idf * it.ubPreSuf[idx]
}

// blockUB returns the IDF-weighted upper-bound for the current block only.
func (it *listIter) blockUB() float64 {
	if it.exhausted || it.blockIdx < 0 || it.blockIdx >= len(it.dir.Blocks) {
		return 0
	}
	return it.idf * bm25block.ComputeUBPre(it.dir.Blocks[it.blockIdx].MaxTF, it.k, it.b)
}

// next advances to the next posting. Returns false if exhausted.
func (it *listIter) next() bool {
	if it.exhausted {
		return false
	}

	// Try advancing within the current block.
	if it.block != nil {
		it.inBlockPos++
		if it.inBlockPos >= 0 && it.inBlockPos < len(it.block) {
			return true
		}
	}

	// Move to the next block.
	for {
		it.blockIdx++
		if it.blockIdx >= len(it.dir.Blocks) {
			it.exhausted = true
			return false
		}
		it.loadBlock(it.blockIdx)
		if len(it.block) > 0 {
			return true
		}
		// Empty block (corruption/race): skip it.
	}
}

// skipTo advances to the first posting with UID >= target.
// Returns false if exhausted.
func (it *listIter) skipTo(target uint64) bool {
	if it.exhausted {
		return false
	}

	// If current doc is already >= target, no-op.
	if it.block != nil && it.inBlockPos >= 0 && it.inBlockPos < len(it.block) &&
		it.block[it.inBlockPos].UID >= target {
		return true
	}

	// Check if target might be in the current block.
	if it.block != nil && len(it.block) > 0 && it.blockIdx >= 0 &&
		it.blockIdx < len(it.dir.Blocks) {
		lastInBlock := it.block[len(it.block)-1].UID
		if target <= lastInBlock {
			startPos := it.inBlockPos
			if startPos < 0 {
				startPos = 0
			} else if startPos > len(it.block) {
				startPos = len(it.block)
			}
			// Binary search within current block from startPos.
			pos := sort.Search(len(it.block)-startPos, func(i int) bool {
				return it.block[startPos+i].UID >= target
			})
			it.inBlockPos = startPos + pos
			if it.inBlockPos < len(it.block) {
				return true
			}
		}
	}

	// Find the right block using the directory.
	blockIdx := it.findBlockForTarget(target)
	if blockIdx >= len(it.dir.Blocks) {
		it.exhausted = true
		return false
	}

	it.blockIdx = blockIdx
	it.loadBlock(blockIdx)
	if len(it.block) == 0 {
		return it.next() // skip empty block
	}

	// Binary search within the block.
	pos := sort.Search(len(it.block), func(i int) bool {
		return it.block[i].UID >= target
	})
	it.inBlockPos = pos
	if pos >= len(it.block) {
		// Target is beyond this block; try the next.
		return it.next()
	}
	return true
}

// skipToWithBMW is like skipTo but uses Block-Max WAND to skip entire blocks
// whose upper bounds can't beat the given threshold.
func (it *listIter) skipToWithBMW(target uint64, theta float64, otherUB float64) bool {
	if it.exhausted {
		return false
	}

	// If current doc is already >= target, no-op.
	if it.block != nil && it.inBlockPos >= 0 && it.inBlockPos < len(it.block) &&
		it.block[it.inBlockPos].UID >= target {
		return true
	}

	blockIdx := it.findBlockForTarget(target)
	for blockIdx < len(it.dir.Blocks) {
		// Check if this block's UB combined with other terms can beat theta.
		blockUB := it.idf * bm25block.ComputeUBPre(it.dir.Blocks[blockIdx].MaxTF, it.k, it.b)
		if blockUB+otherUB > theta {
			// This block might have a winner; load and search it.
			it.blockIdx = blockIdx
			it.loadBlock(blockIdx)
			if len(it.block) == 0 {
				blockIdx++
				continue // skip empty block
			}
			pos := sort.Search(len(it.block), func(i int) bool {
				return it.block[i].UID >= target
			})
			it.inBlockPos = pos
			if pos < len(it.block) {
				return true
			}
			// Fall through to next block.
		}
		blockIdx++
		// Update target to the next block's firstUID.
		if blockIdx < len(it.dir.Blocks) {
			target = it.dir.Blocks[blockIdx].FirstUID
		}
	}
	it.exhausted = true
	return false
}

// findBlockForTarget returns the block index that should contain target.
func (it *listIter) findBlockForTarget(target uint64) int {
	blocks := it.dir.Blocks
	idx := sort.Search(len(blocks), func(i int) bool {
		return blocks[i].FirstUID > target
	})
	if idx > 0 {
		return idx - 1
	}
	return 0
}

// loadBlock decodes the block at the given directory index.
func (it *listIter) loadBlock(idx int) {
	if it.legacy {
		// Legacy mode: single pre-loaded block; don't reset position.
		return
	}
	bm := it.dir.Blocks[idx]
	blockKey := x.BM25TermBlockKey(it.attr, it.encodedTerm, bm.BlockID)
	blob := posting.ReadBM25BlobAt(blockKey, it.readTs)
	it.block = bm25enc.Decode(blob)
	it.inBlockPos = 0
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

// tryPush adds a doc if it beats the current threshold. Returns true if the
// threshold changed.
func (h *topKHeap) tryPush(uid uint64, score float64) bool {
	if len(h.docs) < h.k {
		heap.Push(h, scoredDoc{uid: uid, score: score})
		return len(h.docs) == h.k // threshold only meaningful once heap is full
	}
	if score > h.docs[0].score {
		h.docs[0] = scoredDoc{uid: uid, score: score}
		heap.Fix(h, 0)
		return true
	}
	return false
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

// bm25Score computes the BM25 score for a single term occurrence.
func bm25Score(idf, tf, dl, avgDL, k, b float64) float64 {
	return idf * (k + 1) * tf / (k*(1-b+b*dl/avgDL) + tf)
}

// docLenCache caches document length lookups within a single query to avoid
// repeated Badger reads for the same doclen block directory and blocks.
type docLenCache struct {
	attr   string
	readTs uint64
	dir    *bm25block.Dir
	loaded bool
	legacy bool
	// Per-block cache: blockIdx -> decoded entries.
	blocks map[int][]bm25enc.Entry
	// Legacy entries (when using monolithic blob).
	legacyEntries []bm25enc.Entry
}

func newDocLenCache(attr string, readTs uint64) *docLenCache {
	return &docLenCache{
		attr:   attr,
		readTs: readTs,
		blocks: make(map[int][]bm25enc.Entry),
	}
}

func (c *docLenCache) ensureLoaded() {
	if c.loaded {
		return
	}
	c.loaded = true
	dirKey := x.BM25DocLenDirKey(c.attr)
	dirBlob := posting.ReadBM25BlobAt(dirKey, c.readTs)
	c.dir = bm25block.DecodeDir(dirBlob)
	if len(c.dir.Blocks) == 0 {
		// Try legacy.
		legacyKey := x.BM25DocLenKey(c.attr)
		legacyBlob := posting.ReadBM25BlobAt(legacyKey, c.readTs)
		c.legacyEntries = bm25enc.Decode(legacyBlob)
		c.legacy = true
	}
}

func (c *docLenCache) lookup(uid uint64) float64 {
	c.ensureLoaded()
	if c.legacy {
		if v, ok := bm25enc.Search(c.legacyEntries, uid); ok {
			return float64(v)
		}
		return 1.0
	}
	if len(c.dir.Blocks) == 0 {
		return 1.0
	}
	blockIdx := c.dir.FindBlock(uid)
	entries, ok := c.blocks[blockIdx]
	if !ok {
		bm := c.dir.Blocks[blockIdx]
		blockKey := x.BM25DocLenBlockKey(c.attr, bm.BlockID)
		blob := posting.ReadBM25BlobAt(blockKey, c.readTs)
		entries = bm25enc.Decode(blob)
		c.blocks[blockIdx] = entries
	}
	if v, ok := bm25enc.Search(entries, uid); ok {
		return float64(v)
	}
	return 1.0
}

// wandSearch performs a WAND top-k search over block-based posting lists.
// If topK <= 0, it scores all matching documents (no early termination).
func wandSearch(attr string, readTs uint64, queryTokens []string,
	k, b, avgDL, N float64, topK int, filterSet map[uint64]struct{},
	useBMW bool) []scoredDoc {

	dlCache := newDocLenCache(attr, readTs)

	// Build iterators for each query term.
	var iters []*listIter
	for _, token := range queryTokens {
		// Compute df: try block directory first, then fall back to legacy blob.
		var df uint64
		dirKey := x.BM25TermDirKey(attr, token)
		dirBlob := posting.ReadBM25BlobAt(dirKey, readTs)
		dir := bm25block.DecodeDir(dirBlob)
		if len(dir.Blocks) > 0 {
			for _, bm := range dir.Blocks {
				df += uint64(bm.Count)
			}
		} else {
			// Legacy fallback: read the monolithic blob to get df.
			legacyKey := x.BM25IndexKey(attr, token)
			legacyBlob := posting.ReadBM25BlobAt(legacyKey, readTs)
			legacyEntries := bm25enc.Decode(legacyBlob)
			df = uint64(len(legacyEntries))
		}
		if df == 0 {
			continue
		}
		idf := math.Log1p((N - float64(df) + 0.5) / (float64(df) + 0.5))

		it := newListIter(attr, token, readTs, idf, k, b, dir)
		if !it.exhausted {
			it.next() // prime the iterator
			if !it.exhausted {
				iters = append(iters, it)
			}
		}
	}

	if len(iters) == 0 {
		return nil
	}

	// If no top-k limit, score all matching documents.
	if topK <= 0 {
		return scoreAllDocs(iters, dlCache, k, b, avgDL, filterSet)
	}

	// WAND algorithm with top-k heap.
	h := &topKHeap{k: topK}
	heap.Init(h)

	for {
		// Remove exhausted iterators.
		active := iters[:0]
		for _, it := range iters {
			if !it.exhausted {
				active = append(active, it)
			}
		}
		iters = active
		if len(iters) == 0 {
			break
		}

		// Sort iterators by currentDoc ascending.
		sort.Slice(iters, func(i, j int) bool {
			return iters[i].currentDoc() < iters[j].currentDoc()
		})

		theta := h.threshold()

		// Find pivot: accumulate UBs until they exceed theta.
		var sumUB float64
		pivot := -1
		var pivotDoc uint64
		for i, it := range iters {
			sumUB += it.remainingUB()
			if sumUB > theta && pivot == -1 {
				pivot = i
				pivotDoc = it.currentDoc()
			}
		}
		// sumUB now contains the total UB across ALL iterators (needed for BMW).
		if pivot == -1 {
			break // sum of all UBs can't beat theta
		}

		// Advance all iterators before pivot to pivotDoc.
		allAtPivot := true
		for i := 0; i < pivot; i++ {
			if iters[i].currentDoc() < pivotDoc {
				var ok bool
				if useBMW {
					// Compute otherUB = total UB - this iter's UB (O(1) instead of O(q)).
					otherUB := sumUB - iters[i].remainingUB()
					ok = iters[i].skipToWithBMW(pivotDoc, theta, otherUB)
				} else {
					ok = iters[i].skipTo(pivotDoc)
				}
				if !ok {
					allAtPivot = false
					break
				}
				if iters[i].currentDoc() != pivotDoc {
					allAtPivot = false
				}
			}
		}

		if !allAtPivot {
			continue // re-evaluate after advances
		}

		// All iterators up to pivot are at pivotDoc. Score the candidate.
		if filterSet != nil {
			if _, ok := filterSet[pivotDoc]; !ok {
				// Skip this doc (filtered out). Advance all iters at pivotDoc.
				for _, it := range iters {
					if it.currentDoc() == pivotDoc {
						it.next()
					}
				}
				continue
			}
		}

		dl := dlCache.lookup(pivotDoc)
		var score float64
		for _, it := range iters {
			if it.currentDoc() == pivotDoc {
				tf := float64(it.currentTF())
				score += bm25Score(it.idf, tf, dl, avgDL, k, b)
			}
		}
		h.tryPush(pivotDoc, score)

		// Advance all iterators at pivotDoc.
		for _, it := range iters {
			if it.currentDoc() == pivotDoc {
				it.next()
			}
		}
	}

	return h.sorted()
}

// scoreAllDocs scores every matching document without early termination.
// Used when no top-k limit is specified (the original behavior).
func scoreAllDocs(iters []*listIter, dlCache *docLenCache,
	k, b, avgDL float64, filterSet map[uint64]struct{}) []scoredDoc {

	// Collect all (uid, term) matches.
	type termMatch struct {
		idf float64
		tf  uint32
	}
	matches := make(map[uint64][]termMatch)

	for _, it := range iters {
		for !it.exhausted {
			uid := it.currentDoc()
			tf := it.currentTF()
			if filterSet == nil {
				matches[uid] = append(matches[uid], termMatch{idf: it.idf, tf: tf})
			} else if _, ok := filterSet[uid]; ok {
				matches[uid] = append(matches[uid], termMatch{idf: it.idf, tf: tf})
			}
			it.next()
		}
	}

	// Score all matching documents.
	results := make([]scoredDoc, 0, len(matches))
	for uid, terms := range matches {
		dl := dlCache.lookup(uid)
		var score float64
		for _, tm := range terms {
			score += bm25Score(tm.idf, float64(tm.tf), dl, avgDL, k, b)
		}
		results = append(results, scoredDoc{uid: uid, score: score})
	}

	// Sort by score descending, then UID ascending.
	sort.Slice(results, func(i, j int) bool {
		if results[i].score != results[j].score {
			return results[i].score > results[j].score
		}
		return results[i].uid < results[j].uid
	})
	return results
}
