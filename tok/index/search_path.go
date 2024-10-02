/*
 * Copyright 2016-2024 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * Co-authored by: jairad26@gmail.com, sunil@hypermode.com, bill@hypdermode.com
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
