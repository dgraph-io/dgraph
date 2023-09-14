/*
 * Copyright 2016-2023 Dgraph Labs, Inc. and Contributors
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
 */

package types

// Below functions are taken from go's sort library zsortinterface.go
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
	b, c = order2(data, b, c)
	a, b = order2(data, a, b)
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

func QuickSelect(data byValue, low, high, k int) {
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
