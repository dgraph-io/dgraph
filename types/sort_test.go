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
	"math"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/task"
)

func toString(t *testing.T, values []Val, vID TypeID) []string {
	out := make([]string, len(values))
	for i, v := range values {
		b := ValueForType(StringID)
		require.NoError(t, Marshal(v, &b))
		out[i] = b.Value.(string)
	}
	return out
}

func getInput(t *testing.T, tid TypeID, in []string) []Val {
	list := make([]Val, len(in))
	for i, s := range in {
		v := ValueForType(tid)
		va := Val{StringID, []byte(s)}
		require.NoError(t, Convert(va, &v))
		list[i] = v
	}
	return list
}

func getUIDList(n int) *task.List {
	data := make([]uint64, 0, n)
	for i := 1; i <= n; i++ {
		data = append(data, uint64(i*100))
	}
	return &task.List{Uids: data}
}

func TestSortStrings(t *testing.T) {
	list := getInput(t, StringID, []string{"bb", "aaa", "aa", "bab"})
	ul := getUIDList(4)
	require.NoError(t, Sort(StringID, list, ul, false))
	require.EqualValues(t, []uint64{300, 200, 400, 100}, ul.Uids)
	require.EqualValues(t, []string{"aa", "aaa", "bab", "bb"},
		toString(t, list, StringID))
}

func TestSortInts(t *testing.T) {
	list := getInput(t, Int32ID, []string{"22", "111", "11", "212"})
	ul := getUIDList(4)
	require.NoError(t, Sort(Int32ID, list, ul, false))
	require.EqualValues(t, []uint64{300, 100, 200, 400}, ul.Uids)
	require.EqualValues(t, []string{"11", "22", "111", "212"},
		toString(t, list, Int32ID))
}

func TestSortFloats(t *testing.T) {
	list := getInput(t, FloatID, []string{"22.2", "11.2", "11.5", "2.12"})
	ul := getUIDList(4)
	require.NoError(t, Sort(FloatID, list, ul, false))
	require.EqualValues(t, []uint64{400, 200, 300, 100}, ul.Uids)
	require.EqualValues(t,
		[]string{"2.12E+00", "1.12E+01", "1.15E+01", "2.22E+01"},
		toString(t, list, FloatID))
}

func TestSortFloatsDesc(t *testing.T) {
	list := getInput(t, FloatID, []string{"22.2", "11.2", "11.5", "2.12"})
	ul := getUIDList(4)
	require.NoError(t, Sort(FloatID, list, ul, true))
	require.EqualValues(t, []uint64{100, 300, 200, 400}, ul.Uids)
	require.EqualValues(t,
		[]string{"2.22E+01", "1.15E+01", "1.12E+01", "2.12E+00"},
		toString(t, list, FloatID))
}

func TestSortDates(t *testing.T) {
	in := []string{"2022-01-01", "2022-02-03", "2021-01-05", "2021-01-07"}
	list := getInput(t, DateID, in)
	ul := getUIDList(4)
	require.NoError(t, Sort(DateID, list, ul, false))
	require.EqualValues(t, []uint64{300, 400, 100, 200}, ul.Uids)
	require.EqualValues(t,
		[]string{"2021-01-05", "2021-01-07", "2022-01-01", "2022-02-03"},
		toString(t, list, DateID))
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
	require.NoError(t, Sort(DateTimeID, list, ul, false))
	require.EqualValues(t, []uint64{400, 200, 300, 100}, ul.Uids)
	require.EqualValues(t,
		[]string{"2006-01-02T15:04:01Z", "2006-01-02T15:04:05Z",
			"2006-01-02T15:04:06Z", "2016-01-02T15:04:05Z"},
		toString(t, list, DateTimeID))
}

type encL struct {
	ints   []int32
	tokens []string
}

type byEnc struct{ encL }

func (o byEnc) Less(i, j int) bool {
	return o.ints[i] < o.ints[j]
}

func (o byEnc) Len() int { return len(o.ints) }

func (o byEnc) Swap(i, j int) {
	o.ints[i], o.ints[j] = o.ints[j], o.ints[i]
	o.tokens[i], o.tokens[j] = o.tokens[j], o.tokens[i]
}

func TestIntEncoding(t *testing.T) {
	a := int32(2<<24 + 10)
	b := int32(-2<<24 - 1)
	c := int32(math.MaxInt32)
	d := int32(math.MinInt32)
	enc := encL{}
	arr := []int32{a, b, c, d, 1, 2, 3, 4, -1, -2, -3, 0, 234, 10000, 123, -1543}
	enc.ints = arr
	for _, it := range arr {
		encoded, err := encodeInt(int32(it))
		require.NoError(t, err)
		enc.tokens = append(enc.tokens, encoded[0])
	}
	sort.Sort(byEnc{enc})
	for i := 1; i < len(enc.tokens); i++ {
		// The corresponding string tokens should be greater.
		require.True(t, enc.tokens[i-1] < enc.tokens[i])
	}
}
