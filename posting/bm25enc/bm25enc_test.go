/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package bm25enc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRoundtrip(t *testing.T) {
	entries := []Entry{
		{UID: 1, Value: 3},
		{UID: 5, Value: 1},
		{UID: 100, Value: 7},
		{UID: 200, Value: 2},
	}
	data := Encode(entries)
	got := Decode(data)
	require.Equal(t, entries, got)
}

func TestRoundtripEmpty(t *testing.T) {
	require.Nil(t, Encode(nil))
	require.Nil(t, Encode([]Entry{}))
	require.Nil(t, Decode(nil))
	require.Nil(t, Decode([]byte{}))
	require.Nil(t, Decode([]byte{0, 0, 0, 0})) // count=0
}

func TestRoundtripSingle(t *testing.T) {
	entries := []Entry{{UID: 42, Value: 10}}
	got := Decode(Encode(entries))
	require.Equal(t, entries, got)
}

func TestRoundtripLargeUIDs(t *testing.T) {
	entries := []Entry{
		{UID: 1<<40 + 1, Value: 1},
		{UID: 1<<40 + 1000, Value: 5},
		{UID: 1<<50 + 999, Value: 99},
	}
	got := Decode(Encode(entries))
	require.Equal(t, entries, got)
}

func TestUpsertNew(t *testing.T) {
	entries := []Entry{{UID: 1, Value: 3}, {UID: 5, Value: 1}}
	entries = Upsert(entries, 3, 7)
	require.Equal(t, []Entry{{UID: 1, Value: 3}, {UID: 3, Value: 7}, {UID: 5, Value: 1}}, entries)
}

func TestUpsertExisting(t *testing.T) {
	entries := []Entry{{UID: 1, Value: 3}, {UID: 5, Value: 1}}
	entries = Upsert(entries, 5, 99)
	require.Equal(t, []Entry{{UID: 1, Value: 3}, {UID: 5, Value: 99}}, entries)
}

func TestUpsertEmpty(t *testing.T) {
	var entries []Entry
	entries = Upsert(entries, 10, 5)
	require.Equal(t, []Entry{{UID: 10, Value: 5}}, entries)
}

func TestRemove(t *testing.T) {
	entries := []Entry{{UID: 1, Value: 3}, {UID: 5, Value: 1}, {UID: 10, Value: 2}}
	entries = Remove(entries, 5)
	require.Equal(t, []Entry{{UID: 1, Value: 3}, {UID: 10, Value: 2}}, entries)
}

func TestRemoveNotFound(t *testing.T) {
	entries := []Entry{{UID: 1, Value: 3}, {UID: 5, Value: 1}}
	entries = Remove(entries, 99)
	require.Equal(t, []Entry{{UID: 1, Value: 3}, {UID: 5, Value: 1}}, entries)
}

func TestSearch(t *testing.T) {
	entries := []Entry{{UID: 1, Value: 3}, {UID: 5, Value: 1}, {UID: 100, Value: 7}}
	v, ok := Search(entries, 5)
	require.True(t, ok)
	require.Equal(t, uint32(1), v)

	_, ok = Search(entries, 50)
	require.False(t, ok)
}

func TestUIDs(t *testing.T) {
	entries := []Entry{{UID: 1, Value: 3}, {UID: 5, Value: 1}, {UID: 100, Value: 7}}
	require.Equal(t, []uint64{1, 5, 100}, UIDs(entries))
}

func TestStatsRoundtrip(t *testing.T) {
	data := EncodeStats(12345, 98765)
	dc, tt := DecodeStats(data)
	require.Equal(t, uint64(12345), dc)
	require.Equal(t, uint64(98765), tt)
}

func TestStatsInvalid(t *testing.T) {
	dc, tt := DecodeStats(nil)
	require.Zero(t, dc)
	require.Zero(t, tt)
	dc, tt = DecodeStats([]byte{1, 2, 3})
	require.Zero(t, dc)
	require.Zero(t, tt)
}

func BenchmarkEncode(b *testing.B) {
	entries := make([]Entry, 10000)
	for i := range entries {
		entries[i] = Entry{UID: uint64(i*3 + 1), Value: uint32(i % 100)}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Encode(entries)
	}
}

func BenchmarkDecode(b *testing.B) {
	entries := make([]Entry, 10000)
	for i := range entries {
		entries[i] = Entry{UID: uint64(i*3 + 1), Value: uint32(i % 100)}
	}
	data := Encode(entries)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Decode(data)
	}
}
