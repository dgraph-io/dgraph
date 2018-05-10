/*
 * Copyright 2016-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package types

import (
	"testing"

	"github.com/dgraph-io/dgraph/protos/intern"
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

func getUIDList(n int) *intern.List {
	data := make([]uint64, 0, n)
	for i := 1; i <= n; i++ {
		data = append(data, uint64(i*100))
	}
	return &intern.List{data}
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

func TestSortIntAndFloat(t *testing.T) {
	list := [][]Val{
		[]Val{Val{Tid: IntID, Value: int64(55)}},
		[]Val{Val{Tid: FloatID, Value: 21.5}},
		[]Val{Val{Tid: IntID, Value: int64(100)}},
	}
	ul := getUIDList(3)
	require.NoError(t, Sort(list, ul, []bool{false}))
	require.EqualValues(t, []uint64{200, 100, 300}, ul.Uids)
	require.EqualValues(t,
		[]string{"2.15E+01", "55", "100"},
		toString(t, list, DateTimeID))

}

func findIndex(t *testing.T, uids []uint64, uid uint64) int {
	for i := range uids {
		if uids[i] == uid {
			return i
		}
	}
	t.Errorf("Could not find index")
	return -1
}

func TestSortMismatchedTypes(t *testing.T) {
	list := [][]Val{
		[]Val{Val{Tid: StringID, Value: "cat"}},
		[]Val{Val{Tid: IntID, Value: int64(55)}},
		[]Val{Val{Tid: BoolID, Value: true}},
		[]Val{Val{Tid: FloatID, Value: 21.5}},
		[]Val{Val{Tid: StringID, Value: "aardvark"}},
		[]Val{Val{Tid: StringID, Value: "buffalo"}},
		[]Val{Val{Tid: FloatID, Value: 33.33}},
	}
	ul := getUIDList(7)
	require.NoError(t, Sort(list, ul, []bool{false}))

	// Don't care about relative ordering between types. However, like types
	// should be sorted with each other.
	catIdx := findIndex(t, ul.Uids, 100)
	aarIdx := findIndex(t, ul.Uids, 500)
	bufIdx := findIndex(t, ul.Uids, 600)
	require.True(t, aarIdx < bufIdx)
	require.True(t, bufIdx < catIdx)

	idx55 := findIndex(t, ul.Uids, 200)
	idx21 := findIndex(t, ul.Uids, 400)
	idx33 := findIndex(t, ul.Uids, 700)
	require.True(t, idx21 < idx33)
	require.True(t, idx33 < idx55)
}
