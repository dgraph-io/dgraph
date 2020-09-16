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
	trie := NewTrie()

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
	t.Logf("Size used by allocator: %d\n", trie.alloc.Size())
}

var N = 10000000

// $ go test -bench=BenchmarkWordsTrie --run=XXX -benchmem -memprofile mem.out
// $ go tool pprof mem.out
//
// Results show that Trie uses ~450 bytes per word. While Map uses ~100 bytes per word. So, Trie
// doesn't make sense.
func BenchmarkWordsTrie(b *testing.B) {
	buf := make([]byte, 16)
	trie := NewTrie()
	var uid uint64

	for i := 0; i < N; i++ {
		rand.Read(buf)
		uid++
		word := string(buf)
		trie.Put(word, uid)
	}
	b.Logf("Number of words added: %d\n", uid)
	b.Logf("Size used by allocator: %s\n", humanize.IBytes(trie.alloc.Size()))
	b.Logf("Size per word: %d\n", trie.alloc.Size()/uid)
}

func BenchmarkWordsMap(b *testing.B) {
	buf := make([]byte, 16)
	m := make(map[string]uint64)
	var uid uint64

	for i := 0; i < N; i++ {
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
