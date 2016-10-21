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

func TestSortStrings(t *testing.T) {
	in := []string{"bb", "aaa", "aa", "bab"}

	list := make([]Value, len(in))
	for i, s := range in {
		v := ValueForType(StringID)
		v.UnmarshalText([]byte(s))
		list[i] = v
	}

	idx, err := stringType.Sort(list)
	require.NoError(t, err)
	require.EqualValues(t, []uint32{2, 1, 3, 0}, idx)
	require.EqualValues(t, []string{"aa", "aaa", "bab", "bb"},
		toString(t, list))
}

func TestSortByteArrays(t *testing.T) {
	in := []string{"bb", "aaa", "aa", "bab"}

	list := make([]Value, len(in))
	for i, s := range in {
		v := ValueForType(BytesID)
		v.UnmarshalText([]byte(s))
		list[i] = v
	}

	idx, err := byteArrayType.Sort(list)
	require.NoError(t, err)
	require.EqualValues(t, []uint32{2, 1, 3, 0}, idx)
	require.EqualValues(t, []string{"aa", "aaa", "bab", "bb"},
		toString(t, list))
}

func TestSortInts(t *testing.T) {
	in := []string{"22", "111", "11", "212"}

	list := make([]Value, len(in))
	for i, s := range in {
		v := ValueForType(Int32ID)
		v.UnmarshalText([]byte(s))
		list[i] = v
	}

	idx, err := int32Type.Sort(list)
	require.NoError(t, err)
	require.EqualValues(t, []uint32{2, 0, 1, 3}, idx)
	require.EqualValues(t, []string{"11", "22", "111", "212"},
		toString(t, list))
}

func TestSortFloats(t *testing.T) {
	in := []string{"22.2", "11.2", "11.5", "2.12"}

	list := make([]Value, len(in))
	for i, s := range in {
		v := ValueForType(FloatID)
		v.UnmarshalText([]byte(s))
		list[i] = v
	}

	idx, err := floatType.Sort(list)
	require.NoError(t, err)
	require.EqualValues(t, []uint32{3, 1, 2, 0}, idx)
	require.EqualValues(t,
		[]string{"2.12E+00", "1.12E+01", "1.15E+01", "2.22E+01"},
		toString(t, list))
}

func TestSortDates(t *testing.T) {
	in := []string{"2022-01-01", "2022-02-03", "2021-01-05", "2021-01-07"}

	list := make([]Value, len(in))
	for i, s := range in {
		v := ValueForType(DateID)
		v.UnmarshalText([]byte(s))
		list[i] = v
	}

	idx, err := dateType.Sort(list)
	require.NoError(t, err)
	require.EqualValues(t, []uint32{2, 3, 0, 1}, idx)
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

	list := make([]Value, len(in))
	for i, s := range in {
		v := ValueForType(DateTimeID)
		v.UnmarshalText([]byte(s))
		list[i] = v
	}

	idx, err := dateTimeType.Sort(list)
	require.NoError(t, err)
	require.EqualValues(t, []uint32{3, 1, 2, 0}, idx)
	require.EqualValues(t,
		[]string{"2006-01-02T15:04:01Z", "2006-01-02T15:04:05Z",
			"2006-01-02T15:04:06Z", "2016-01-02T15:04:05Z"},
		toString(t, list))
}
