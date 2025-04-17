/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package hnsw

import (
	c "github.com/hypermodeinc/dgraph/v25/tok/constraints"
	"github.com/hypermodeinc/dgraph/v25/tok/index"

	"fmt"
)

type searchLayerResult[T c.Float] struct {
	// neighbors represents the candidates with the best scores so far.
	neighbors []minPersistentHeapElement[T]
	// visited represents elements seen (so we don't try to re-visit).
	visited map[uint64]minPersistentHeapElement[T]
	path    []uint64
	metrics map[string]uint64
	level   int
	// filtered represents num elements of meighbors that don't
	// belong in final return set since they should be filtered out.
	// When we encounter a node that we consider a "best" node, but where
	// it should be filtered out, we allow it to enter the "neighbors"
	// attribute as an element. However, we then allow neighbors to
	// grow by this extra "filtered" amount. Theoretically, it could be
	// pushed out, but that will be okay! At the end, we grab all
	// non-filtered elements up to the limit of what is expected.
	filtered int
}

func newLayerResult[T c.Float](level int) *searchLayerResult[T] {
	return &searchLayerResult[T]{
		neighbors: []minPersistentHeapElement[T]{},
		visited:   make(map[uint64]minPersistentHeapElement[T]),
		path:      []uint64{},
		metrics:   make(map[string]uint64),
		level:     level,
	}
}

func (slr *searchLayerResult[T]) setFirstPathNode(n minPersistentHeapElement[T]) {
	slr.neighbors = []minPersistentHeapElement[T]{n}
	slr.visited = make(map[uint64]minPersistentHeapElement[T])
	slr.visited[n.index] = n
	slr.path = []uint64{n.index}
}

func (slr *searchLayerResult[T]) addPathNode(
	n minPersistentHeapElement[T],
	simType SimilarityType[T],
	maxResults int) {
	slr.neighbors = simType.insortHeap(slr.neighbors, n)
	if n.filteredOut {
		slr.filtered++
	}
	effectiveMaxLen := maxResults + slr.filtered
	if len(slr.neighbors) > effectiveMaxLen {
		slr.neighbors = slr.neighbors[:effectiveMaxLen]
	}

	if slr.neighbors[0].index == n.index {
		slr.path = append(slr.path, slr.neighbors[0].index)
	}
}

func (slr *searchLayerResult[T]) numNeighbors() int {
	return len(slr.neighbors) - slr.filtered
}

func (slr *searchLayerResult[T]) markFirstDistanceComputation() {
	slr.metrics[distanceComputations] = 1
}

func (slr *searchLayerResult[T]) incrementDistanceComputations() {
	slr.metrics[distanceComputations]++
}

// slr.lastNeighborScore() returns the "score" (based on similarity type)
// of the last neighbor being tracked. The score is reflected as a value
// of the minPersistentHeapElement.
// If slr is empty, this will panic.
func (slr *searchLayerResult[T]) lastNeighborScore() T {
	return slr.neighbors[len(slr.neighbors)-1].value
}

// slr.bestNeighbor() returns the heap element with the "best" score.
// panics if there is no such element.
func (slr *searchLayerResult[T]) bestNeighbor() minPersistentHeapElement[T] {
	return slr.neighbors[0]
}

func (slr *searchLayerResult[T]) indexVisited(n uint64) bool {
	_, ok := slr.visited[n]
	return ok
}

func (slr *searchLayerResult[T]) addToVisited(n minPersistentHeapElement[T]) {
	slr.visited[n.index] = n
}

func (slr *searchLayerResult[T]) updateFinalMetrics(r *index.SearchPathResult) {
	visitName := ConcatStrings(visitedVectorsLevel, fmt.Sprint(slr.level))
	r.Metrics[visitName] += uint64(len(slr.visited))
	for k, v := range slr.metrics {
		r.Metrics[k] += v
	}
}

func (slr *searchLayerResult[T]) updateFinalPath(r *index.SearchPathResult) {
	r.Path = append(r.Path, slr.path...)
}

func (slr *searchLayerResult[T]) addFinalNeighbors(r *index.SearchPathResult) {
	for _, n := range slr.neighbors {
		if !n.filteredOut {
			r.Neighbors = append(r.Neighbors, n.index)
		}
	}
}
