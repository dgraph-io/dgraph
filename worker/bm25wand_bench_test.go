/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/dgraph-io/dgraph/v25/posting"
)

// genBenchPostings builds n UID-ascending postings with realistic tf/doclen spreads.
func genBenchPostings(n int, rng *rand.Rand) []posting.BM25Posting {
	ps := make([]posting.BM25Posting, n)
	uid := uint64(0)
	for i := range ps {
		uid += uint64(1 + rng.Intn(16))
		ps[i] = posting.BM25Posting{
			Uid:    uid,
			TF:     uint32(1 + rng.Intn(8)),
			DocLen: uint32(5 + rng.Intn(200)),
		}
	}
	return ps
}

func benchCursors(postings [][]posting.BM25Posting, idfs []float64,
	k, b, avgDL float64) []*termCursor {
	cs := make([]*termCursor, len(postings))
	for i := range postings {
		cs[i] = newTermCursor(postings[i], idfs[i], k, b, avgDL)
	}
	return cs
}

// BenchmarkBM25Search compares the three scoring strategies over identical postings:
// WAND top-k, Block-Max WAND top-k, and exhaustive scoring. Cursor construction
// (per-block upper-bound precompute) is part of every real query, so it is included
// in each iteration; its standalone cost is measured by the cursors sub-benchmark.
// The point of the top-k benchmarks is to verify pruning actually beats scoreAll —
// if wandTopK10 is not clearly faster than scoreAll at the larger sizes, WAND's
// early-termination is broken even if results are correct.
func BenchmarkBM25Search(b *testing.B) {
	k, bb, avgDL := 1.2, 0.75, 100.0
	idfs := []float64{0.4, 1.1, 2.5} // common, medium, rare term

	for _, n := range []int{1_000, 100_000, 1_000_000} {
		rng := rand.New(rand.NewSource(7))
		postings := [][]posting.BM25Posting{
			genBenchPostings(n, rng),      // common term
			genBenchPostings(n/4+1, rng),  // medium term
			genBenchPostings(n/50+1, rng), // rare term
		}

		b.Run(fmt.Sprintf("cursors/n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = benchCursors(postings, idfs, k, bb, avgDL)
			}
		})
		b.Run(fmt.Sprintf("wandTopK10/n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = wandTopK(benchCursors(postings, idfs, k, bb, avgDL), k, bb, avgDL,
					10, nil, false)
			}
		})
		b.Run(fmt.Sprintf("bmwTopK10/n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = wandTopK(benchCursors(postings, idfs, k, bb, avgDL), k, bb, avgDL,
					10, nil, true)
			}
		})
		b.Run(fmt.Sprintf("scoreAll/n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = scoreAllDocs(benchCursors(postings, idfs, k, bb, avgDL), k, bb, avgDL, nil)
			}
		})
	}
}
