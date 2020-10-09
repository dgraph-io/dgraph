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
	"sort"
	"unsafe"

	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
)

const (
	branchingFactor = 5
)

type BpTree struct {
	root uint32
	buf  *z.Buffer
}

type bpNode struct {
	keys     [branchingFactor - 1]uint64
	vals     [branchingFactor - 1]uint64
	children [branchingFactor]uint32
	leaf     bool
}

var bpNodeSz = int(unsafe.Sizeof(bpNode{}))

func (n *bpNode) insert(key, val uint64) bool {
	if n.keys[len(n.keys)-1] > 0 {
		return false
	}

	for i, otherKey := range n.keys {
		if otherKey == 0 {
			n.keys[i] = key
			n.vals[i] = val
			break
		}
	}
	n.sort()
	return true
}

type kv struct {
	key uint64
	val uint64
}

func (n *bpNode) sort() {
	kvs := make([]kv, 0)
	for i := range n.keys {
		if n.keys[i] == 0 {
			break
		}
		kvs = append(kvs, kv{key: n.keys[i], val: n.vals[i]})
	}
	sort.Slice(kvs, func(i, j int) bool {
		return kvs[i].key < kvs[j].key
	})

	for i, kv := range kvs {
		n.keys[i] = kv.key
		n.vals[i] = kv.val
	}
}

func (n *bpNode) search(key uint64) (uint64, uint32, bool) {
	for i, otherKey := range n.keys {
		if otherKey == 0 {
			return 0, n.children[i], false
		}
		if key < otherKey {
			return 0, n.children[i], false
		}
		if key == otherKey {
			return n.vals[i], n.children[i], false
		}
	}
	return 0, n.children[len(n.children)-1], false
}

func NewBpTree() *BpTree {
	buf, err := z.NewBufferWith(32<<20, math.MaxUint32, z.UseMmap)
	x.Check(err)
	ro := buf.AllocateOffset(nodeSz)
	t := &BpTree{
		root: uint32(ro),
		buf:  buf,
	}
	rNode := t.getNode(t.root)
	rNode.leaf = true
	return t
}

func (t *BpTree) getNode(offset uint32) *bpNode {
	if offset == 0 {
		return nil
	}
	data := t.buf.Data(int(offset))
	return (*bpNode)(unsafe.Pointer(&data[0]))
}

func (t *BpTree) Get(key uint64) (uint64, bool) {
	node := t.getNode(t.root)
	for node != nil {
		val, nextNode, found := node.search(key)
		if found {
			return val, true
		}
		if node.leaf {
			break
		}
		if nextNode == 0 {
			break
		}

		node = t.getNode(nextNode)
	}
	return 0, false
}

func (t *BpTree) Insert(key, val uint64) {
	// Find the node.
	node := t.getNode(t.root)
	for node != nil {
		if node.leaf {
			break
		}

		_, next, _ := node.search(key)
		if next == 0 {
			break
		}
		node = t.getNode(next)
	}

	if node.insert(key, val) {
		return
	}

	// Split nodes recursively.
}
