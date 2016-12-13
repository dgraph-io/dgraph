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
	"bytes"
	"sort"

	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/x"
)

type sortBase struct {
	values []Value
	ul     *task.List
}

// Len returns size of vector.
func (s sortBase) Len() int { return len(s.values) }

// Swap swaps two elements.
func (s sortBase) Swap(i, j int) {
	s.values[i], s.values[j] = s.values[j], s.values[i]
	data := s.ul.Uids
	data[i], data[j] = data[j], data[i]
}

type byDate struct{ sortBase }

func (s byDate) Less(i, j int) bool {
	return s.values[i].(*Date).Time.Before(s.values[j].(*Date).Time)
}

type byDateTime struct{ sortBase }

func (s byDateTime) Less(i, j int) bool {
	return s.values[i].(*Time).Time.Before(s.values[j].(*Time).Time)
}

type byInt32 struct{ sortBase }

func (s byInt32) Less(i, j int) bool {
	return *(s.values[i].(*Int32)) < *(s.values[j].(*Int32))
}

type byFloat struct{ sortBase }

func (s byFloat) Less(i, j int) bool {
	return *(s.values[i].(*Float)) < *(s.values[j].(*Float))
}

type byString struct{ sortBase }

func (s byString) Less(i, j int) bool {
	return *(s.values[i].(*String)) < *(s.values[j].(*String))
}

type byByteArray struct{ sortBase }

func (s byByteArray) Less(i, j int) bool {
	return bytes.Compare(*(s.values[i].(*Bytes)), *(s.values[j].(*Bytes))) < 0
}

// Sort sorts the given array in-place.
func (s Scalar) Sort(v []Value, ul *task.List) error {
	b := sortBase{v, ul}
	switch s.ID() {
	case DateID:
		sort.Sort(byDate{b})
		return nil
	case TimeID:
		sort.Sort(byDateTime{b})
		return nil
	case Int32ID:
		sort.Sort(byInt32{b})
		return nil
	case FloatID:
		sort.Sort(byFloat{b})
		return nil
	case StringID:
		sort.Sort(byString{b})
		return nil
	case BytesID:
		sort.Sort(byByteArray{b})
		return nil
	}
	return x.Errorf("Scalar doesn't support sorting %s", s)
}
