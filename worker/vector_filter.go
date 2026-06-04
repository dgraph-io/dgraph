/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import (
	"sort"

	"github.com/dgraph-io/dgraph/v25/tok/index"
)

// uidMembershipFilter builds a SearchFilter that accepts only uids present in the
// given allow-set. It powers pre-filtered ANN: the filter is applied during the HNSW
// traversal (not after it), so a scoped similar_to search explores enough of the
// graph to return k in-scope neighbors instead of post-filtering a fixed k and
// returning fewer. The query and candidate vectors are irrelevant to membership, so
// only the uid is examined.
//
// Membership uses a sorted copy of the uids plus binary search rather than a hash
// set: the allow-set can be large (whole-tenant scopes), and a compact 8-bytes/uid
// slice avoids the per-query allocation and GC pressure of a map with millions of
// entries while keeping lookups at O(log n). An empty allow-set rejects everything.
func uidMembershipFilter(uids []uint64) index.SearchFilter[float32] {
	// Copy before sorting: the input may alias a shared, sorted-ascending var list
	// that other goroutines read concurrently; we must not mutate it.
	sorted := make([]uint64, len(uids))
	copy(sorted, uids)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	return func(_, _ []float32, uid uint64) bool {
		i := sort.Search(len(sorted), func(i int) bool { return sorted[i] >= uid })
		return i < len(sorted) && sorted[i] == uid
	}
}
