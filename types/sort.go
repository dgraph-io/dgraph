/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package types

import (
	"fmt"
	"sort"
	"time"

	"github.com/dgraph-io/dgraph/protos/taskp"
	"github.com/dgraph-io/dgraph/x"
)

type sortBase struct {
	values []Val
	ul     *taskp.List
}

// Len returns size of vector.
func (s sortBase) Len() int { return len(s.values) }

// Swap swaps two elements.
func (s sortBase) Swap(i, j int) {
	s.values[i], s.values[j] = s.values[j], s.values[i]
	data := s.ul.Uids
	data[i], data[j] = data[j], data[i]
}

type byValue struct{ sortBase }

// Less compares two elements
func (s byValue) Less(i, j int) bool {
	if s.values[i].Tid != s.values[j].Tid {
		return false
	}
	switch s.values[i].Tid {
	case DateTimeID:
		return s.values[i].Value.(time.Time).Before(s.values[j].Value.(time.Time))
	case DateID:
		return s.values[i].Value.(time.Time).Before(s.values[j].Value.(time.Time))
	case IntID:
		return (s.values[i].Value.(int64)) < (s.values[j].Value.(int64))
	case FloatID:
		return (s.values[i].Value.(float64)) < (s.values[j].Value.(float64))
	case StringID, DefaultID:
		return (s.values[i].Value.(string)) < (s.values[j].Value.(string))
	}
	x.Fatalf("Unexpected scalar: %v", s.values[i].Tid)
	return false
}

// Sort sorts the given array in-place.
func Sort(v []Val, ul *taskp.List, desc bool) error {
	typ := v[0].Tid
	switch typ {
	case DateTimeID, DateID, IntID, FloatID, StringID, DefaultID:
		// Don't do anything, we can sort values of this type.
	default:
		return fmt.Errorf("Value of type: %s isn't sortable.", typ.Name())
	}

	var toBeSorted sort.Interface
	b := sortBase{v, ul}
	toBeSorted = byValue{b}
	if desc {
		toBeSorted = sort.Reverse(toBeSorted)
	}
	sort.Sort(toBeSorted)
	return nil
}

// Less returns true if a is strictly less than b.
func Less(a, b Val) (bool, error) {
	if a.Tid != b.Tid {
		return false, x.Errorf("Arguments of different type can not be compared.")
	}
	switch a.Tid {
	case DateID:
		return a.Value.(time.Time).Before(b.Value.(time.Time)), nil
	case DateTimeID:
		return a.Value.(time.Time).Before(b.Value.(time.Time)), nil
	case IntID:
		return (a.Value.(int64)) < (b.Value.(int64)), nil
	case FloatID:
		return (a.Value.(float64)) < (b.Value.(float64)), nil
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
	case DateID:
		return a.Value.(time.Time) == (b.Value.(time.Time)), nil
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
