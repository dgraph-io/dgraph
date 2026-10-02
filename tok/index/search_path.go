/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package index

// SearchPathResult is the return type for the optional
// SearchWithPath function for a VectorIndex
// (by way of extending OptionalIndexSupport).
type SearchPathResult struct {
	// The collection of nearest neighbors in sorted order after filtering
	// out neighbors that fail any Filter criteria.
	Neighbors []uint64
	// Distances holds the higher-is-better similarity score for each entry in
	// Neighbors (positionally aligned). It is populated by scored searches and may
	// be empty for callers that only need the neighbor uids.
	Distances []float64
	// The path from the start of search to the closest neighbor vector.
	Path []uint64
	// A collection of captured named counters that occurred for the
	// particular search.
	Metrics map[string]uint64
}

// NewSearchPathResult provides an initialised (empty) *SearchPathResult.
// The attributes will be non‑nil but empty.
func NewSearchPathResult() *SearchPathResult {
	return &SearchPathResult{
		Neighbors: []uint64{},
		Distances: []float64{},
		Path:      []uint64{},
		Metrics:   make(map[string]uint64),
	}
}
