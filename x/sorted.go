// Utilities with sorted uint64s / UIDs.
package x

//	"fmt"
//	"log"

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
