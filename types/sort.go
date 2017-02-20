/*
 * Copyright 2016 Dgraph Labs, Inc.
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

package types

import (
	"sort"
	"time"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/x"
)

type sortBase struct {
	values []Val
	ul     *task.List
}

// Len returns size of vector.
func (s sortBase) Len() int { return len(s.values) }

// Swap swaps two elements.
func (s sortBase) Swap(i, j int) {
	s.values[i], s.values[j] = s.values[j], s.values[i]
	data := s.ul
	algo.Swap(data, i, j)
}

type byValue struct{ sortBase }

// Less compares two elements
func (s byValue) Less(i, j int) bool {
	switch s.values[i].Tid {
	case DateTimeID:
		return s.values[i].Value.(time.Time).Before(s.values[j].Value.(time.Time))
	case DateID:
		return s.values[i].Value.(time.Time).Before(s.values[j].Value.(time.Time))
	case Int32ID:
		return (s.values[i].Value.(int32)) < (s.values[j].Value.(int32))
	case FloatID:
		return (s.values[i].Value.(float64)) < (s.values[j].Value.(float64))
	case StringID:
		return (s.values[i].Value.(string)) < (s.values[j].Value.(string))
	}
	x.Fatalf("Unexpected scalar: %v", s.values[i].Tid)
	return false
}

// Sort sorts the given array in-place.
func Sort(sID TypeID, v []Val, ul *task.List, desc bool) error {
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
	case Int32ID:
		return (a.Value.(int32)) < (b.Value.(int32)), nil
	case FloatID:
		return (a.Value.(float64)) < (b.Value.(float64)), nil
	case StringID:
		return (a.Value.(string)) < (b.Value.(string)), nil
	}
	return false, x.Errorf("Compare not supported for type: %v", a.Tid)
}
