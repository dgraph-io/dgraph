/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

// Package types contains some very common utilities used by Dgraph. These utilities
// are of "miscellaneous" nature, e.g., error checking.
package types

import (
	"hash/maphash"
	"math/bits"
	"runtime"
	"sync"
)

// shardSeed makes string hashing allocation-free. maphash.String takes the
// string directly, unlike farm.Fingerprint64 which needs a []byte conversion
// that escapes to the heap on every call — previously ~13% of all allocations
// in a 20k-edge mutation batch, since every delta write hashes its key. The
// seed is process-local, which is fine: shard assignment is never persisted or
// compared across processes.
var shardSeed = maphash.MakeSeed()

// lockedShards is the default shard count for LockedShardedMap. The mutation
// pipeline writes every posting-list delta through one of these maps from every
// predicate worker at once, so the shard count is a direct contention knob.
// NumShards (30) is sized for the unrelated, single-threaded types.ShardedMap
// and is far too thin on a many-core box.
//
// Oversubscribe relative to GOMAXPROCS so unlucky hash collisions between two
// active writers stay rare, and clamp so small boxes do not pay for empty maps
// and huge ones do not allocate absurdly.
func lockedShards() int {
	n := 4 * runtime.GOMAXPROCS(0)
	if n < 64 {
		n = 64
	}
	if n > 1024 {
		n = 1024
	}
	// Round up to a power of two: getShardIndex masks rather than divides.
	return 1 << bits.Len(uint(n-1))
}

// LockedShardedMap is a thread-safe, sharded map with generic key-value types.
type LockedShardedMap[K comparable, V any] struct {
	shards []map[K]V
	locks  []sync.RWMutex
	// mask is len(shards)-1; len(shards) is always a power of two.
	mask uint64
}

// NewLockedShardedMap creates a new LockedShardedMap sized for this machine.
func NewLockedShardedMap[K comparable, V any]() *LockedShardedMap[K, V] {
	return NewLockedShardedMapWithShards[K, V](lockedShards())
}

// NewLockedShardedMapWithShards creates a LockedShardedMap with an explicit
// shard count, rounded up to a power of two. Exposed mainly so tests can pin a
// non-default count and exercise the indexing.
func NewLockedShardedMapWithShards[K comparable, V any](n int) *LockedShardedMap[K, V] {
	if n < 1 {
		n = 1
	}
	n = 1 << bits.Len(uint(n-1))
	shards := make([]map[K]V, n)
	locks := make([]sync.RWMutex, n)
	for i := range shards {
		shards[i] = make(map[K]V)
	}
	return &LockedShardedMap[K, V]{shards: shards, locks: locks, mask: uint64(n - 1)}
}

func (s *LockedShardedMap[K, V]) getShardIndex(key K) int {
	// Index off this map's OWN shard count, never the package-level NumShards.
	// The two are unrelated now, and reading the global here would either
	// over-run s.shards or silently strand shards above index 30.
	switch k := any(key).(type) {
	case uint64:
		// Mix: uid keys are dense and monotonic, so the low bits alone would
		// stripe poorly once the shard count is a power of two.
		return int((k * 0x9E3779B97F4A7C15) >> 32 & s.mask)
	case string:
		return int(maphash.String(shardSeed, k) & s.mask)
	default:
		panic("LockedShardedMap only supports uint64 and string keys for now")
	}
}

func (s *LockedShardedMap[K, V]) Set(key K, value V) {
	if s == nil {
		return
	}
	index := s.getShardIndex(key)
	s.locks[index].Lock()
	defer s.locks[index].Unlock()
	s.shards[index][key] = value
}

func (s *LockedShardedMap[K, V]) Get(key K) (V, bool) {
	var zero V
	if s == nil {
		return zero, false
	}
	index := s.getShardIndex(key)
	s.locks[index].RLock()
	defer s.locks[index].RUnlock()
	val, ok := s.shards[index][key]
	return val, ok
}

func (s *LockedShardedMap[K, V]) Update(key K, update func(V, bool) V) {
	if s == nil {
		return
	}
	index := s.getShardIndex(key)
	s.locks[index].Lock()
	defer s.locks[index].Unlock()
	val, ok := s.shards[index][key]
	s.shards[index][key] = update(val, ok)
}

func (s *LockedShardedMap[K, V]) Merge(other *LockedShardedMap[K, V], ag func(a, b V) V) {
	// Shard counts are per-instance now, so shard i of `other` no longer
	// necessarily corresponds to shard i of `s`. Merging positionally across
	// differently-sized maps would drop or misplace keys silently, so route
	// mismatched maps through the key-by-key path instead.
	if len(s.shards) != len(other.shards) {
		_ = other.Iterate(func(k K, v V) error {
			s.Update(k, func(existing V, ok bool) V {
				if ok {
					return ag(existing, v)
				}
				return v
			})
			return nil
		})
		return
	}

	var wg sync.WaitGroup
	for i := range s.shards {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			otherShard := other.shards[i]
			for k, v := range otherShard {
				s.locks[i].Lock()
				if existing, ok := s.shards[i][k]; ok {
					s.shards[i][k] = ag(existing, v)
				} else {
					s.shards[i][k] = v
				}
				s.locks[i].Unlock()
			}
		}(i)
	}
	wg.Wait()
}

func (s *LockedShardedMap[K, V]) Len() int {
	if s == nil {
		return 0
	}
	var count int
	for i := range s.shards {
		s.locks[i].RLock()
		count += len(s.shards[i])
		s.locks[i].RUnlock()
	}
	return count
}

func (s *LockedShardedMap[K, V]) ParallelIterate(f func(K, V) error) error {
	if s == nil {
		return nil
	}

	var (
		wg    sync.WaitGroup
		errCh = make(chan error, 1)
		once  sync.Once
	)

	for i := range s.shards {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			s.locks[i].RLock()
			defer s.locks[i].RUnlock()

			for k, v := range s.shards[i] {
				if err := f(k, v); err != nil {
					once.Do(func() {
						errCh <- err
					})
					return
				}
			}
		}(i)
	}

	// Wait in a separate goroutine so we can still select on errCh.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case err := <-errCh:
		return err
	case <-done:
		return nil
	}
}

func (s *LockedShardedMap[K, V]) Iterate(f func(K, V) error) error {
	if s == nil {
		return nil
	}
	for i := range s.shards {
		s.locks[i].RLock()
		for k, v := range s.shards[i] {
			if err := f(k, v); err != nil {
				s.locks[i].RUnlock()
				return err
			}
		}
		s.locks[i].RUnlock()
	}
	return nil
}

func (s *LockedShardedMap[K, V]) IsEmpty() bool {
	if s == nil {
		return true
	}
	for i := range s.shards {
		s.locks[i].RLock()
		if len(s.shards[i]) > 0 {
			s.locks[i].RUnlock()
			return false
		}
		s.locks[i].RUnlock()
	}
	return true
}
