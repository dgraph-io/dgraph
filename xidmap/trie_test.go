package xidmap

import (
	"math/rand"
	"testing"

	"github.com/dustin/go-humanize"
	"github.com/stretchr/testify/require"
)

func TestTrie(t *testing.T) {
	require.Equal(t, uint32(24), uint32(nodeSz),
		"Size of Trie node should be 24. Got: %d\n", nodeSz)

	trie := NewTrie()

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

// $ go test -bench=BenchmarkWordsTrie --run=XXX -benchmem -memprofile mem.out
// $ go tool pprof mem.out
//
// Results show that Trie uses ~450 bytes per word. While Map uses ~100 bytes per word. So, Trie
// doesn't make sense.
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
