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

import (
	"sort"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
)

type sortBase struct {
	values [][]Val // Each uid could have multiple values which we need to sort it by.
	desc   []bool  // Sort orders for different values.
	ul     *[]uint64
	o      []*pb.Facets
	cl     *collate.Collator // Compares Unicode strings according to the given collation order.
}

// Len returns size of vector.
func (s sortBase) Len() int { return len(s.values) }

// Swap swaps two elements.
func (s sortBase) Swap(i, j int) {
	s.values[i], s.values[j] = s.values[j], s.values[i]
	(*s.ul)[i], (*s.ul)[j] = (*s.ul)[j], (*s.ul)[i]
	if s.o != nil {
		s.o[i], s.o[j] = s.o[j], s.o[i]
	}
}

type byValue struct {
	sortBase
}

func (s byValue) IsNil(i int) bool {
	first := s.values[i]
	if len(first) == 0 {
		return true
	}
	if first[0].Value == nil {
		return true
	}
	return false
}

// Less compares two elements
func (s byValue) Less(i, j int) bool {
	first, second := s.values[i], s.values[j]
	if len(first) == 0 || len(second) == 0 {
		return false
	}
	for vidx := range first {
		// Null values are appended at the end of the sort result for both ascending and descending.
		// If both first and second has nil values, then maintain the order by UID.
		if first[vidx].Value == nil && second[vidx].Value == nil {
			return s.desc[vidx]
		}

		if first[vidx].Value == nil {
			return false
		}

		if second[vidx].Value == nil {
			//fmt.Println("second val true", vidx, i, j, first[vidx].Value)
			return true
		}

		// We have to look at next value to decide.
		if eq := equal(first[vidx], second[vidx]); eq {
			continue
		}

		// Its either less or greater.
		less := less(first[vidx], second[vidx], s.cl)
		if s.desc[vidx] {
			return !less
		}
		return less
	}
	return false
}

// IsSortable returns true, if tid is sortable. Otherwise it returns false.
func IsSortable(tid TypeID) bool {
	switch tid {
	case DateTimeID, IntID, FloatID, StringID, DefaultID:
		return true
	default:
		return false
	}
}

type dataHeap struct {
	heapIndices []int
	data        byValue
}

func (h dataHeap) Len() int           { return len(h.heapIndices) }
func (h dataHeap) Less(i, j int) bool { return h.data.Less(h.heapIndices[j], h.heapIndices[i]) }
func (h dataHeap) Swap(i, j int) {
	h.heapIndices[i], h.heapIndices[j] = h.heapIndices[j], h.heapIndices[i]
}

func (h *dataHeap) Push(x interface{}) {
	h.heapIndices = append(h.heapIndices, x.(int))
}

func (h *dataHeap) Pop() interface{} {
	old := h.heapIndices
	n := len(old)
	x := old[n-1]
	h.heapIndices = old[0 : n-1]
	return x
}

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
	data.Swap(a, pivot)
	i, j := a+1, b-1 // i and j are inclusive of the elements remaining to be partitioned

	for i <= j && data.Less(i, a) {
		i++
	}
	for i <= j && !data.Less(j, a) {
		j--
	}
	if i > j {
		data.Swap(j, a)
		return j
	}
	data.Swap(i, j)
	i++
	j--

	for {
		for i <= j && data.Less(i, a) {
			i++
		}
		for i <= j && !data.Less(j, a) {
			j--
		}
		if i > j {
			break
		}
		data.Swap(i, j)
		i++
		j--
	}
	data.Swap(j, a)
	return j
}

func randomizedSelectionFinding(data byValue, low, high, k int) {
	var pivotIndex int

	for {
		if low >= high {
			return
		} else if high-low <= 8 {
			insertionSort(data, low, high+1)
			return
		}

		pivotIndexPre := choosePivot(data, low, high)

		pivotIndex = partition(data, low, high, pivotIndexPre)
		//fmt.Println(low, pivotIndexPre, pivotIndex, high)

		if k < pivotIndex {
			high = pivotIndex - 1
		} else if k > pivotIndex {
			low = pivotIndex + 1
		} else {
			return
		}
	}
}

// SortWithFacet sorts the given array in-place and considers the given facets to calculate
// the proper ordering.
func SortTopN(v [][]Val, ul *[]uint64, desc []bool, lang string, n int) error {
	if len(v) == 0 || len(v[0]) == 0 {
		return nil
	}

	for _, val := range v[0] {
		if !IsSortable(val.Tid) {
			return errors.Errorf("Value of type: %v isn't sortable", val.Tid.Name())
		}
	}

	var cl *collate.Collator
	if lang != "" {
		// Collator is nil if we are unable to parse the language.
		// We default to bytewise comparison in that case.
		if langTag, err := language.Parse(lang); err == nil {
			cl = collate.New(langTag)
		}
	}

	b := sortBase{v, desc, ul, nil, cl}
	toBeSorted := byValue{b}

	nul := 0
	for i := 0; i < len(*ul); i++ {
		if toBeSorted.IsNil(i) {
			continue
		}
		if i != nul {
			toBeSorted.Swap(i, nul)
		}
		nul += 1
	}

	if nul > n {
		b1 := sortBase{v[:nul], desc, ul, nil, cl}
		toBeSorted1 := byValue{b1}
		randomizedSelectionFinding(toBeSorted1, 0, nul-1, n)
	}
	toBeSorted.values = toBeSorted.values[:n]
	sort.Sort(toBeSorted)

	return nil
}

// SortWithFacet sorts the given array in-place and considers the given facets to calculate
// the proper ordering.
func SortWithFacet(v [][]Val, ul *[]uint64, l []*pb.Facets, desc []bool, lang string) error {
	if len(v) == 0 || len(v[0]) == 0 {
		return nil
	}

	for _, val := range v[0] {
		if !IsSortable(val.Tid) {
			return errors.Errorf("Value of type: %s isn't sortable", val.Tid.Name())
		}
	}

	var cl *collate.Collator
	if lang != "" {
		// Collator is nil if we are unable to parse the language.
		// We default to bytewise comparison in that case.
		if langTag, err := language.Parse(lang); err == nil {
			cl = collate.New(langTag)
		}
	}

	b := sortBase{v, desc, ul, l, cl}
	toBeSorted := byValue{b}
	sort.Sort(toBeSorted)
	return nil
}

// Sort sorts the given array in-place.
func Sort(v [][]Val, ul *[]uint64, desc []bool, lang string) error {
	return SortWithFacet(v, ul, nil, desc, lang)
}

// Less returns true if a is strictly less than b.
func Less(a, b Val) (bool, error) {
	if a.Tid != b.Tid {
		return false, errors.Errorf("Arguments of different type can not be compared.")
	}
	typ := a.Tid
	switch typ {
	case DateTimeID, UidID, IntID, FloatID, StringID, DefaultID:
		// Don't do anything, we can sort values of this type.
	default:
		return false, errors.Errorf("Compare not supported for type: %v", a.Tid)
	}
	return less(a, b, nil), nil
}

func less(a, b Val, cl *collate.Collator) bool {
	if a.Tid != b.Tid {
		return mismatchedLess(a, b)
	}
	switch a.Tid {
	case DateTimeID:
		return a.Value.(time.Time).Before(b.Value.(time.Time))
	case IntID:
		return (a.Value.(int64)) < (b.Value.(int64))
	case FloatID:
		return (a.Value.(float64)) < (b.Value.(float64))
	case UidID:
		return (a.Value.(uint64) < b.Value.(uint64))
	case StringID, DefaultID:
		// Use language comparator.
		if cl != nil {
			return cl.CompareString(a.Safe().(string), b.Safe().(string)) < 0
		}
		return (a.Safe().(string)) < (b.Safe().(string))
	}
	return false
}

func mismatchedLess(a, b Val) bool {
	x.AssertTrue(a.Tid != b.Tid)
	if (a.Tid != IntID && a.Tid != FloatID) || (b.Tid != IntID && b.Tid != FloatID) {
		// Non-float/int are sorted arbitrarily by type.
		return a.Tid < b.Tid
	}

	// Floats and ints can be sorted together in a sensible way. The approach
	// here isn't 100% correct, and will be wrong when dealing with ints and
	// floats close to each other and greater in magnitude than 1<<53 (the
	// point at which consecutive floats are more than 1 apart).
	if a.Tid == FloatID {
		return a.Value.(float64) < float64(b.Value.(int64))
	}
	x.AssertTrue(b.Tid == FloatID)
	return float64(a.Value.(int64)) < b.Value.(float64)
}

// Equal returns true if a is equal to b.
func Equal(a, b Val) (bool, error) {
	if a.Tid != b.Tid {
		return false, errors.Errorf("Arguments of different type can not be compared.")
	}
	typ := a.Tid
	switch typ {
	case DateTimeID, IntID, FloatID, StringID, DefaultID, BoolID:
		// Don't do anything, we can sort values of this type.
	default:
		return false, errors.Errorf("Equal not supported for type: %v", a.Tid)
	}
	return equal(a, b), nil
}

func equal(a, b Val) bool {
	if a.Tid != b.Tid {
		return false
	}
	switch a.Tid {
	case DateTimeID:
		aVal, aOk := a.Value.(time.Time)
		bVal, bOk := b.Value.(time.Time)
		return aOk && bOk && aVal.Equal(bVal)
	case IntID:
		aVal, aOk := a.Value.(int64)
		bVal, bOk := b.Value.(int64)
		return aOk && bOk && aVal == bVal
	case FloatID:
		aVal, aOk := a.Value.(float64)
		bVal, bOk := b.Value.(float64)
		return aOk && bOk && aVal == bVal
	case StringID, DefaultID:
		aVal, aOk := a.Value.(string)
		bVal, bOk := b.Value.(string)
		return aOk && bOk && aVal == bVal
	case BoolID:
		aVal, aOk := a.Value.(bool)
		bVal, bOk := b.Value.(bool)
		return aOk && bOk && aVal == bVal
	}
	return false
}
