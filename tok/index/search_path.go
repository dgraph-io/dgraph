/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package index

// SearchPathResult is the return-type for the optional
// SearchWithPath function for a VectorIndex
// (by way of extending OptionalIndexSupport).
type SearchPathResult struct {
	// The collection of nearest-neighbors in sorted order after filtlering
	// out neighbors that fail any Filter criteria.
	Neighbors []uint64
	// The path from the start of search to the closest neighbor vector.
	Path []uint64
	// A collection of captured named counters that occurred for the
	// particular search.
	Metrics map[string]uint64
}

// NewSearchPathResult() provides an initialized (empty) *SearchPathResult.
// The attributes will be non-nil, but empty.
func NewSearchPathResult() *SearchPathResult {
	return &SearchPathResult{
		Neighbors: []uint64{},
		Path:      []uint64{},
		Metrics:   make(map[string]uint64),
	}
}
