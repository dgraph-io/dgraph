/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import (
	"slices"

	"github.com/dgraph-io/dgraph/v25/tok/index"
)

// uidMembershipFilter builds a SearchFilter that accepts only uids present in the
// given allow-set. It powers pre-filtered ANN: the filter is applied during the HNSW
// traversal (not after it), so a scoped similar_to search explores past out-of-scope
// nodes rather than post-filtering a fixed k (though a very selective scope may still
// return fewer than k, since the traversal budget is fixed). The query and candidate
// vectors are irrelevant to membership, so only the uid is examined.
//
// Membership uses a sorted copy of the uids plus binary search rather than a hash
// set: the allow-set can be large (whole-tenant scopes), and a compact 8-bytes/uid
// slice avoids the per-query allocation and GC pressure of a map with millions of
// entries while keeping lookups at O(log n). An empty allow-set rejects everything.
func uidMembershipFilter(uids []uint64) index.SearchFilter[float32] {
	// Copy before sorting: the input may alias a shared var list that other goroutines
	// read concurrently; we must not mutate it. uid variables reach the worker already
	// sorted ascending (algo.MergeSorted), so the common case skips the O(n log n) sort
	// — a meaningful saving for whole-tenant scopes (millions of uids per query). We
	// still sort defensively when the invariant does not hold, since the binary-search
	// lookup below requires ascending order for correctness.
	sorted := make([]uint64, len(uids))
	copy(sorted, uids)
	if !slices.IsSorted(sorted) {
		slices.Sort(sorted)
	}

	return func(_, _ []float32, uid uint64) bool {
		_, found := slices.BinarySearch(sorted, uid)
		return found
	}
}
