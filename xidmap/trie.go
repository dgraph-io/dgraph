package xidmap

import (
	"io/ioutil"
	"os"
	"sync/atomic"
	"unicode/utf8"
	"unsafe"

	"github.com/dgraph-io/badger/v2/y"
	"github.com/dgraph-io/dgraph/x"
)

// Arena is thread-safe.
type Arena struct {
	data   []byte
	fd     *os.File
	offset int64
}

func (a *Arena) Allocate(sz int64) []byte {
	off := atomic.AddInt64(&a.offset, sz)
	x.AssertTrue(off < int64(len(a.data)))
	return a.data[off-sz : off]
}
func (a *Arena) Size() int64 {
	return atomic.LoadInt64(&a.offset)
}
func (a *Arena) Release() {
	// TODO: Add a x.Log
	x.Check(y.Munmap(a.data))
	x.Check(a.fd.Truncate(0))
	x.Check(os.Remove(a.fd.Name()))
}
func NewArena(sz int64) *Arena {
	f, err := ioutil.TempFile("", "arena")
	x.Check(err)
	f.Truncate(sz)

	// mtype := unix.PROT_READ | unix.PROT_WRITE
	// data, err := unix.Mmap(-1, 0, int(sz), mtype, unix.MAP_SHARED|unix.MAP_ANONYMOUS)
	data, err := y.Mmap(f, true, sz)
	x.Check(err)

	return &Arena{
		data:   data,
		fd:     f,
		offset: 0,
	}
}

type Trie struct {
	root   *node
	alloc  *Arena
	offset int
}

func NewTrie(alloc *Arena) *Trie {
	return &Trie{
		root:  &node{},
		alloc: alloc,
		// alloc: z.NewAllocator(1024),
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

// arena of 4GB. Then we can use uint32 for offsets.
type node struct {
	left  *node // 8 bytes
	mid   *node
	right *node
	uid   uint64
	r     rune
}

var nodeSz = int64(unsafe.Sizeof(node{}))

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
