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
	"testing"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/stretchr/testify/require"
)

func toString(t *testing.T, values [][]Val, vID TypeID) []string {
	out := make([]string, len(values))
	for i, v := range values {
		b := ValueForType(StringID)
		require.NoError(t, Marshal(v[0], &b))
		out[i] = b.Value.(string)
	}
	return out
}

func getInput(t *testing.T, tid TypeID, in []string) [][]Val {
	list := make([][]Val, len(in))
	for i, s := range in {
		va := Val{StringID, []byte(s)}
		v, err := Convert(va, tid)
		require.NoError(t, err)
		list[i] = []Val{v}
	}
	return list
}

func getUIDList(n int) *protos.List {
	data := make([]uint64, 0, n)
	for i := 1; i <= n; i++ {
		data = append(data, uint64(i*100))
	}
	return &protos.List{data}
}

func TestSortStrings(t *testing.T) {
	list := getInput(t, StringID, []string{"bb", "aaa", "aa", "bab"})
	ul := getUIDList(4)
	require.NoError(t, Sort(list, ul, []bool{false}))
	require.EqualValues(t, []uint64{300, 200, 400, 100}, ul.Uids)
	require.EqualValues(t, []string{"aa", "aaa", "bab", "bb"},
		toString(t, list, StringID))
}

func TestSortInts(t *testing.T) {
	list := getInput(t, IntID, []string{"22", "111", "11", "212"})
	ul := getUIDList(4)
	require.NoError(t, Sort(list, ul, []bool{false}))
	require.EqualValues(t, []uint64{300, 100, 200, 400}, ul.Uids)
	require.EqualValues(t, []string{"11", "22", "111", "212"},
		toString(t, list, IntID))
}

func TestSortFloats(t *testing.T) {
	list := getInput(t, FloatID, []string{"22.2", "11.2", "11.5", "2.12"})
	ul := getUIDList(4)
	require.NoError(t, Sort(list, ul, []bool{false}))
	require.EqualValues(t, []uint64{400, 200, 300, 100}, ul.Uids)
	require.EqualValues(t,
		[]string{"2.12E+00", "1.12E+01", "1.15E+01", "2.22E+01"},
		toString(t, list, FloatID))
}

func TestSortFloatsDesc(t *testing.T) {
	list := getInput(t, FloatID, []string{"22.2", "11.2", "11.5", "2.12"})
	ul := getUIDList(4)
	require.NoError(t, Sort(list, ul, []bool{true}))
	require.EqualValues(t, []uint64{100, 300, 200, 400}, ul.Uids)
	require.EqualValues(t,
		[]string{"2.22E+01", "1.15E+01", "1.12E+01", "2.12E+00"},
		toString(t, list, FloatID))
}

func TestSortDateTimes(t *testing.T) {
	in := []string{
		"2016-01-02T15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:06",
		"2006-01-02T15:04:01",
	}
	list := getInput(t, DateTimeID, in)
	ul := getUIDList(4)
	require.NoError(t, Sort(list, ul, []bool{false}))
	require.EqualValues(t, []uint64{400, 200, 300, 100}, ul.Uids)
	require.EqualValues(t,
		[]string{"2006-01-02T15:04:01Z", "2006-01-02T15:04:05Z",
			"2006-01-02T15:04:06Z", "2016-01-02T15:04:05Z"},
		toString(t, list, DateTimeID))
}
