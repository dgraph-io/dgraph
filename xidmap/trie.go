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
	"math"
	"unsafe"

	"github.com/dgraph-io/ristretto/z"
)

// Trie is an implementation of Ternary Search Tries to store XID to UID map. It uses Arena to
// allocate nodes in the trie. It is not thread-safe.
type Trie struct {
	root uint32
	buf  *z.Buffer
}

// NewTrie would return back a Trie backed by the provided Arena. Trie would assume ownership of the
// Arena. Release must be called at the end to release Arena's resources.
func NewTrie() *Trie {
	buf := z.NewBuffer(32<<20, "Trie").WithMaxSize(math.MaxUint32)
	// Add additional 8 bytes at the start, because offset=0 is used for checking non-existing node.
	// Therefore we can't keep root at 0 offset.
	ro := buf.AllocateOffset(nodeSz + 8)
	return &Trie{
		root: uint32(ro + 8),
		buf:  buf,
	}
}
func (t *Trie) getNode(offset uint32) *node {
	if offset == 0 {
		return nil
	}
	data := t.buf.Data(int(offset))
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
	return uint32(t.buf.LenNoPadding())
}

type iterFn func(key string, uid uint64) error

func (t *Trie) Iterate(fn iterFn) error {
	return t.iterate(t.root, "", fn)
}

// Release would release the resources used by the Arena.
func (t *Trie) Release() {
	t.buf.Release()
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

var nodeSz = int(unsafe.Sizeof(node{}))

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
		offset = uint32(t.buf.AllocateOffset(nodeSz))
		n = t.getNode(offset)
		n.r = r
	}

	switch {
	case r < n.r:
		n.left = t.put(n.left, key, uid)

	case r > n.r:
		n.right = t.put(n.right, key, uid)

	case len(key[1:]) > 0:
		n.mid = t.put(n.mid, key[1:], uid)

	default:
		n.uid = uid
	}
	return offset
}

func (t *Trie) iterate(offset uint32, prefix string, fn iterFn) error {
	if offset == 0 {
		return nil
	}

	n := t.getNode(offset)
	if n == nil {
		return nil
	}

	if err := t.iterate(n.left, prefix, fn); err != nil {
		return err
	}

	if n.uid != 0 {
		if err := fn(prefix+string(n.r), n.uid); err != nil {
			return err
		}
	}
	if err := t.iterate(n.mid, prefix+string(n.r), fn); err != nil {
		return err
	}

	if err := t.iterate(n.right, prefix, fn); err != nil {
		return err
	}

	return nil
}
