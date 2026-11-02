/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package types

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLockedShardedMapShardCounts guards the indexing trap: getShardIndex used
// to derive its index from the package-level NumShards while the shard slices
// were sized separately. Once the shard count became per-instance, that would
// either over-run s.shards or strand every shard above index 30. Exercise a
// spread of counts — below, at, and well above the old constant — with both
// supported key types.
func TestLockedShardedMapShardCounts(t *testing.T) {
	for _, n := range []int{1, 2, 7, 30, 64, 100, 1024} {
		t.Run(fmt.Sprintf("shards=%d", n), func(t *testing.T) {
			sm := NewLockedShardedMapWithShards[string, int](n)
			// Rounded up to a power of two, never below 1.
			require.Equal(t, len(sm.shards), len(sm.locks))
			require.Equal(t, uint64(len(sm.shards)-1), sm.mask)
			require.Zero(t, len(sm.shards)&(len(sm.shards)-1), "shard count must be a power of two")

			const keys = 2000
			for i := 0; i < keys; i++ {
				sm.Set(fmt.Sprintf("key-%d", i), i)
			}
			require.Equal(t, keys, sm.Len())
			for i := 0; i < keys; i++ {
				got, ok := sm.Get(fmt.Sprintf("key-%d", i))
				require.True(t, ok, "key-%d missing", i)
				require.Equal(t, i, got)
			}

			um := NewLockedShardedMapWithShards[uint64, int](n)
			// Dense monotonic uids are the realistic case and the one a plain
			// low-bit mask would stripe badly.
			for i := 0; i < keys; i++ {
				um.Set(uint64(1_000_000+i), i)
			}
			require.Equal(t, keys, um.Len())
			for i := 0; i < keys; i++ {
				got, ok := um.Get(uint64(1_000_000 + i))
				require.True(t, ok)
				require.Equal(t, i, got)
			}
		})
	}
}

// TestLockedShardedMapUidSpread checks that dense sequential uids — exactly what
// Dgraph's allocator produces — actually spread across shards rather than
// piling onto a few. A mask over the raw uid would leave most shards empty.
func TestLockedShardedMapUidSpread(t *testing.T) {
	sm := NewLockedShardedMapWithShards[uint64, int](64)
	for i := 0; i < 6400; i++ {
		sm.Set(uint64(9_000_000+i), i)
	}
	used := 0
	minLen, maxLen := 1<<30, 0
	for i := range sm.shards {
		l := len(sm.shards[i])
		if l > 0 {
			used++
		}
		if l < minLen {
			minLen = l
		}
		if l > maxLen {
			maxLen = l
		}
	}
	require.Equal(t, len(sm.shards), used, "every shard should receive keys")
	require.Less(t, maxLen, 4*minLen, "shard occupancy is badly skewed (min=%d max=%d)", minLen, maxLen)
}

// TestLockedShardedMapConcurrent is the property that matters in the mutation
// pipeline: many workers writing disjoint keys at once. Run under -race.
func TestLockedShardedMapConcurrent(t *testing.T) {
	sm := NewLockedShardedMap[string, int]()
	const workers, per = 16, 500
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < per; i++ {
				sm.Set(fmt.Sprintf("w%d-k%d", w, i), w*per+i)
			}
		}(w)
	}
	wg.Wait()
	require.Equal(t, workers*per, sm.Len())
}

// BenchmarkLockedShardedMapSet measures the delta-write path as the mutation
// pipeline drives it: many goroutines Set-ing distinct string keys. Reports
// allocations too — the string hash used to heap-allocate on every call.
func BenchmarkLockedShardedMapSet(b *testing.B) {
	for _, workers := range []int{1, 8, 32, 64} {
		b.Run(fmt.Sprintf("workers=%d", workers), func(b *testing.B) {
			sm := NewLockedShardedMap[string, []byte]()
			val := make([]byte, 64)
			b.ReportAllocs()
			b.ResetTimer()
			var wg sync.WaitGroup
			per := b.N / workers
			if per < 1 {
				per = 1
			}
			for w := 0; w < workers; w++ {
				wg.Add(1)
				go func(w int) {
					defer wg.Done()
					for i := 0; i < per; i++ {
						sm.Set(fmt.Sprintf("\x00\x01pred\x00key-%d-%d", w, i), val)
					}
				}(w)
			}
			wg.Wait()
		})
	}
}

// BenchmarkLockedShardedMapGetShardIndex isolates the hash itself.
func BenchmarkLockedShardedMapGetShardIndex(b *testing.B) {
	sm := NewLockedShardedMap[string, int]()
	key := "\x00\x01somepredicate\x00\x00\x00\x00\x00\x00\x00\x01\x02"
	b.ReportAllocs()
	b.ResetTimer()
	sink := 0
	for i := 0; i < b.N; i++ {
		sink += sm.getShardIndex(key)
	}
	_ = sink
}

// TestLockedShardedMapMergeMismatchedShards covers the positional-merge hazard
// introduced by per-instance shard counts: shard i of one map no longer
// corresponds to shard i of another.
func TestLockedShardedMapMergeMismatchedShards(t *testing.T) {
	dst := NewLockedShardedMapWithShards[string, int](64)
	src := NewLockedShardedMapWithShards[string, int](8)
	for i := 0; i < 500; i++ {
		src.Set(fmt.Sprintf("k%d", i), 1)
	}
	dst.Set("k0", 10) // forces the aggregator path for one key

	dst.Merge(src, func(a, b int) int { return a + b })

	require.Equal(t, 500, dst.Len(), "no keys may be dropped across shard-count mismatch")
	v, ok := dst.Get("k0")
	require.True(t, ok)
	require.Equal(t, 11, v, "aggregator must run for pre-existing keys")
	v, ok = dst.Get("k499")
	require.True(t, ok)
	require.Equal(t, 1, v)
}
