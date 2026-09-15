/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package types

import (
	"testing"
)

func TestShardedMapBasic(t *testing.T) {
	m := NewShardedMap()
	v := Val{Tid: IntID, Value: int64(42)}
	m.Set(7, v)

	got, ok := m.Get(7)
	if !ok || got.Value != int64(42) {
		t.Fatalf("Get(7) = %v, %v; want 42, true", got, ok)
	}
	if _, ok := m.Get(8); ok {
		t.Fatalf("Get(8) = ok; want missing")
	}
	if m.Len() != 1 {
		t.Fatalf("Len = %d; want 1", m.Len())
	}
}

func TestShardedMapEmpty(t *testing.T) {
	var m *ShardedMap
	if !m.IsEmpty() {
		t.Fatalf("nil IsEmpty should be true")
	}
	if _, ok := m.Get(1); ok {
		t.Fatalf("nil Get returned ok=true")
	}
	if m.Len() != 0 {
		t.Fatalf("nil Len = %d; want 0", m.Len())
	}

	m = NewShardedMap()
	if !m.IsEmpty() {
		t.Fatalf("Fresh ShardedMap IsEmpty should be true")
	}
}

func TestShardedMapIterate(t *testing.T) {
	m := NewShardedMap()
	for i := uint64(0); i < 100; i++ {
		m.Set(i, Val{Tid: IntID, Value: int64(i)})
	}
	count := 0
	err := m.Iterate(func(k uint64, v Val) error {
		count++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count != 100 {
		t.Fatalf("iter count = %d; want 100", count)
	}
}

func TestShardedMapMerge(t *testing.T) {
	a := NewShardedMap()
	b := NewShardedMap()
	for i := uint64(0); i < 10; i++ {
		a.Set(i, Val{Tid: IntID, Value: int64(i)})
		b.Set(i+5, Val{Tid: IntID, Value: int64(i + 100)})
	}
	a.Merge(b, func(x, y Val) Val { return y })
	if a.Len() != 15 {
		t.Fatalf("merged Len = %d; want 15", a.Len())
	}
	got, _ := a.Get(7)
	if got.Value != int64(102) {
		t.Fatalf("Merged Get(7).Value = %v; want 102", got)
	}
}

// BenchmarkShardedMapNew measures the cost of constructing a fresh ShardedMap
// with no inserts. This is the case where lazy shard init pays off the most:
// no shard maps should be allocated.
func BenchmarkShardedMapNew(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m := NewShardedMap()
		_ = m
	}
}

// BenchmarkShardedMapSetGet measures a typical small-data pattern: a few
// inserts, a few reads, a Len/IsEmpty check.
func BenchmarkShardedMapSetGet(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m := NewShardedMap()
		for j := uint64(0); j < 8; j++ {
			m.Set(j, Val{Tid: IntID, Value: int64(j)})
		}
		for j := uint64(0); j < 8; j++ {
			_, _ = m.Get(j)
		}
	}
}

// BenchmarkShardedMapFull writes to all 30 shards — measures the worst case
// where lazy init provides no savings.
func BenchmarkShardedMapFull(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m := NewShardedMap()
		for j := uint64(0); j < 30; j++ {
			m.Set(j, Val{Tid: IntID, Value: int64(j)})
		}
	}
}
