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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/task"
)

func toString(t *testing.T, values []Value) []string {
	out := make([]string, len(values))
	for i, v := range values {
		b, err := v.MarshalText()
		require.NoError(t, err)
		out[i] = string(b)
	}
	return out
}

func getInput(tid TypeID, in []string) []Value {
	list := make([]Value, len(in))
	for i, s := range in {
		v := ValueForType(tid)
		v.UnmarshalText([]byte(s))
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
	list := getInput(StringID, []string{"bb", "aaa", "aa", "bab"})
	ul := getUIDList(4)
	require.NoError(t, stringType.Sort(list, ul, false))
	require.EqualValues(t, []uint64{300, 200, 400, 100}, ul.Uids)
	require.EqualValues(t, []string{"aa", "aaa", "bab", "bb"},
		toString(t, list))
}

func TestSortByteArrays(t *testing.T) {
	list := getInput(BytesID, []string{"bb", "aaa", "aa", "bab"})
	ul := getUIDList(4)
	require.NoError(t, byteArrayType.Sort(list, ul, false))
	require.EqualValues(t, []uint64{300, 200, 400, 100}, ul.Uids)
	require.EqualValues(t, []string{"aa", "aaa", "bab", "bb"},
		toString(t, list))
}

func TestSortInts(t *testing.T) {
	list := getInput(Int32ID, []string{"22", "111", "11", "212"})
	ul := getUIDList(4)
	require.NoError(t, int32Type.Sort(list, ul, false))
	require.EqualValues(t, []uint64{300, 100, 200, 400}, ul.Uids)
	require.EqualValues(t, []string{"11", "22", "111", "212"},
		toString(t, list))
}

func TestSortFloats(t *testing.T) {
	list := getInput(FloatID, []string{"22.2", "11.2", "11.5", "2.12"})
	ul := getUIDList(4)
	require.NoError(t, floatType.Sort(list, ul, false))
	require.EqualValues(t, []uint64{400, 200, 300, 100}, ul.Uids)
	require.EqualValues(t,
		[]string{"2.12E+00", "1.12E+01", "1.15E+01", "2.22E+01"},
		toString(t, list))
}

func TestSortFloatsDesc(t *testing.T) {
	list := getInput(FloatID, []string{"22.2", "11.2", "11.5", "2.12"})
	ul := getUIDList(4)
	require.NoError(t, floatType.Sort(list, ul, true))
	require.EqualValues(t, []uint64{100, 300, 200, 400}, ul.Uids)
	require.EqualValues(t,
		[]string{"2.22E+01", "1.15E+01", "1.12E+01", "2.12E+00"},
		toString(t, list))
}

func TestSortDates(t *testing.T) {
	in := []string{"2022-01-01", "2022-02-03", "2021-01-05", "2021-01-07"}
	list := getInput(DateID, in)
	ul := getUIDList(4)
	require.NoError(t, dateType.Sort(list, ul, false))
	require.EqualValues(t, []uint64{300, 400, 100, 200}, ul.Uids)
	require.EqualValues(t,
		[]string{"2021-01-05", "2021-01-07", "2022-01-01", "2022-02-03"},
		toString(t, list))
}

func TestSortDateTimes(t *testing.T) {
	in := []string{
		"2016-01-02T15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:06",
		"2006-01-02T15:04:01",
	}
	list := getInput(DateTimeID, in)
	ul := getUIDList(4)
	require.NoError(t, dateTimeType.Sort(list, ul, false))
	require.EqualValues(t, []uint64{400, 200, 300, 100}, ul.Uids)
	require.EqualValues(t,
		[]string{"2006-01-02T15:04:01Z", "2006-01-02T15:04:05Z",
			"2006-01-02T15:04:06Z", "2016-01-02T15:04:05Z"},
		toString(t, list))
}
