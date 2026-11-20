/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package bulk

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/posting"
)

// TestMergeBM25Stats verifies that per-mapper corpus-statistics partials are summed
// across mappers per (predicate, bucket). This is the linchpin of correct bulk BM25
// stats: each mapper sees a disjoint subset of documents, so the final doc count and
// total term count for a bucket must be the sum of every mapper's partial — never just
// one mapper's (which a unioned/last-write-wins posting would produce).
func TestMergeBM25Stats(t *testing.T) {
	mk := func(attr string, bucket int, count, terms int64) *mapper {
		m := &mapper{bm25Stats: map[string]*bm25StatEntry{}}
		e := &bm25StatEntry{}
		e.count[bucket] = count
		e.terms[bucket] = terms
		m.bm25Stats[attr] = e
		return m
	}

	mappers := []*mapper{
		mk("name", 1, 3, 30),
		mk("name", 1, 2, 25), // same predicate+bucket as above -> must sum
		mk("name", 5, 4, 40), // same predicate, different bucket
		mk("bio", 1, 7, 70),  // different predicate
		nil,                  // released mappers are skipped
	}

	merged := mergeBM25Stats(mappers)
	require.Len(t, merged, 2)

	require.Equal(t, int64(5), merged["name"].count[1], "bucket 1 doc count must sum across mappers")
	require.Equal(t, int64(55), merged["name"].terms[1], "bucket 1 term count must sum across mappers")
	require.Equal(t, int64(4), merged["name"].count[5])
	require.Equal(t, int64(40), merged["name"].terms[5])
	require.Equal(t, int64(7), merged["bio"].count[1])
	require.Equal(t, int64(70), merged["bio"].terms[1])

	// Untouched buckets stay zero.
	require.Equal(t, int64(0), merged["name"].count[0])
	require.Equal(t, int64(0), merged["bio"].count[5])

	// Total doc count across the predicate equals the sum of all contributing mappers.
	var nameDocs int64
	for b := 0; b < posting.NumBM25StatsBuckets; b++ {
		nameDocs += merged["name"].count[b]
	}
	require.Equal(t, int64(9), nameDocs)
}
