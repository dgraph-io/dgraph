/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

// Q31 WAND effectiveness benchmarks. Synthetic corpora (10k and 100k docs) with
// Zipfian term selectivity — a rare term (df ~0.1%), a mid term (df ~5%), and a
// stopword-like term (df ~60%) — are fed directly into wandTopK / scoreAllDocs,
// mirroring what wandSearch does after ReadBM25TermPostings materializes each
// term's posting list. The matrix covers 1–3 term queries, topK in {10, 100} for
// WAND (BMW on and off) against the topK=0 exhaustive scoreAllDocs path, plus the
// deep-offset shape (first:10 offset:10000 => topK=10010 via bm25TopK) so the
// early-termination crossover is visible.
//
// Each benchmark reports two domain metrics alongside ns/op:
//   - docs_scored/op: documents fully scored (tryPush'd for WAND, accumulated for
//     scoreAllDocs), counted by a replica of the wandTopK loop since the product
//     code exposes no counter. The replica is checked bit-identical against the
//     real wandTopK before timing, so it cannot drift silently. The count is
//     deterministic per config, computed off the clock, and reported after the
//     timed loop (b.ResetTimer clears metrics reported before it).
//   - postings/op: total materialized postings across the query's cursors — the
//     work floor that full materialization imposes regardless of pruning.
//
// Cursor construction (newTermCursor block-bound precompute) is inside the timed
// loop, matching wandSearch, which rebuilds cursors per query.

import (
	"container/heap"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"testing"

	"github.com/dgraph-io/dgraph/v25/posting"
)

const benchSeed = 20260718

// benchTerm is one query term's materialized posting list plus its smoothed IDF,
// exactly as wandSearch computes it.
type benchTerm struct {
	postings []posting.BM25Posting
	idf      float64
}

// benchCorpus is a deterministic synthetic corpus: per-doc lengths shared across
// terms, and three terms tiered by document frequency.
type benchCorpus struct {
	numDocs int
	avgDL   float64
	terms   map[string]benchTerm
}

var benchCorpora = map[int]*benchCorpus{}

// getBenchCorpus builds (once per size) a corpus of numDocs documents with
// uniform doc lengths in [5, 80] and Zipf-distributed term frequencies, then
// materializes posting lists for the three selectivity tiers:
//
//	rare ~0.1% df, mid ~5% df, stop ~60% df.
//
// Membership is an independent coin flip per (doc, term) at the tier's rate, so
// postings interleave uniformly across the UID space (UID-ascending, as real
// posting lists are). Benchmarks share corpora; cursors are rebuilt per iteration
// because they are stateful.
func getBenchCorpus(numDocs int) *benchCorpus {
	if c, ok := benchCorpora[numDocs]; ok {
		return c
	}
	rng := rand.New(rand.NewSource(benchSeed + int64(numDocs)))
	zipfTF := rand.NewZipf(rng, 1.5, 1, 9) // tf-1 in 0..9, Zipf-skewed toward 0

	docLens := make([]uint32, numDocs+1) // 1-based UIDs
	var totalLen float64
	for uid := 1; uid <= numDocs; uid++ {
		docLens[uid] = uint32(5 + rng.Intn(76))
		totalLen += float64(docLens[uid])
	}
	avgDL := totalLen / float64(numDocs)

	c := &benchCorpus{numDocs: numDocs, avgDL: avgDL, terms: map[string]benchTerm{}}
	for _, tier := range []struct {
		name string
		rate float64
	}{
		{"rare", 0.001},
		{"mid", 0.05},
		{"stop", 0.60},
	} {
		var ps []posting.BM25Posting
		for uid := 1; uid <= numDocs; uid++ {
			if rng.Float64() >= tier.rate {
				continue
			}
			ps = append(ps, posting.BM25Posting{
				Uid:    uint64(uid),
				TF:     uint32(1 + zipfTF.Uint64()),
				DocLen: docLens[uid],
			})
		}
		// Smoothed IDF exactly as wandSearch computes it (N >= df by construction).
		df := float64(len(ps))
		idf := math.Log1p((float64(numDocs) - df + 0.5) / (df + 0.5))
		c.terms[tier.name] = benchTerm{postings: ps, idf: idf}
	}
	benchCorpora[numDocs] = c
	return c
}

// buildBenchCursors materializes fresh cursors for the named terms, as wandSearch
// does per query.
func buildBenchCursors(c *benchCorpus, termNames []string, k, b float64) []*termCursor {
	cs := make([]*termCursor, 0, len(termNames))
	for _, name := range termNames {
		t := c.terms[name]
		if len(t.postings) == 0 {
			continue
		}
		cs = append(cs, newTermCursor(t.postings, t.idf, k, b, c.avgDL))
	}
	return cs
}

func benchTotalPostings(c *benchCorpus, termNames []string) int {
	var n int
	for _, name := range termNames {
		n += len(c.terms[name].postings)
	}
	return n
}

// wandTopKCounting is a line-for-line replica of wandTopK (filterSet elided —
// the benchmarks pass nil) that additionally counts pivot documents fully scored.
// The product code exposes no counter, so the count lives in this harness copy;
// benchmarks verify its output bit-identical to the real wandTopK before timing.
func wandTopKCounting(cursors []*termCursor, k, b, avgDL float64, topK int,
	useBMW bool) (results []scoredDoc, docsScored int) {

	h := &topKHeap{k: topK}
	heap.Init(h)

	for {
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

		sort.Slice(cursors, func(i, j int) bool {
			return cursors[i].currentDoc() < cursors[j].currentDoc()
		})

		theta := h.threshold()

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
			break
		}

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

		var score float64
		for _, c := range cursors {
			if c.currentDoc() == pivotDoc {
				dl := float64(c.currentDocLen())
				score += bm25Score(c.idf, float64(c.currentTF()), dl, avgDL, k, b)
			}
		}
		docsScored++
		h.tryPush(pivotDoc, score)

		for _, c := range cursors {
			if c.currentDoc() == pivotDoc {
				c.next()
			}
		}
	}

	return h.sorted(), docsScored
}

// benchWandDocsScored computes docs_scored for one config via the counting
// replica and guards the replica against drift from the real wandTopK
// (bit-identical uids and scores). Called before the timed loop; the caller
// must ReportMetric AFTER the loop, because b.ResetTimer clears extra metrics.
func benchWandDocsScored(b *testing.B, c *benchCorpus, termNames []string,
	k, bp float64, topK int, useBMW bool) int {

	want := wandTopK(buildBenchCursors(c, termNames, k, bp), k, bp, c.avgDL,
		topK, nil, useBMW)
	got, docsScored := wandTopKCounting(buildBenchCursors(c, termNames, k, bp),
		k, bp, c.avgDL, topK, useBMW)
	if len(got) != len(want) {
		b.Fatalf("counting replica drifted from wandTopK: len %d != %d", len(got), len(want))
	}
	for i := range want {
		if got[i].uid != want[i].uid ||
			math.Float64bits(got[i].score) != math.Float64bits(want[i].score) {
			b.Fatalf("counting replica drifted from wandTopK at rank %d: "+
				"got (%d, %v) want (%d, %v)", i, got[i].uid, got[i].score,
				want[i].uid, want[i].score)
		}
	}
	return docsScored
}

// BenchmarkWandEffectiveness is the Q31 matrix: corpus size x query shape x
// {exhaustive, wand/bmw x topK}. scoreAll is the topK=0 path a query with no
// first: limit takes; wand/bmw topK=10/100 are the first:10/first:100 windows.
func BenchmarkWandEffectiveness(b *testing.B) {
	const k, bp = 1.2, 0.75

	queries := []struct {
		name  string
		terms []string
	}{
		{"q=1term-rare", []string{"rare"}},
		{"q=1term-stop", []string{"stop"}},
		{"q=2term-rare+stop", []string{"rare", "stop"}},
		{"q=3term-rare+mid+stop", []string{"rare", "mid", "stop"}},
	}

	for _, size := range []struct {
		name string
		docs int
	}{
		{"docs=10k", 10_000},
		{"docs=100k", 100_000},
	} {
		c := getBenchCorpus(size.docs)
		for _, q := range queries {
			prefix := size.name + "/" + q.name

			// topK=0: the exhaustive scoreAllDocs path (no first: limit).
			b.Run(prefix+"/algo=scoreAll/topK=0", func(b *testing.B) {
				full := scoreAllDocs(buildBenchCursors(c, q.terms, k, bp), k, bp,
					c.avgDL, nil)
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					cursors := buildBenchCursors(c, q.terms, k, bp)
					_ = scoreAllDocs(cursors, k, bp, c.avgDL, nil)
				}
				// After the loop: ResetTimer clears metrics reported before it.
				b.ReportMetric(float64(len(full)), "docs_scored/op")
				b.ReportMetric(float64(benchTotalPostings(c, q.terms)), "postings/op")
			})

			for _, algo := range []struct {
				name   string
				useBMW bool
			}{
				{"algo=wand", false},
				{"algo=bmw", true},
			} {
				for _, topK := range []int{10, 100} {
					name := prefix + "/" + algo.name + "/topK=" + strconv.Itoa(topK)
					useBMW := algo.useBMW
					topK := topK
					b.Run(name, func(b *testing.B) {
						docsScored := benchWandDocsScored(b, c, q.terms, k, bp, topK, useBMW)
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							cursors := buildBenchCursors(c, q.terms, k, bp)
							_ = wandTopK(cursors, k, bp, c.avgDL, topK, nil, useBMW)
						}
						b.ReportMetric(float64(docsScored), "docs_scored/op")
						b.ReportMetric(float64(benchTotalPostings(c, q.terms)), "postings/op")
					})
				}
			}
		}
	}
}

// BenchmarkWandDeepOffset is the deep-pagination shape on the 100k corpus:
// first:10 offset:0 retains topK=10, while first:10 offset:10000 retains
// topK=10010 (bm25TopK), so the heap threshold stays 0 for the first 10010 docs
// and early termination degrades toward exhaustive.
func BenchmarkWandDeepOffset(b *testing.B) {
	const k, bp = 1.2, 0.75
	c := getBenchCorpus(100_000)
	terms := []string{"rare", "mid", "stop"}

	for _, cfg := range []struct {
		name  string
		first int
		off   int
	}{
		{"first=10/offset=0", 10, 0},
		{"first=10/offset=10000", 10, 10000},
	} {
		topK := bm25TopK(cfg.first, cfg.off)
		for _, algo := range []struct {
			name   string
			useBMW bool
		}{
			{"algo=wand", false},
			{"algo=bmw", true},
		} {
			useBMW := algo.useBMW
			b.Run(cfg.name+"/"+algo.name+"/topK="+strconv.Itoa(topK), func(b *testing.B) {
				docsScored := benchWandDocsScored(b, c, terms, k, bp, topK, useBMW)
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					cursors := buildBenchCursors(c, terms, k, bp)
					_ = wandTopK(cursors, k, bp, c.avgDL, topK, nil, useBMW)
				}
				b.ReportMetric(float64(docsScored), "docs_scored/op")
				b.ReportMetric(float64(benchTotalPostings(c, terms)), "postings/op")
			})
		}
	}
}
