/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
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

package xidmap

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/dustin/go-humanize"
	"github.com/stretchr/testify/require"
)

func TestTrie(t *testing.T) {
	require.Equal(t, uint32(24), uint32(nodeSz),
		"Size of Trie node should be 24. Got: %d\n", nodeSz)

	trie := NewTrie()
	defer trie.Release()

	trie.Put("trie", 1)
	trie.Put("tree", 2)
	trie.Put("bird", 3)
	trie.Put("birds", 4)
	trie.Put("t", 5)

	require.Equal(t, uint64(0), trie.Get(""))
	require.Equal(t, uint64(1), trie.Get("trie"))
	require.Equal(t, uint64(2), trie.Get("tree"))
	require.Equal(t, uint64(3), trie.Get("bird"))
	require.Equal(t, uint64(4), trie.Get("birds"))
	require.Equal(t, uint64(5), trie.Get("t"))
	t.Logf("Size of node: %d\n", nodeSz)
	t.Logf("Size used by allocator: %d\n", trie.Size())
}

func TestTrieIterate(t *testing.T) {
	keys := make([]string, 0)
	uids := make([]uint64, 0)
	trie := NewTrie()

	i := uint64(1)
	for ; i <= 1000; i++ {
		trie.Put(fmt.Sprintf("%05d", i), i)
	}

	err := trie.Iterate(func(key string, uid uint64) error {
		keys = append(keys, key)
		uids = append(uids, uid)
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 1000, len(keys))
	require.Equal(t, 1000, len(uids))

	for i := range keys {
		val := uint64(i + 1)
		require.Equal(t, fmt.Sprintf("%05d", val), keys[i])
		require.Equal(t, val, uids[i])
	}
}

// $ go test -bench=BenchmarkWordsTrie --run=XXX -benchmem -memprofile mem.out
// $ go tool pprof mem.out
func BenchmarkWordsTrie(b *testing.B) {
	buf := make([]byte, 32)

	trie := NewTrie()
	defer trie.Release()

	var uid uint64
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		rand.Read(buf)
		uid++
		word := string(buf)
		trie.Put(word, uid)
	}
	b.Logf("Words: %d. Allocator: %s. Per word: %d\n", uid,
		humanize.IBytes(uint64(trie.Size())),
		uint64(trie.Size())/uid)
	b.StopTimer()
}

func BenchmarkWordsMap(b *testing.B) {
	buf := make([]byte, 32)
	m := make(map[string]uint64)
	var uid uint64

	for i := 0; i < b.N; i++ {
		rand.Read(buf)
		uid++
		word := string(buf)
		m[word] = uid
	}

	var count int
	for word := range m {
		_ = word
		count++
	}
	b.Logf("Number of words added: %d\n", count)
}
