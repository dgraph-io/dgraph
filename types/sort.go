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

	"github.com/dgraph-io/dgraph/x"
)

func intRange(n int) []uint32 {
	out := make([]uint32, n)
	for i := 0; i < n; i++ {
		out[i] = uint32(i)
	}
	return out
}

type dateSorter struct {
	values []Value
	idx    []uint32
}

func (s *dateSorter) Len() int { return len(s.values) }
func (s *dateSorter) Swap(i, j int) {
	s.values[i], s.values[j] = s.values[j], s.values[i]
	s.idx[i], s.idx[j] = s.idx[j], s.idx[i]
}
func (s *dateSorter) Less(i, j int) bool {
	return s.values[i].(*Date).Time.Before(s.values[j].(*Date).Time)
}

type dateTimeSorter struct {
	values []Value
	idx    []uint32
}

func (s *dateTimeSorter) Len() int { return len(s.values) }
func (s *dateTimeSorter) Swap(i, j int) {
	s.values[i], s.values[j] = s.values[j], s.values[i]
	s.idx[i], s.idx[j] = s.idx[j], s.idx[i]
}
func (s *dateTimeSorter) Less(i, j int) bool {
	return s.values[i].(*Time).Time.Before(s.values[j].(*Time).Time)
}

type int32Sorter struct {
	values []Value
	idx    []uint32
}

func (s *int32Sorter) Len() int { return len(s.values) }
func (s *int32Sorter) Swap(i, j int) {
	s.values[i], s.values[j] = s.values[j], s.values[i]
	s.idx[i], s.idx[j] = s.idx[j], s.idx[i]
}
func (s *int32Sorter) Less(i, j int) bool {
	return *(s.values[i].(*Int32)) < *(s.values[j].(*Int32))
}

type floatSorter struct {
	values []Value
	idx    []uint32
}

func (s *floatSorter) Len() int { return len(s.values) }
func (s *floatSorter) Swap(i, j int) {
	s.values[i], s.values[j] = s.values[j], s.values[i]
	s.idx[i], s.idx[j] = s.idx[j], s.idx[i]
}
func (s *floatSorter) Less(i, j int) bool {
	return *(s.values[i].(*Float)) < *(s.values[j].(*Float))
}

type stringSorter struct {
	values []Value
	idx    []uint32
}

func (s *stringSorter) Len() int { return len(s.values) }
func (s *stringSorter) Swap(i, j int) {
	s.values[i], s.values[j] = s.values[j], s.values[i]
	s.idx[i], s.idx[j] = s.idx[j], s.idx[i]
}
func (s *stringSorter) Less(i, j int) bool {
	return *(s.values[i].(*String)) < *(s.values[j].(*String))
}

type bytesSorter struct {
	values []Value
	idx    []uint32
}

func (s *bytesSorter) Len() int { return len(s.values) }
func (s *bytesSorter) Swap(i, j int) {
	s.values[i], s.values[j] = s.values[j], s.values[i]
	s.idx[i], s.idx[j] = s.idx[j], s.idx[i]
}
func (s *bytesSorter) Less(i, j int) bool {
	return bytes.Compare(*(s.values[i].(*Bytes)), *(s.values[j].(*Bytes))) < 0
}

// Sort sorts the given array in-place.
func (s Scalar) Sort(v []Value) ([]uint32, error) {
	switch s.ID() {
	case DateID:
		sorter := &dateSorter{v, intRange(len(v))}
		sort.Sort(sorter)
		return sorter.idx, nil
	case DateTimeID:
		sorter := &dateTimeSorter{v, intRange(len(v))}
		sort.Sort(sorter)
		return sorter.idx, nil
	case Int32ID:
		sorter := &int32Sorter{v, intRange(len(v))}
		sort.Sort(sorter)
		return sorter.idx, nil
	case FloatID:
		sorter := &floatSorter{v, intRange(len(v))}
		sort.Sort(sorter)
		return sorter.idx, nil
	case StringID:
		sorter := &stringSorter{v, intRange(len(v))}
		sort.Sort(sorter)
		return sorter.idx, nil
	case BytesID:
		sorter := &bytesSorter{v, intRange(len(v))}
		sort.Sort(sorter)
		return sorter.idx, nil
	}
	return nil, x.Errorf("Type does not support sorting: %s", s)
}
