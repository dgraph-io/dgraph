package xidmap

import (
	"unicode/utf8"
	"unsafe"

	"github.com/dgraph-io/ristretto/z"
)

type Trie struct {
	root  *node
	alloc *z.Allocator
}

func NewTrie() *Trie {
	return &Trie{
		root:  &node{},
		alloc: z.NewAllocator(1024),
	}
}
func (t *Trie) Get(key string) uint64 {
	return get(t.root, key)
}
func (t *Trie) Put(key string, uid uint64) {
	t.put(t.root, key, uid)
}
func (t *Trie) Release() {
	t.alloc.Release()
}

type node struct {
	left  *node
	mid   *node
	right *node
	uid   uint64
	r     rune
}

var nodeSz = int(unsafe.Sizeof(node{}))

func get(n *node, key string) uint64 {
	if n == nil {
		return 0
	}
	r, width := utf8.DecodeRuneInString(key)
	if r < n.r {
		return get(n.left, key)
	}
	if r > n.r {
		return get(n.right, key)
	}

	// rune matches
	if len(key[width:]) > 0 {
		return get(n.mid, key[width:])
	}
	return n.uid
}

func (t *Trie) put(n *node, key string, uid uint64) *node {
	r, width := utf8.DecodeRuneInString(key)
	if n == nil {
		b := t.alloc.Allocate(nodeSz)
		n = (*node)(unsafe.Pointer(&b[0]))
		n.r = r
	}
	switch {
	case r < n.r:
		n.left = t.put(n.left, key, uid)

	case r > n.r:
		n.right = t.put(n.right, key, uid)

	case len(key[width:]) > 0:
		n.mid = t.put(n.mid, key[width:], uid)

	default:
		n.uid = uid
	}
	return n
}
