/*
 * Copyright 2016-2018 Dgraph Labs, Inc. and Contributors
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

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

type sortBase struct {
	values [][]Val // Each uid could have multiple values which we need to sort it by.
	desc   []bool  // Sort orders for different values.
	ul     *[]uint64
	o      []*pb.Facets
	cl     *collate.Collator // Compares Unicode strings according to the given collation order.
}

// Len returns size of vector.
// skipcq: CRT-P0003
func (s sortBase) Len() int { return len(s.values) }

// Swap swaps two elements.
// skipcq: CRT-P0003
func (s sortBase) Swap(i, j int) {
	s.values[i], s.values[j] = s.values[j], s.values[i]
	(*s.ul)[i], (*s.ul)[j] = (*s.ul)[j], (*s.ul)[i]
	if s.o != nil {
		s.o[i], s.o[j] = s.o[j], s.o[i]
	}
}

type byValue struct{ sortBase }

// Less compares two elements
// skipcq: CRT-P0003
func (s byValue) Less(i, j int) bool {
	first, second := s.values[i], s.values[j]
	if len(first) == 0 || len(second) == 0 {
		return false
	}
	for vidx := range first {
		// Null value is considered greatest hence comes at first place while doing descending sort
		// and at last place while doing ascending sort.
		if first[vidx].Value == nil {
			return s.desc[vidx]
		}

		if second[vidx].Value == nil {
			return !s.desc[vidx]
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
