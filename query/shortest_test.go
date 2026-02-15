package query

import (
	"container/heap"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPriorityQueueTrimToMax_RemovesHighestCost(t *testing.T) {
	var pq priorityQueue
	heap.Init(&pq)

	costs := []float64{1, 50, 2, 60, 3, 70, 4}
	for _, c := range costs {
		heap.Push(&pq, &queueItem{cost: c})
	}

	// Trim to keep N-1 elements.
	(&pq).TrimToMax(int64(len(costs) - 1))
	require.Equal(t, len(costs)-1, pq.Len())

	// Pop all remaining costs and ensure the maximum was removed.
	seen := make(map[float64]bool, len(costs))
	for pq.Len() > 0 {
		seen[heap.Pop(&pq).(*queueItem).cost] = true
	}

	require.False(t, seen[70], "expected highest cost to be removed")
	for _, c := range []float64{1, 2, 3, 4, 50, 60} {
		require.True(t, seen[c], "expected cost to remain: %v", c)
	}
}
