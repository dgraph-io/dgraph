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

// TestBM25PerDocStatsFold verifies the reducer-side fold semantics that make bulk
// BM25 stats exact: the mapper emits one (docCount=1, totalTerms=docLen) posting
// PER DOCUMENT on the bucket's stats key, the reducer dedupes postings with the
// same uid (exactly like term postings — so duplicate nquads cannot inflate the
// stats), and the deduped contributions sum into the aggregate bucket posting.
// This test models the fold: dedup by uid, then sum decoded contributions.
func TestBM25PerDocStatsFold(t *testing.T) {
	type docEntry struct {
		uid    uint64
		docLen uint64
	}
	// Simulated post-sort reducer input for one bucket key: uid-ascending with
	// duplicates (the duplicate-nquad case that inflated stats before the fold).
	entries := []docEntry{
		{uid: 1, docLen: 10},
		{uid: 1, docLen: 10}, // duplicate nquad — must count once
		{uid: 33, docLen: 5},
		{uid: 33, docLen: 5}, // duplicate
		{uid: 33, docLen: 5}, // triplicate
		{uid: 65, docLen: 7},
	}

	var docCount, totalTerms uint64
	var lastUid uint64
	for _, e := range entries {
		if e.uid == lastUid {
			continue
		}
		lastUid = e.uid
		// Round-trip through the wire format the mapper emits.
		c, terms := posting.DecodeBM25Stats(posting.EncodeBM25Stats(1, e.docLen))
		docCount += c
		totalTerms += terms
	}

	require.Equal(t, uint64(3), docCount, "duplicates must not inflate docCount")
	require.Equal(t, uint64(22), totalTerms, "duplicates must not inflate totalTerms")
}
