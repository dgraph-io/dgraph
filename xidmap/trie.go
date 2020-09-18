package xidmap

import (
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"unsafe"

	"github.com/dgraph-io/badger/v2/y"
	"github.com/dgraph-io/dgraph/x"
)

// Arena uses file mmap to allocate memory. This allows us to store big data structures like Tries
// without using physical memory. Arena limits to 4GB, so we can use uint32 to save the cost of
// allocating a node. Shards can take care of bigger datasets. Each trie would have it's own arena,
// no need for making this thread-safe. Arena can grow the file as needed, to avoid pre-allocating
// really big files.
// Arena is not thread-safe.
type Arena struct {
	data   []byte
	fd     *os.File
	offset uint32
	fsz    int64
}

// Allocate would allocate the given size of bytes in the Arena. If needed, it would remap the
// underlying file to double the existing size. Allocate would crash if we reach MaxUint32.
func (a *Arena) Allocate(sz uint32) uint32 {
	if a.fsz-int64(a.offset) < int64(sz) {
		x.AssertTrue(a.fsz < math.MaxUint32)
		toSize := a.fsz * 2
		if toSize > math.MaxUint32 {
			toSize = math.MaxUint32
		}

		// glog.V(2).Infof("Remapping Arena from %d to %d\n", len(a.data), toSize)
		fmt.Printf("Remapping Arena from %d to %d\n", len(a.data), toSize)
		// x.Check(unix.Msync(a.data, unix.MS_SYNC))
		// x.Check(y.Munmap(a.data))
		// x.Check(a.fd.Sync())
		x.Check(a.fd.Truncate(toSize))
		a.fsz = toSize

		// data, err := y.Mmap(a.fd, true, toSize)
		// x.Check(err)
		// a.zeroOut()
		// a.data = data
	}
	a.offset += sz
	return a.offset - sz
}

// Size returns the current Arena offset.
func (a *Arena) Size() uint32 {
	return a.offset
}

// Data returns a slice of data from the provided offset. It does not cap the end of the slice. So,
// use carefully.
func (a *Arena) Data(offset uint32) []byte {
	x.AssertTrue(int(offset) < len(a.data))
	return a.data[offset:]
}

// Release unmaps the file, truncates it and deletes it.
func (a *Arena) Release() {
	x.Log(y.Munmap(a.data), "while unmapping Arena")
	x.Log(a.fd.Truncate(0), "while truncating Arena file")
	x.Log(os.Remove(a.fd.Name()), "while deleting Arena file")
}

// Don't think we need this. Doing this is expensive.
// func (a *Arena) zeroOut() {
// 	data := a.data[a.offset:a.fsz]
// 	if len(data) == 0 {
// 		return
// 	}
// 	data[0] = 0x00
// 	for bp := 1; bp < len(data); bp *= 2 {
// 		copy(data[bp:], data[:bp])
// 	}
// }

func NewArena(sz int64) *Arena {
	x.AssertTruef(sz <= math.MaxUint32,
		"Arena Size %d should be under MaxUint32 %d", sz, math.MaxUint32)
	fd, err := ioutil.TempFile("", "arena")
	x.Check(err)
	fd.Truncate(sz)

	// mtype := unix.PROT_READ | unix.PROT_WRITE
	// data, err := unix.Mmap(-1, 0, int(sz), mtype, unix.MAP_SHARED|unix.MAP_ANONYMOUS)
	data, err := y.Mmap(fd, true, 4<<30) // Mmap the entire 4GB.
	x.Check(err)

	data[0] = 0x00
	a := &Arena{
		// data:   make([]byte, sz),
		data:   data,
		fd:     fd,
		fsz:    sz,
		offset: 1, // Skip offset zero for nil nodes.
	}
	// a.zeroOut()
	return a
}

// Trie is an implementation of Ternary Search Tries to store XID to UID map. It uses Arena to
// allocate nodes in the trie. It is not thread-safe.
type Trie struct {
	root  uint32
	alloc *Arena
}

// NewTrie would return back a Trie backed by the provided Arena. Trie would assume ownership of the
// Arena. Release must be called at the end to release Arena's resources.
func NewTrie(alloc *Arena) *Trie {
	ro := alloc.Allocate(nodeSz)
	return &Trie{
		root:  ro,
		alloc: alloc,
		// alloc: z.NewAllocator(1024),
	}
}
func (t *Trie) getNode(offset uint32) *node {
	if offset == 0 {
		return nil
	}
	data := t.alloc.Data(offset)
	return (*node)(unsafe.Pointer(&data[0]))
}

// Get would return the UID for the key. If the key is not found, it would return 0.
func (t *Trie) Get(key string) uint64 {
	return t.get(t.root, key)
}

// Put would store the UID for the key.
func (t *Trie) Put(key string, uid uint64) {
	t.put(t.root, key, uid)
}

// Size returns the size of Arena used by this Trie so far.
func (t *Trie) Size() uint32 {
	return t.alloc.Size()
}

// Release would release the resources used by the Arena.
func (t *Trie) Release() {
	t.alloc.Release()
}

// node uses 4-byte offsets to save the cost of storing 8-byte pointers. Also, offsets allow us to
// truncate the file bigger and remap it. This struct costs 24 bytes.
type node struct {
	uid   uint64
	r     byte
	left  uint32
	mid   uint32
	right uint32
}

// TODO: Try using skiplists on mmap instead?

var nodeSz = uint32(unsafe.Sizeof(node{}))

func (t *Trie) get(offset uint32, key string) uint64 {
	if len(key) == 0 {
		return 0
	}
	for offset != 0 {
		n := t.getNode(offset)
		r := key[0]
		switch {
		case r < n.r:
			offset = n.left
		case r > n.r:
			offset = n.right
		case len(key[1:]) > 0:
			key = key[1:]
			offset = n.mid
		default:
			return n.uid
		}
	}
	return 0
}

func (t *Trie) put(offset uint32, key string, uid uint64) uint32 {
	n := t.getNode(offset)
	r := key[0]
	if n == nil {
		offset = t.alloc.Allocate(nodeSz)
		n = t.getNode(offset)
		n.r = r
	}

	switch {
	case r < n.r:
		off := t.put(n.left, key, uid)
		// We need to get the node again to avoid holding a reference to arena's data struct, which
		// might have remapped due to a call to Allocate.
		t.getNode(offset).left = off

	case r > n.r:
		off := t.put(n.right, key, uid)
		t.getNode(offset).right = off

	case len(key[1:]) > 0:
		off := t.put(n.mid, key[1:], uid)
		t.getNode(offset).mid = off

	default:
		n.uid = uid
	}
	return offset
}
