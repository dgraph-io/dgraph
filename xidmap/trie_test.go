package xidmap

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/dustin/go-humanize"
	"github.com/stretchr/testify/require"
)

func printTrie(t *testing.T, n *node, indent int) {
	if n == nil {
		return
	}
	t.Logf("%s node: %q %d\n", strings.Repeat(" ", indent), n.r, n.uid)
	printTrie(t, n.left, indent+1)
	printTrie(t, n.mid, indent+1)
	printTrie(t, n.right, indent+1)
}

func TestTrie(t *testing.T) {
	arena := NewArena(1 << 20)
	defer arena.Release()

	trie := NewTrie(arena)

	trie.Put("trie", 1)
	trie.Put("tree", 2)
	trie.Put("bird", 3)
	trie.Put("birds", 4)

	printTrie(t, trie.root, 0)

	require.Equal(t, uint64(1), trie.Get("trie"))
	require.Equal(t, uint64(2), trie.Get("tree"))
	require.Equal(t, uint64(3), trie.Get("bird"))
	require.Equal(t, uint64(4), trie.Get("birds"))
	t.Logf("Size of node: %d\n", nodeSz)
	t.Logf("Size used by allocator: %d\n", trie.offset)
}

// var N = 10000000

// $ go test -bench=BenchmarkWordsTrie --run=XXX -benchmem -memprofile mem.out
// $ go tool pprof mem.out
//
// Results show that Trie uses ~450 bytes per word. While Map uses ~100 bytes per word. So, Trie
// doesn't make sense.
func BenchmarkWordsTrie(b *testing.B) {
	buf := make([]byte, 32)
	arena := NewArena(8 << 30)
	defer arena.Release()

	trie := NewTrie(arena)
	var uid uint64
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		rand.Read(buf)
		uid++
		word := string(buf)
		trie.Put(word, uid)
	}
	b.Logf("Words: %d. Allocator: %s. Per word: %d\n", uid,
		humanize.IBytes(uint64(trie.offset)),
		uint64(trie.offset)/uid)
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
