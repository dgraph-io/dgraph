/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import (
	"container/heap"
	"math"
	"math/rand"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/posting"
)

func TestTopKHeapBasic(t *testing.T) {
	h := &topKHeap{k: 3}
	heap.Init(h)

	require.Equal(t, 0.0, h.threshold())

	h.tryPush(1, 5.0)
	h.tryPush(2, 3.0)
	require.Equal(t, 0.0, h.threshold()) // not full yet

	h.tryPush(3, 7.0)
	require.InEpsilon(t, 3.0, h.threshold(), 1e-9) // full, min is 3.0

	h.tryPush(4, 4.0)
	require.InEpsilon(t, 4.0, h.threshold(), 1e-9) // 3.0 evicted, min is now 4.0

	// 2.0 shouldn't be accepted.
	h.tryPush(5, 2.0)
	require.InEpsilon(t, 4.0, h.threshold(), 1e-9)

	sorted := h.sorted()
	require.Len(t, sorted, 3)
	require.Equal(t, uint64(3), sorted[0].uid) // highest score (7.0)
	require.Equal(t, uint64(1), sorted[1].uid) // 5.0
	require.Equal(t, uint64(4), sorted[2].uid) // 4.0
}

func TestTopKHeapTieBreaking(t *testing.T) {
	h := &topKHeap{k: 5}
	heap.Init(h)

	// Same score, different UIDs — should sort by UID ascending.
	h.tryPush(10, 5.0)
	h.tryPush(5, 5.0)
	h.tryPush(15, 5.0)

	sorted := h.sorted()
	require.Equal(t, uint64(5), sorted[0].uid)
	require.Equal(t, uint64(10), sorted[1].uid)
	require.Equal(t, uint64(15), sorted[2].uid)
}

// TestTopKHeapTieBreakEviction pins the boundary case the randomized brute-force
// tests deliberately skip (they treat tied-boundary uids as interchangeable). Docs
// arrive UID-ascending, as the WAND pivot advances. With k=2, two docs tied at score
// 1.0 (uid 3, uid 4) then a strictly higher doc (uid 5), the correct top-2 by
// (score desc, UID asc) is {5, 3}: among the tied pair uid3 (lower) outranks uid4,
// matching the no-limit scoreAllDocs path. A score-only heap evicts the lowest-UID
// root and wrongly returns {5, 4}.
func TestTopKHeapTieBreakEviction(t *testing.T) {
	h := &topKHeap{k: 2}
	heap.Init(h)
	h.tryPush(3, 1.0)
	h.tryPush(4, 1.0)
	h.tryPush(5, 2.0)

	sorted := h.sorted()
	uids := make([]uint64, len(sorted))
	for i, d := range sorted {
		uids[i] = d.uid
	}
	require.Equal(t, []uint64{5, 3}, uids,
		"WAND top-k must keep the lowest UID among score ties, matching scoreAllDocs")

	// The eviction threshold must remain the true minimum score after the tie-break.
	require.InEpsilon(t, 1.0, h.threshold(), 1e-9)
}

func TestBm25TopK(t *testing.T) {
	// No first limit: score every matching document (0 means "no early termination").
	require.Equal(t, 0, bm25TopK(0, 0))
	require.Equal(t, 0, bm25TopK(0, 100))

	// With a first limit, WAND must retain first+offset documents so the offset can be
	// dropped afterward — NOT 0 (which would fall back to scoring the entire corpus
	// just because an offset was supplied, the memory blow-up this guards against).
	require.Equal(t, 10, bm25TopK(10, 0))
	require.Equal(t, 15, bm25TopK(10, 5))
	require.Equal(t, 1001, bm25TopK(1, 1000))
}

func TestBm25PaginateScored(t *testing.T) {
	mk := func(uids ...uint64) []scoredDoc {
		out := make([]scoredDoc, len(uids))
		for i, u := range uids {
			out[i] = scoredDoc{uid: u, score: float64(len(uids) - i)} // already score-descending
		}
		return out
	}
	ids := func(ds []scoredDoc) []uint64 {
		out := make([]uint64, len(ds))
		for i, d := range ds {
			out[i] = d.uid
		}
		return out
	}

	full := mk(1, 2, 3, 4, 5)
	require.Equal(t, []uint64{1, 2, 3, 4, 5}, ids(bm25PaginateScored(full, 0, 0)))
	require.Equal(t, []uint64{1, 2}, ids(bm25PaginateScored(mk(1, 2, 3, 4, 5), 2, 0)))
	require.Equal(t, []uint64{3, 4}, ids(bm25PaginateScored(mk(1, 2, 3, 4, 5), 2, 2)))
	require.Equal(t, []uint64{4, 5}, ids(bm25PaginateScored(mk(1, 2, 3, 4, 5), 10, 3)))
	// Offset past the end yields nothing rather than panicking.
	require.Empty(t, bm25PaginateScored(mk(1, 2, 3), 2, 10))
}

func TestBm25ScoreFunction(t *testing.T) {
	k, b := 1.2, 0.75
	avgDL := 10.0

	// idf * (k+1) * tf / (k*(1-b+b*dl/avgDL) + tf)
	idf := 1.5
	tf := 3.0
	dl := 10.0

	expected := idf * (k + 1) * tf / (k*(1-b+b*dl/avgDL) + tf)
	got := bm25Score(idf, tf, dl, avgDL, k, b)
	require.InEpsilon(t, expected, got, 1e-9)

	// With b=0: no length normalization.
	expected0 := idf * (k + 1) * tf / (k + tf)
	got0 := bm25Score(idf, tf, dl, avgDL, k, 0)
	require.InEpsilon(t, expected0, got0, 1e-9)

	// Score should be positive for positive inputs.
	require.Greater(t, bm25Score(1.0, 1.0, 5.0, 10.0, k, b), 0.0)

	// Higher tf should produce higher score (same dl).
	s1 := bm25Score(idf, 1.0, dl, avgDL, k, b)
	s3 := bm25Score(idf, 3.0, dl, avgDL, k, b)
	require.Greater(t, s3, s1)

	// Shorter doc should score higher (same tf).
	sShort := bm25Score(idf, tf, 5.0, avgDL, k, b)
	sLong := bm25Score(idf, tf, 20.0, avgDL, k, b)
	require.Greater(t, sShort, sLong)
}

func TestBm25ScoreNaN(t *testing.T) {
	// Ensure no NaN/Inf for edge-case inputs.
	score := bm25Score(0.5, 1.0, 0.0, 10.0, 1.2, 0.75)
	require.False(t, math.IsNaN(score))
	require.False(t, math.IsInf(score, 0))
	require.Greater(t, score, 0.0)
}

// brute force scores every doc across all cursors (ground truth for WAND). When
// filterSet is non-nil, only documents in it are scored — mirroring @filter(bm25(...)).
func bruteForceTopK(termPostings [][]posting.BM25Posting, idfs []float64,
	k, b, avgDL float64, topK int) []scoredDoc {
	return bruteForceTopKFiltered(termPostings, idfs, k, b, avgDL, topK, nil)
}

func bruteForceTopKFiltered(termPostings [][]posting.BM25Posting, idfs []float64,
	k, b, avgDL float64, topK int, filterSet map[uint64]struct{}) []scoredDoc {
	scores := map[uint64]float64{}
	for ti, ps := range termPostings {
		for _, p := range ps {
			if filterSet != nil {
				if _, ok := filterSet[p.Uid]; !ok {
					continue
				}
			}
			scores[p.Uid] += bm25Score(idfs[ti], float64(p.TF), float64(p.DocLen), avgDL, k, b)
		}
	}
	out := make([]scoredDoc, 0, len(scores))
	for uid, s := range scores {
		out = append(out, scoredDoc{uid: uid, score: s})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		return out[i].uid < out[j].uid
	})
	if topK > 0 && len(out) > topK {
		out = out[:topK]
	}
	return out
}

// TestWandFilteredMatchesBruteForce checks that WAND/Block-Max WAND and the
// score-all path honor a filter set identically to exhaustive filtered scoring. The
// filter must never change which documents or scores are produced (only which are
// considered), so WAND pruning driven by a threshold built from filtered-in documents
// must still be sound.
func TestWandFilteredMatchesBruteForce(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	k, b, avgDL := 1.2, 0.75, 9.0

	for trial := 0; trial < 200; trial++ {
		numTerms := 1 + rng.Intn(4)
		termPostings := make([][]posting.BM25Posting, numTerms)
		idfs := make([]float64, numTerms)
		allUids := map[uint64]bool{}
		for ti := 0; ti < numTerms; ti++ {
			n := rng.Intn(400)
			seen := map[uint64]bool{}
			var ps []posting.BM25Posting
			for j := 0; j < n; j++ {
				uid := uint64(1 + rng.Intn(500))
				if seen[uid] {
					continue
				}
				seen[uid] = true
				ps = append(ps, posting.BM25Posting{
					Uid:    uid,
					TF:     uint32(1 + rng.Intn(10)),
					DocLen: uint32(1 + rng.Intn(30)),
				})
				allUids[uid] = true
			}
			sort.Slice(ps, func(i, j int) bool { return ps[i].Uid < ps[j].Uid })
			termPostings[ti] = ps
			idfs[ti] = 0.5 + rng.Float64()*2
		}

		// Random filter subset (may be empty).
		filterSet := map[uint64]struct{}{}
		for uid := range allUids {
			if rng.Intn(2) == 0 {
				filterSet[uid] = struct{}{}
			}
		}

		build := func() []*termCursor {
			cs := make([]*termCursor, 0, numTerms)
			for ti, ps := range termPostings {
				if len(ps) == 0 {
					continue
				}
				cs = append(cs, newTermCursor(ps, idfs[ti], k, b, avgDL))
			}
			return cs
		}

		// score-all path with filter must reproduce the full filtered ranking exactly.
		wantAll := bruteForceTopKFiltered(termPostings, idfs, k, b, avgDL, 0, filterSet)
		gotAll := scoreAllDocs(build(), k, b, avgDL, filterSet)
		require.Lenf(t, gotAll, len(wantAll), "trial %d filtered score-all len", trial)
		for i := range wantAll {
			require.InEpsilonf(t, wantAll[i].score, gotAll[i].score, 1e-9,
				"trial %d filtered score-all rank %d score", trial, i)
		}

		// top-k WAND/BMW with filter must match the filtered top-k scores.
		topK := 1 + rng.Intn(8)
		want := bruteForceTopKFiltered(termPostings, idfs, k, b, avgDL, topK, filterSet)
		wantPlus := bruteForceTopKFiltered(termPostings, idfs, k, b, avgDL, topK+1, filterSet)
		for _, useBMW := range []bool{false, true} {
			got := wandTopK(build(), k, b, avgDL, topK, filterSet, useBMW)
			require.Lenf(t, got, len(want), "trial %d filtered bmw=%v len", trial, useBMW)
			for i := range want {
				require.InEpsilonf(t, want[i].score, got[i].score, 1e-9,
					"trial %d filtered bmw=%v rank %d score", trial, useBMW, i)
				tied := (i > 0 && wantPlus[i].score == wantPlus[i-1].score) ||
					(i+1 < len(wantPlus) && wantPlus[i].score == wantPlus[i+1].score)
				if !tied {
					require.Equalf(t, want[i].uid, got[i].uid,
						"trial %d filtered bmw=%v rank %d uid", trial, useBMW, i)
				}
			}
		}
	}
}

// TestWandMatchesBruteForce checks that WAND and Block-Max WAND return exactly the
// same top-k documents and scores as exhaustive scoring, across many randomized
// posting lists. This is the core correctness guarantee: pruning must never change
// the result, only the work done.
func TestWandMatchesBruteForce(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	k, b, avgDL := 1.2, 0.75, 12.0

	for trial := 0; trial < 200; trial++ {
		numTerms := 1 + rng.Intn(4)
		termPostings := make([][]posting.BM25Posting, numTerms)
		idfs := make([]float64, numTerms)
		for ti := 0; ti < numTerms; ti++ {
			n := rng.Intn(400) // spans multiple wandBlockSize blocks
			seen := map[uint64]bool{}
			var ps []posting.BM25Posting
			for j := 0; j < n; j++ {
				uid := uint64(1 + rng.Intn(500))
				if seen[uid] {
					continue
				}
				seen[uid] = true
				ps = append(ps, posting.BM25Posting{
					Uid:    uid,
					TF:     uint32(1 + rng.Intn(10)),
					DocLen: uint32(1 + rng.Intn(30)),
				})
			}
			sort.Slice(ps, func(i, j int) bool { return ps[i].Uid < ps[j].Uid })
			termPostings[ti] = ps
			// Vary IDF per term so different terms carry different weight.
			idfs[ti] = 0.5 + rng.Float64()*2
		}

		topK := 1 + rng.Intn(10)
		want := bruteForceTopK(termPostings, idfs, k, b, avgDL, topK)
		// One extra result lets us detect a tie between the cutoff rank and the
		// first excluded document (a boundary tie outside the top-k window).
		wantPlus := bruteForceTopK(termPostings, idfs, k, b, avgDL, topK+1)

		build := func() []*termCursor {
			cs := make([]*termCursor, 0, numTerms)
			for ti, ps := range termPostings {
				if len(ps) == 0 {
					continue
				}
				cs = append(cs, newTermCursor(ps, idfs[ti], k, b, avgDL))
			}
			return cs
		}

		for _, useBMW := range []bool{false, true} {
			got := wandTopK(build(), k, b, avgDL, topK, nil, useBMW)
			require.Lenf(t, got, len(want), "trial %d bmw=%v len", trial, useBMW)
			for i := range want {
				// The score at each rank must match exactly: WAND/BMW pruning must
				// never change which scores make the top-k, only the work done.
				require.InEpsilonf(t, want[i].score, got[i].score, 1e-9,
					"trial %d bmw=%v rank %d score", trial, useBMW, i)
				// The uid is only guaranteed when this rank's score is not tied with
				// a neighbor (including the first excluded doc); tied-boundary docs
				// are interchangeable in the ranking.
				tied := (i > 0 && wantPlus[i].score == wantPlus[i-1].score) ||
					(i+1 < len(wantPlus) && wantPlus[i].score == wantPlus[i+1].score)
				if !tied {
					require.Equalf(t, want[i].uid, got[i].uid,
						"trial %d bmw=%v rank %d uid", trial, useBMW, i)
				}
			}
		}
	}
}
