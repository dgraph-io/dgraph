/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package hnsw

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSimilarityScoreOrientation verifies that every metric surfaces a
// higher-is-better similarity score, which native hybrid-search fusion relies on.
func TestSimilarityScoreOrientation(t *testing.T) {
	cosine := GetSimType[float32](Cosine, 32)
	dot := GetSimType[float32](DotProd, 32)
	euclid := GetSimType[float32](Euclidean, 32)

	// Cosine / dot product: returned as-is (already higher-is-better).
	require.InDelta(t, 0.9, cosine.similarityScore(0.9), 1e-6)
	require.InDelta(t, -0.2, cosine.similarityScore(-0.2), 1e-6)
	require.InDelta(t, 12.5, dot.similarityScore(12.5), 1e-6)

	// Euclidean: squared distance mapped to 1/(1+d), so a smaller distance yields a
	// larger score (closer = better).
	near := euclid.similarityScore(0.0) // distance 0 -> perfect match
	mid := euclid.similarityScore(1.0)
	far := euclid.similarityScore(9.0)
	require.InDelta(t, 1.0, near, 1e-6)
	require.InDelta(t, 0.5, mid, 1e-6)
	require.InDelta(t, 0.1, far, 1e-6)
	require.Greater(t, near, mid)
	require.Greater(t, mid, far)
}
