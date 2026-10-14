/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package hnsw

import (
	"context"
	"testing"

	"github.com/dgraph-io/dgraph/v25/tok/index"
)

// TestMergeResultsFewerCandidatesThanMaxResults pins the bounds handling in
// MergeResults: when the merged candidate list is shorter than maxResults, the
// loop must stop at len(result) instead of indexing past it.
func TestMergeResultsFewerCandidatesThanMaxResults(t *testing.T) {
	emptyTsDbs()
	ph := flatPhs[0]

	query := []float64{0.0, 0.0, 0.0}
	vecs := map[uint64][]float64{
		1: {0.9, 0.9, 0.9},
		2: {0.1, 0.1, 0.1},
		3: {0.5, 0.5, 0.5},
	}
	for uid, vec := range vecs {
		key := DataKey(ph.pred, uid)
		for i := range tsDbs {
			tsDbs[i].inMemTestDb[string(key[:])] = floatArrayAsBytes(vec)
		}
	}
	tc := NewTxnCache(&inMemTxn{startTs: 12, commitTs: 40}, 12)

	// maxResults far larger than the candidate list: must not panic and must
	// return every candidate ordered by distance to the query.
	uids, err := ph.MergeResults(context.TODO(), tc, []uint64{1, 2, 3}, query, 10, index.AcceptAll[float64])
	if err != nil {
		t.Fatalf("MergeResults returned error: %v", err)
	}
	expected := []uint64{2, 3, 1}
	if len(uids) != len(expected) {
		t.Fatalf("expected %d uids, got %d (%v)", len(expected), len(uids), uids)
	}
	for i := range expected {
		if uids[i] != expected[i] {
			t.Fatalf("expected order %v, got %v", expected, uids)
		}
	}

	// maxResults smaller than the candidate list: truncates to the closest.
	uids, err = ph.MergeResults(context.TODO(), tc, []uint64{1, 2, 3}, query, 2, index.AcceptAll[float64])
	if err != nil {
		t.Fatalf("MergeResults returned error: %v", err)
	}
	expected = []uint64{2, 3}
	if len(uids) != len(expected) {
		t.Fatalf("expected %d uids, got %d (%v)", len(expected), len(uids), uids)
	}
	for i := range expected {
		if uids[i] != expected[i] {
			t.Fatalf("expected order %v, got %v", expected, uids)
		}
	}

	// Empty candidate list: no results, no panic.
	uids, err = ph.MergeResults(context.TODO(), tc, nil, query, 3, index.AcceptAll[float64])
	if err != nil {
		t.Fatalf("MergeResults returned error: %v", err)
	}
	if len(uids) != 0 {
		t.Fatalf("expected no uids for empty candidate list, got %v", uids)
	}
}
