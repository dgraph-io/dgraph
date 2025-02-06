/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package types

// Below functions are taken from go's sort library zsortinterface.go
// https://go.dev/src/sort/zsortinterface.go
func insertionSort(data byValue, a, b int) {
	for i := a + 1; i < b; i++ {
		for j := i; j > a && data.Less(j, j-1); j-- {
			data.Swap(j, j-1)
		}
	}
}

func order2(data byValue, a, b int) (int, int) {
	if data.Less(b, a) {
		return b, a
	}
	return a, b
}

func median(data byValue, a, b, c int) int {
	a, b = order2(data, a, b)
	b, _ = order2(data, b, c)
	_, b = order2(data, a, b)
	return b
}

func medianAdjacent(data byValue, a int) int {
	return median(data, a-1, a, a+1)
}

// [shortestNinther,∞): uses the Tukey ninther method.
func choosePivot(data byValue, a, b int) (pivot int) {
	const (
		shortestNinther = 50
		maxSwaps        = 4 * 3
	)

	l := b - a

	var (
		i = a + l/4*1
		j = a + l/4*2
		k = a + l/4*3
	)

	if l >= 8 {
		if l >= shortestNinther {
			// Tukey ninther method, the idea came from Rust's implementation.
			i = medianAdjacent(data, i)
			j = medianAdjacent(data, j)
			k = medianAdjacent(data, k)
		}
		// Find the median among i, j, k and stores it into j.
		j = median(data, i, j, k)
	}

	return j
}

func partition(data byValue, a, b, pivot int) int {
	partitionIndex := a
	data.Swap(pivot, b)
	for i := a; i < b; i++ {
		if data.Less(i, b) {
			data.Swap(i, partitionIndex)
			partitionIndex++
		}
	}
	data.Swap(partitionIndex, b)
	return partitionIndex
}

func quickSelect(data byValue, low, high, k int) {
	var pivotIndex int

	for {
		if low >= high {
			return
		} else if high-low <= 8 {
			insertionSort(data, low, high+1)
			return
		}

		pivotIndex = choosePivot(data, low, high)
		pivotIndex = partition(data, low, high, pivotIndex)

		if k < pivotIndex {
			high = pivotIndex - 1
		} else if k > pivotIndex {
			low = pivotIndex + 1
		} else {
			return
		}
	}
}
