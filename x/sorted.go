/*
 * Copyright 2016 DGraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package x

// IntersectSorted returns intersection of lists of sorted UIDs / uint64s.
func IntersectSorted(a [][]uint64) []uint64 {
	if len(a) == 0 {
		return []uint64{}
	}
	if len(a) == 1 {
		return a[0]
	}
	// Scan through the smallest list. Denote as A.
	// For each x in A,
	//   For each other list B,
	//     Keep popping elements until we get a y >= x.
	//     If y > x, we want to skip x. Break out of loop for B.
	//   If we reach here, append our output by x.
	// We also remove all duplicates.
	var minLen, minLenIndex int
	for i, l := range a {
		if len(l) < minLen {
			minLen = len(l)
			minLenIndex = i
		}
	}
	// minLen array is a[minLenIndex].
	ptr := make([]int, len(a))
	var output []uint64
	for k, x := range a[minLenIndex] {
		if k > 0 && x == a[minLenIndex][k-1] {
			continue // Avoid duplicates.
		}
		var skipX bool
		for i, l := range a {
			if i == minLenIndex {
				continue
			}
			j := ptr[i]
			for ; j < len(l) && l[j] < x; j++ {
			}
			ptr[i] = j
			if l[j] > x {
				skipX = true
				break
			}
			// Otherwise, l[j] = x and we continue checking other lists.
		}
		if !skipX {
			output = append(output, x)
		}
	}
	return output
}
