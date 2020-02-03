/*
 * Copyright 2017-2020 Dgraph Labs, Inc. and Contributors
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
package bulk

import (
	"testing"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/stretchr/testify/require"
)

type SimpleIterator struct {
	keys    [][]byte
	current *pb.MapEntry
	idx     int
}

func (itr *SimpleIterator) Next() {
	if itr.idx < len(itr.keys) {
		itr.current = &pb.MapEntry{Key: itr.keys[itr.idx]}
		itr.idx++
		return
	}
	itr.current = nil
}

func (itr *SimpleIterator) Current() *pb.MapEntry {
	return itr.current
}

func getSimpleIterator(input []string) *SimpleIterator {
	out := [][]byte{}
	for _, in := range input {
		out = append(out, []byte(in))
	}
	return &SimpleIterator{
		keys: out,
	}
}

func getAll(itr Iterator) []string {
	out := []string{}
	for {
		current := itr.Current()
		if current == nil {
			break
		}
		out = append(out, string(current.Key))
		itr.Next()
	}
	return out
}

func TestMergeIterator(t *testing.T) {
	it := getSimpleIterator([]string{"a", "d", "h"})
	it.Next()
	it2 := getSimpleIterator([]string{"b", "e", "g", "k"})
	it2.Next()
	it3 := getSimpleIterator([]string{"c", "f", "i", "j"})
	it3.Next()
	mi := NewMergeIterator([]Iterator{it, it2, it3})
	out := getAll(mi)
	expected := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"}
	require.Equal(t, out, expected)
}
