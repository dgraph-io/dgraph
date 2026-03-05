/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import (
	"container/heap"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
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
