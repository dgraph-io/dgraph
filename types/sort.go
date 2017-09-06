/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package types

import (
	"fmt"
	"sort"
	"time"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
)

type sortBase struct {
	values [][]Val // Each uid could have multiple values which we need to sort it by.
	desc   []bool  // Sort orders for different values.
	ul     *protos.List
	o      []*protos.Facets
}

// Len returns size of vector.
func (s sortBase) Len() int { return len(s.values) }

// Swap swaps two elements.
func (s sortBase) Swap(i, j int) {
	s.values[i], s.values[j] = s.values[j], s.values[i]
	data := s.ul.Uids
	data[i], data[j] = data[j], data[i]
	if s.o != nil {
		s.o[i], s.o[j] = s.o[j], s.o[i]
	}
}

type byValue struct{ sortBase }

// Less compares two elements
func (s byValue) Less(i, j int) bool {
	if len(s.values[i]) == 0 || len(s.values[j]) == 0 {
		return false
	}

	for idx, _ := range s.values[i] {
		// We have to look at next value to decide.
		// TOOD(pawan) - Handle error.
		if eq, _ := Equal(s.values[i][idx], s.values[j][idx]); eq {
			continue
		}

		// Its either less or greater.
		less, _ := Less(s.values[i][idx], s.values[j][idx])
		if s.desc[idx] {
			return !less
		}
		return less
	}
	return false
}

// Sort sorts the given array in-place.
func SortWithFacet(v [][]Val, ul *protos.List, l []*protos.Facets, desc []bool) error {
	if len(v) == 0 || len(v[0]) == 0 {
		return nil
	}
	typ := v[0][0].Tid
	switch typ {
	case DateTimeID, IntID, FloatID, StringID, DefaultID:
		// Don't do anything, we can sort values of this type.
	default:
		return fmt.Errorf("Value of type: %s isn't sortable.", typ.Name())
	}
	var toBeSorted sort.Interface
	b := sortBase{v, desc, ul, l}
	toBeSorted = byValue{b}
	sort.Sort(toBeSorted)
	return nil
}

// Sort sorts the given array in-place.
func Sort(v [][]Val, ul *protos.List, desc []bool) error {
	return SortWithFacet(v, ul, nil, desc)
}

// Less returns true if a is strictly less than b.
func Less(a, b Val) (bool, error) {
	if a.Tid != b.Tid {
		return false, x.Errorf("Arguments of different type can not be compared.")
	}
	switch a.Tid {
	case DateTimeID:
		return a.Value.(time.Time).Before(b.Value.(time.Time)), nil
	case IntID:
		return (a.Value.(int64)) < (b.Value.(int64)), nil
	case FloatID:
		return (a.Value.(float64)) < (b.Value.(float64)), nil
	case UidID:
		return (a.Value.(uint64) < b.Value.(uint64)), nil
	case StringID, DefaultID:
		return (a.Value.(string)) < (b.Value.(string)), nil

	}
	return false, x.Errorf("Compare not supported for type: %v", a.Tid)
}

// Equal returns true if a is equal to b.
func Equal(a, b Val) (bool, error) {
	if a.Tid != b.Tid {
		return false, x.Errorf("Arguments of different type can not be compared.")
	}
	switch a.Tid {
	case DateTimeID:
		return a.Value.(time.Time) == (b.Value.(time.Time)), nil
	case IntID:
		return (a.Value.(int64)) == (b.Value.(int64)), nil
	case FloatID:
		return (a.Value.(float64)) == (b.Value.(float64)), nil
	case StringID, DefaultID:
		return (a.Value.(string)) == (b.Value.(string)), nil
	case BoolID:
		return a.Value.(bool) == (b.Value.(bool)), nil
	}
	return false, x.Errorf("Equal not supported for type: %v", a.Tid)
}
