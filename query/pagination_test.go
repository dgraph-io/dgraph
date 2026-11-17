/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package query

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/schema"
	"github.com/dgraph-io/dgraph/v25/x"
)

func initPaginationSchema(t *testing.T) {
	schema.Init(nil)
	require.NoError(t, schema.ParseBytes([]byte(`
		lossyOnly: string @index(term) .
		lossyAndExact: string @index(term, exact) .
		sortable: int @index(int) .
		vecOnly: float32vector @index(hnsw(metric:"euclidean")) .
	`), 1))
}

// A pushed-down first is applied by posting.(*List).Uids to each posting list the worker reads, so
// it is only sound for a function that reads one list per result. A function that reads a list per
// token and then intersects them needs the whole of each list, because the first (or last) n of an
// intersection is not the intersection of the first (or last) n. calculatePaginationParams is what
// keeps those functions off the pushdown, and this pins that.
func TestPaginationPushdownExcludesIntersectingFunctions(t *testing.T) {
	initPaginationSchema(t)
	// Reading a list per token and intersecting: a pushdown would drop matches.
	for _, fn := range []string{"regexp", "alloftext", "allofterms", "match", "ngram"} {
		t.Run(fn, func(t *testing.T) {
			for _, count := range []int{5, -5} {
				sg := &SubGraph{
					SrcFunc: &Function{Name: fn},
					Params:  params{Count: count, Offset: 7},
				}
				first, offset := calculatePaginationParams(sg, x.RootNamespace)
				require.Equal(t, int32(math.MaxInt32), first, "count: %d", count)
				require.Zero(t, offset, "count: %d", count)
			}
		})
	}

	// Reading one list, or reading several and merging them: a pushdown is sound, because the
	// first (or last) n of a union is contained in the union of each list's first (or last) n,
	// and the query layer takes its own slice of the merged result afterwards.
	for _, fn := range []string{"eq", "anyofterms", "uid_in"} {
		t.Run(fn, func(t *testing.T) {
			sg := &SubGraph{
				Attr:    "lossyAndExact",
				SrcFunc: &Function{Name: fn},
				Params:  params{Count: 5, Offset: 7},
			}
			first, offset := calculatePaginationParams(sg, x.RootNamespace)
			require.Equal(t, int32(5), first)
			require.Equal(t, int32(7), offset)
		})
	}
}

// allof (a custom index) intersects per-token lists like allofterms does, geo functions re-check
// the real geometry after the posting list was read, and eq or an inequality over a lossy-only
// index re-checks the real values. A pushed-down first would cut uids those later passes need.
func TestPaginationPushdownExcludesPostFilteredFunctions(t *testing.T) {
	initPaginationSchema(t)
	attr := func(name string) string { return name }

	pushed := func(fn, pred string) bool {
		sg := &SubGraph{
			Attr:    pred,
			SrcFunc: &Function{Name: fn},
			Params:  params{Count: 5, Offset: 7},
		}
		first, _ := calculatePaginationParams(sg, x.RootNamespace)
		return first == 5
	}

	for _, fn := range []string{"allof", "anyof", "near", "within", "contains", "intersects"} {
		// anyof merges rather than intersects, so it keeps the pushdown.
		require.Equal(t, fn == "anyof", pushed(fn, attr("lossyAndExact")), fn)
	}
	for _, fn := range []string{"eq", "ge", "gt", "le", "lt", "between"} {
		require.False(t, pushed(fn, attr("lossyOnly")), fn)
	}
	require.True(t, pushed("eq", attr("lossyAndExact")))
	require.True(t, pushed("ge", attr("sortable")))
}

// A filter, an order, or no count at all all mean the whole list is needed, whatever the function.
func TestPaginationPushdownNeedsTheWholeListSometimes(t *testing.T) {
	unbounded := func(sg *SubGraph) {
		first, offset := calculatePaginationParams(sg, x.RootNamespace)
		require.Equal(t, int32(math.MaxInt32), first)
		require.Zero(t, offset)
	}

	unbounded(&SubGraph{Params: params{Count: 0, Offset: 7}})
	unbounded(&SubGraph{Params: params{Count: 5, Offset: 7}, Filters: []*SubGraph{{}}})
	unbounded(&SubGraph{Params: params{Count: 5, Offset: 7, Order: []*pb.Order{{Attr: "name"}}}})
}

// A predicate with only a vector index is indexed but has no scalar tokenizer, so picking one for eq
// must report an error (which keeps the pushdown off) instead of indexing an empty slice.
func TestPaginationPushdownVectorOnlyIndexDoesNotPanic(t *testing.T) {
	initPaginationSchema(t)
	sg := &SubGraph{
		Attr:    "vecOnly",
		SrcFunc: &Function{Name: "eq"},
		Params:  params{Count: 5, Offset: 7},
	}
	require.NotPanics(t, func() {
		first, _ := calculatePaginationParams(sg, x.RootNamespace)
		require.Equal(t, int32(math.MaxInt32), first)
	})
}
