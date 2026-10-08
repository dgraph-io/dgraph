/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package query

import (
	"container/heap"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// --- priorityQueue basic heap operations ---

func TestPriorityQueue_PushPop(t *testing.T) {
	var pq priorityQueue
	heap.Init(&pq)

	heap.Push(&pq, &queueItem{uid: 1, cost: 3.0})
	heap.Push(&pq, &queueItem{uid: 2, cost: 1.0})
	heap.Push(&pq, &queueItem{uid: 3, cost: 2.0})

	require.Equal(t, 3, pq.Len())

	// Min-heap: should pop in ascending cost order.
	item := heap.Pop(&pq).(*queueItem)
	require.Equal(t, uint64(2), item.uid)
	require.Equal(t, 1.0, item.cost)
	require.Equal(t, -1, item.index) // index set to -1 after pop

	item = heap.Pop(&pq).(*queueItem)
	require.Equal(t, uint64(3), item.uid)
	require.Equal(t, 2.0, item.cost)

	item = heap.Pop(&pq).(*queueItem)
	require.Equal(t, uint64(1), item.uid)
	require.Equal(t, 3.0, item.cost)

	require.Equal(t, 0, pq.Len())
}

func TestPriorityQueue_IndexMaintenance(t *testing.T) {
	var pq priorityQueue
	heap.Init(&pq)

	items := []*queueItem{
		{uid: 1, cost: 5.0},
		{uid: 2, cost: 1.0},
		{uid: 3, cost: 3.0},
	}
	for _, item := range items {
		heap.Push(&pq, item)
	}

	// Each item's index should match its position in the slice.
	for i, item := range pq {
		require.Equal(t, i, item.index, "index mismatch for uid %d", item.uid)
	}
}

func TestPriorityQueue_HeapFix(t *testing.T) {
	var pq priorityQueue
	heap.Init(&pq)

	a := &queueItem{uid: 1, cost: 5.0}
	b := &queueItem{uid: 2, cost: 3.0}
	c := &queueItem{uid: 3, cost: 1.0}
	heap.Push(&pq, a)
	heap.Push(&pq, b)
	heap.Push(&pq, c)

	// c (cost 1.0) is at the root. Update its cost to be the highest.
	c.cost = 10.0
	heap.Fix(&pq, c.index)

	// Now b (cost 3.0) should be the minimum.
	item := heap.Pop(&pq).(*queueItem)
	require.Equal(t, uint64(2), item.uid)
	require.Equal(t, 3.0, item.cost)
}

func TestPriorityQueue_SingleElement(t *testing.T) {
	var pq priorityQueue
	heap.Init(&pq)

	heap.Push(&pq, &queueItem{uid: 42, cost: 7.0})
	require.Equal(t, 1, pq.Len())

	item := heap.Pop(&pq).(*queueItem)
	require.Equal(t, uint64(42), item.uid)
	require.Equal(t, 0, pq.Len())
}

func TestPriorityQueue_DuplicateCosts(t *testing.T) {
	var pq priorityQueue
	heap.Init(&pq)

	heap.Push(&pq, &queueItem{uid: 1, cost: 2.0})
	heap.Push(&pq, &queueItem{uid: 2, cost: 2.0})
	heap.Push(&pq, &queueItem{uid: 3, cost: 2.0})

	// All have the same cost; should still pop 3 items without error.
	for i := 0; i < 3; i++ {
		item := heap.Pop(&pq).(*queueItem)
		require.Equal(t, 2.0, item.cost)
	}
	require.Equal(t, 0, pq.Len())
}

// --- removeMax tests ---

func TestRemoveMax_BasicCase(t *testing.T) {
	var pq priorityQueue
	heap.Init(&pq)

	heap.Push(&pq, &queueItem{uid: 1, cost: 1.0})
	heap.Push(&pq, &queueItem{uid: 2, cost: 5.0})
	heap.Push(&pq, &queueItem{uid: 3, cost: 3.0})

	pq.removeMax()

	require.Equal(t, 2, pq.Len())

	// The removed item should be uid=2 (cost 5.0).
	// Remaining items should be uid=1 and uid=3.
	remaining := map[uint64]float64{}
	for pq.Len() > 0 {
		item := heap.Pop(&pq).(*queueItem)
		remaining[item.uid] = item.cost
	}
	require.Equal(t, map[uint64]float64{1: 1.0, 3: 3.0}, remaining)
}

func TestRemoveMax_EmptyQueue(t *testing.T) {
	var pq priorityQueue
	heap.Init(&pq)

	// Should not panic on empty queue.
	require.NotPanics(t, func() {
		pq.removeMax()
	})
	require.Equal(t, 0, pq.Len())
}

func TestRemoveMax_SingleElement(t *testing.T) {
	var pq priorityQueue
	heap.Init(&pq)

	heap.Push(&pq, &queueItem{uid: 1, cost: 42.0})
	pq.removeMax()

	require.Equal(t, 0, pq.Len())
}

func TestRemoveMax_DuplicateMaxCost(t *testing.T) {
	var pq priorityQueue
	heap.Init(&pq)

	heap.Push(&pq, &queueItem{uid: 1, cost: 1.0})
	heap.Push(&pq, &queueItem{uid: 2, cost: 5.0})
	heap.Push(&pq, &queueItem{uid: 3, cost: 5.0})
	heap.Push(&pq, &queueItem{uid: 4, cost: 3.0})

	pq.removeMax()

	// One of the cost-5.0 items should be removed.
	require.Equal(t, 3, pq.Len())

	// The remaining items should have costs summing to 1+5+3=9 or 1+3+5=9.
	totalCost := 0.0
	for pq.Len() > 0 {
		item := heap.Pop(&pq).(*queueItem)
		totalCost += item.cost
	}
	require.Equal(t, 9.0, totalCost)
}

func TestRemoveMax_PreservesHeapInvariant(t *testing.T) {
	var pq priorityQueue
	heap.Init(&pq)

	costs := []float64{10, 4, 7, 1, 8, 3, 9, 2, 6, 5}
	for i, c := range costs {
		heap.Push(&pq, &queueItem{uid: uint64(i), cost: c})
	}

	// Remove max (cost=10), then verify we can still pop in order.
	pq.removeMax()
	require.Equal(t, 9, pq.Len())

	prev := -1.0
	for pq.Len() > 0 {
		item := heap.Pop(&pq).(*queueItem)
		require.GreaterOrEqual(t, item.cost, prev, "heap invariant violated")
		prev = item.cost
	}
}

func TestRemoveMax_RepeatedCalls(t *testing.T) {
	var pq priorityQueue
	heap.Init(&pq)

	for i := 0; i < 10; i++ {
		heap.Push(&pq, &queueItem{uid: uint64(i), cost: float64(i)})
	}

	// Repeatedly remove max — should remove 9, 8, 7, 6, 5 in order.
	for expectedMax := 9.0; expectedMax >= 5.0; expectedMax-- {
		// Find current max before removal.
		maxCost := 0.0
		for _, item := range pq {
			if item.cost > maxCost {
				maxCost = item.cost
			}
		}
		require.Equal(t, expectedMax, maxCost)
		pq.removeMax()
	}

	require.Equal(t, 5, pq.Len())

	// Remaining should be costs 0-4 in heap order.
	prev := -1.0
	for pq.Len() > 0 {
		item := heap.Pop(&pq).(*queueItem)
		require.GreaterOrEqual(t, item.cost, prev)
		prev = item.cost
	}
}

func TestRemoveMax_IndexConsistencyAfterRemoval(t *testing.T) {
	var pq priorityQueue
	heap.Init(&pq)

	heap.Push(&pq, &queueItem{uid: 1, cost: 1.0})
	heap.Push(&pq, &queueItem{uid: 2, cost: 5.0})
	heap.Push(&pq, &queueItem{uid: 3, cost: 3.0})
	heap.Push(&pq, &queueItem{uid: 4, cost: 2.0})

	pq.removeMax()

	// After removeMax, every item's index should match its slice position.
	for i, item := range pq {
		require.Equal(t, i, item.index,
			"index mismatch after removeMax for uid %d", item.uid)
	}
}

func TestRemoveMax_WithNegativeCosts(t *testing.T) {
	var pq priorityQueue
	heap.Init(&pq)

	heap.Push(&pq, &queueItem{uid: 1, cost: -5.0})
	heap.Push(&pq, &queueItem{uid: 2, cost: -1.0})
	heap.Push(&pq, &queueItem{uid: 3, cost: -3.0})

	pq.removeMax()

	// Max is -1.0, should be removed. Remaining: -5.0 and -3.0.
	require.Equal(t, 2, pq.Len())
	item := heap.Pop(&pq).(*queueItem)
	require.Equal(t, -5.0, item.cost)
	item = heap.Pop(&pq).(*queueItem)
	require.Equal(t, -3.0, item.cost)
}

// --- Frontier eviction integration tests ---

// simulateFrontierEviction tests the push-then-evict pattern used in
// shortestPath and runKShortestPaths.
func TestFrontierEviction_PushThenEvict(t *testing.T) {
	var pq priorityQueue
	heap.Init(&pq)
	maxSize := int64(3)

	// Push 3 items — no eviction needed.
	for i := 1; i <= 3; i++ {
		heap.Push(&pq, &queueItem{uid: uint64(i), cost: float64(i)})
	}
	require.Equal(t, 3, pq.Len())

	// Push a 4th item (cost=0.5) — should evict the highest cost (3.0).
	heap.Push(&pq, &queueItem{uid: 4, cost: 0.5})
	if int64(pq.Len()) > maxSize {
		pq.removeMax()
	}

	require.Equal(t, 3, pq.Len())

	// Verify cost=3.0 was evicted, not the new low-cost item.
	remaining := map[uint64]float64{}
	for pq.Len() > 0 {
		item := heap.Pop(&pq).(*queueItem)
		remaining[item.uid] = item.cost
	}
	require.Contains(t, remaining, uint64(4), "new low-cost node should be retained")
	require.NotContains(t, remaining, uint64(3), "highest-cost node should be evicted")
}

func TestFrontierEviction_HighCostNewNodeEvicted(t *testing.T) {
	var pq priorityQueue
	heap.Init(&pq)
	maxSize := int64(3)

	// Push 3 items with costs 1, 2, 3.
	for i := 1; i <= 3; i++ {
		heap.Push(&pq, &queueItem{uid: uint64(i), cost: float64(i)})
	}

	// Push a new item with cost=10 — it should be evicted (it's the new max).
	heap.Push(&pq, &queueItem{uid: 99, cost: 10.0})
	if int64(pq.Len()) > maxSize {
		pq.removeMax()
	}

	require.Equal(t, 3, pq.Len())

	// uid=99 should have been evicted since it has the highest cost.
	for _, item := range pq {
		require.NotEqual(t, uint64(99), item.uid,
			"high-cost new node should be evicted, not retained")
	}
}

func TestFrontierEviction_ExactBoundary(t *testing.T) {
	var pq priorityQueue
	heap.Init(&pq)
	maxSize := int64(3)

	// Push exactly maxSize items — no eviction.
	for i := 1; i <= 3; i++ {
		heap.Push(&pq, &queueItem{uid: uint64(i), cost: float64(i)})
	}
	if int64(pq.Len()) > maxSize {
		pq.removeMax()
	}
	require.Equal(t, 3, pq.Len(), "should not evict when at exactly maxSize")
}

func TestFrontierEviction_SizeNeverExceedsMax(t *testing.T) {
	var pq priorityQueue
	heap.Init(&pq)
	maxSize := int64(5)

	// Push 20 items, each time checking frontier size.
	for i := 0; i < 20; i++ {
		heap.Push(&pq, &queueItem{uid: uint64(i), cost: float64(i % 7)})
		if int64(pq.Len()) > maxSize {
			pq.removeMax()
		}
		require.LessOrEqual(t, int64(pq.Len()), maxSize,
			"frontier size should never exceed max after eviction")
	}
}

func TestFrontierEviction_PreservesLowestCosts(t *testing.T) {
	var pq priorityQueue
	heap.Init(&pq)
	maxSize := int64(5)

	// Push 10 items with costs 0-9. After each push, enforce frontier limit.
	for i := 0; i < 10; i++ {
		heap.Push(&pq, &queueItem{uid: uint64(i), cost: float64(i)})
		if int64(pq.Len()) > maxSize {
			pq.removeMax()
		}
	}

	require.Equal(t, 5, pq.Len())

	// The 5 lowest-cost items (0-4) should be retained.
	for _, item := range pq {
		require.Less(t, item.cost, 5.0,
			"only the 5 lowest-cost items should be retained, but found cost=%v", item.cost)
	}
}

func TestFrontierEviction_ZeroMaxDisabled(t *testing.T) {
	var pq priorityQueue
	heap.Init(&pq)
	maxSize := int64(0) // disabled

	for i := 0; i < 10; i++ {
		heap.Push(&pq, &queueItem{uid: uint64(i), cost: float64(i)})
		if maxSize > 0 && int64(pq.Len()) > maxSize {
			pq.removeMax()
		}
	}

	// No eviction when maxSize=0.
	require.Equal(t, 10, pq.Len())
}

// --- route.indexOf tests ---

func TestRoute_IndexOf_Found(t *testing.T) {
	path := []pathInfo{
		{uid: 10},
		{uid: 20},
		{uid: 30},
	}
	r := route{route: &path}
	require.Equal(t, 0, r.indexOf(10))
	require.Equal(t, 1, r.indexOf(20))
	require.Equal(t, 2, r.indexOf(30))
}

func TestRoute_IndexOf_NotFound(t *testing.T) {
	path := []pathInfo{
		{uid: 10},
		{uid: 20},
	}
	r := route{route: &path}
	require.Equal(t, -1, r.indexOf(99))
}

// --- Less / Swap coverage ---

func TestPriorityQueue_Less(t *testing.T) {
	pq := priorityQueue{
		&queueItem{uid: 1, cost: 1.0, index: 0},
		&queueItem{uid: 2, cost: 2.0, index: 1},
	}
	require.True(t, pq.Less(0, 1))
	require.False(t, pq.Less(1, 0))
	require.False(t, pq.Less(0, 0))
}

func TestPriorityQueue_Swap(t *testing.T) {
	pq := priorityQueue{
		&queueItem{uid: 1, cost: 1.0, index: 0},
		&queueItem{uid: 2, cost: 2.0, index: 1},
	}
	pq.Swap(0, 1)

	require.Equal(t, uint64(2), pq[0].uid)
	require.Equal(t, 0, pq[0].index)
	require.Equal(t, uint64(1), pq[1].uid)
	require.Equal(t, 1, pq[1].index)
}

// --- Edge case: MaxFloat64 cost ---

func TestRemoveMax_MaxFloat64(t *testing.T) {
	var pq priorityQueue
	heap.Init(&pq)

	heap.Push(&pq, &queueItem{uid: 1, cost: 1.0})
	heap.Push(&pq, &queueItem{uid: 2, cost: math.MaxFloat64})
	heap.Push(&pq, &queueItem{uid: 3, cost: 100.0})

	pq.removeMax()

	require.Equal(t, 2, pq.Len())
	item := heap.Pop(&pq).(*queueItem)
	require.Equal(t, 1.0, item.cost)
	item = heap.Pop(&pq).(*queueItem)
	require.Equal(t, 100.0, item.cost)
}
