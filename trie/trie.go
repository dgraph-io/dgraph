// Copyright 2019 ChainSafe Systems (ON) Corp.
// This file is part of gossamer.
//
// The gossamer library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The gossamer library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the gossamer library. If not, see <http://www.gnu.org/licenses/>.

package trie

import (
	"bytes"
	"errors"

	"github.com/ChainSafe/gossamer/common"
)

// Trie is a Merkle Patricia Trie.
// The zero value is an empty trie with no database.
// Use NewTrie to create a trie that sits on top of a database.
type Trie struct {
	db   *Database
	root node
}

// NewEmptyTrie creates a trie with a nil root and merkleRoot
func NewEmptyTrie(db *Database) *Trie {
	return &Trie{
		db:   db,
		root: nil,
	}
}

// NewTrie creates a trie with an existing root node from db
func NewTrie(db *Database, root node) *Trie {
	return &Trie{
		db:   db,
		root: root,
	}
}

// Root returns the root of the trie
func (t *Trie) Root() node {
	return t.root
}

// Db returns the trie's underlying database
func (t *Trie) Db() *Database {
	return t.db
}

// EncodeRoot returns the encoded root of the trie
func (t *Trie) EncodeRoot() ([]byte, error) {
	return Encode(t.root)
}

// Hash returns the hashed root of the trie
func (t *Trie) Hash() (common.Hash, error) {
	encRoot, err := t.EncodeRoot()
	if err != nil {
		return [32]byte{}, err
	}

	return common.Blake2bHash(encRoot)
}

// Entries returns all the key-value pairs in the trie as a map of keys to values
func (t *Trie) Entries() map[string][]byte {
	return t.entries(t.root, nil, make(map[string][]byte))
}

func (t *Trie) entries(current node, prefix []byte, kv map[string][]byte) map[string][]byte {
	switch c := current.(type) {
	case *branch:
		if c.value != nil {
			kv[string(nibblesToKeyLE(append(prefix, c.key...)))] = c.value
		}
		for i, child := range c.children {
			t.entries(child, append(prefix, append(c.key, byte(i))...), kv)
		}
	case *leaf:
		kv[string(nibblesToKeyLE(append(prefix, c.key...)))] = c.value
		return kv
	}

	return kv
}

// Put inserts a key with value into the trie
func (t *Trie) Put(key, value []byte) error {
	if err := t.tryPut(key, value); err != nil {
		return err
	}

	return nil
}

func (t *Trie) tryPut(key, value []byte) (err error) {
	k := keyToNibbles(key)
	var n node

	if len(value) > 0 {
		_, n, err = t.insert(t.root, k, &leaf{key: nil, value: value, dirty: true})
	} else {
		_, n, err = t.delete(t.root, k)
	}

	if err != nil {
		return err
	}

	t.root = n
	return nil
}

// TryPut attempts to insert a key with value into the trie
func (t *Trie) insert(parent node, key []byte, value node) (ok bool, n node, err error) {
	switch p := parent.(type) {
	case *branch:
		ok, n, err = t.updateBranch(p, key, value)
	case nil:
		switch v := value.(type) {
		case *branch:
			v.key = key
			n = v
			ok = true
		case *leaf:
			v.key = key
			n = v
			ok = true
		}
	case *leaf:
		// need to convert this leaf into a branch
		br := &branch{dirty: true}
		length := lenCommonPrefix(key, p.key)

		if bytes.Equal(p.key, key) && len(key) == length {
			return true, value, nil
		}

		br.key = key[:length]
		parentKey := p.key

		// value goes at this branch
		if len(key) == length {
			br.value = value.(*leaf).value

			// if we are not replacing previous leaf, then add it as a child to the new branch
			if len(parentKey) > len(key) {
				p.key = p.key[length+1:]
				br.children[parentKey[length]] = p
			}

			return true, br, nil
		}

		value.setKey(key[length+1:])

		if length == len(p.key) {
			// if leaf's key is covered by this branch, then make the leaf's
			// value the value at this branch
			br.value = p.value
			br.children[key[length]] = value
		} else {
			// otherwise, make the leaf a child of the branch and update its partial key
			p.key = p.key[length+1:]
			br.children[parentKey[length]] = p
			br.children[key[length]] = value
		}

		return ok, br, nil
	default:
		err = errors.New("put error: invalid node")
	}

	return ok, n, err
}

// updateBranch attempts to add the value node to a branch
// inserts the value node as the branch's child at the index that's
// the first nibble of the key
func (t *Trie) updateBranch(p *branch, key []byte, value node) (ok bool, n node, err error) {
	length := lenCommonPrefix(key, p.key)

	// whole parent key matches
	if length == len(p.key) {
		// if node has same key as this branch, then update the value at this branch
		if bytes.Equal(key, p.key) {
			switch v := value.(type) {
			case *branch:
				p.value = v.value
			case *leaf:
				p.value = v.value
			}
			return true, p, nil
		}

		switch c := p.children[key[length]].(type) {
		case *branch, *leaf:
			_, n, err = t.insert(c, key[length+1:], value)
			p.children[key[length]] = n
			n = p
		case nil:
			// otherwise, add node as child of this branch
			value.(*leaf).key = key[length+1:]
			p.children[key[length]] = value
			n = p
		}

		return true, n, err
	}

	// we need to branch out at the point where the keys diverge
	// update partial keys, new branch has key up to matching length
	br := &branch{key: key[:length], dirty: true}

	parentIndex := p.key[length]
	_, br.children[parentIndex], err = t.insert(nil, p.key[length+1:], p)
	if err != nil {
		return false, nil, err
	}

	if len(key) <= length {
		br.value = value.(*leaf).value
	} else {
		_, br.children[key[length]], err = t.insert(nil, key[length+1:], value)
		if err == nil {
			ok = true
		}
	}

	return ok, br, err
}

// Get returns the value for key stored in the trie at the corresponding key
func (t *Trie) Get(key []byte) (value []byte, err error) {
	l, err := t.tryGet(key)
	if l != nil {
		return l.value, err
	}
	return nil, err
}

// getLeaf returns the leaf node stored in the trie at the corresponding key
// leaf includes both partial key and value, need the partial key for encoding
func (t *Trie) getLeaf(key []byte) (value *leaf, err error) {
	l, err := t.tryGet(key)
	return l, err
}

func (t *Trie) tryGet(key []byte) (value *leaf, err error) {
	k := keyToNibbles(key)

	value, err = t.retrieve(t.root, k)
	return value, err
}

func (t *Trie) retrieve(parent node, key []byte) (value *leaf, err error) {
	switch p := parent.(type) {
	case *branch:
		length := lenCommonPrefix(p.key, key)

		// found the value at this node
		if bytes.Equal(p.key, key) || len(key) == 0 {
			return &leaf{key: p.key, value: p.value, dirty: true}, nil
		}

		// did not find value
		if bytes.Equal(p.key[:length], key) && len(key) < len(p.key) {
			return nil, nil
		}

		value, err = t.retrieve(p.children[key[length]], key[length+1:])
	case *leaf:
		if bytes.Equal(p.key, key) {
			value = p
		}
	case nil:
		return nil, nil
	default:
		err = errors.New("get error: invalid node")
	}
	return value, err
}

// Delete removes any existing value for key from the trie.
func (t *Trie) Delete(key []byte) error {
	k := keyToNibbles(key)
	_, n, err := t.delete(t.root, k)
	if err != nil {
		return err
	}
	t.root = n
	return nil
}

func (t *Trie) delete(parent node, key []byte) (ok bool, n node, err error) {
	switch p := parent.(type) {
	case *branch:
		length := lenCommonPrefix(p.key, key)

		if bytes.Equal(p.key, key) || len(key) == 0 {
			// found the value at this node
			p.value = nil
			n = p
		} else {
			_, n, err = t.delete(p.children[key[length]], key[length+1:])
			if err != nil {
				return false, p, err
			}
			p.children[key[length]] = n
			n = p
		}

		ok, n, err = handleDeletion(p, n, key)
	case *leaf:
		if bytes.Equal(key, p.key) || len(key) == 0 {
			ok = true
		} else {
			ok = true
			n = p
		}
	case nil:
		// do nothing
	}
	return ok, n, err
}

// handleDeletion is called when a value is deleted from a branch
// if the updated branch only has 1 child, it should be combined with that child
// if the upated branch only has a value, it should be turned into a leaf
func handleDeletion(p *branch, n node, key []byte) (ok bool, nn node, err error) {
	nn = n
	length := lenCommonPrefix(p.key, key)
	bitmap := p.childrenBitmap()

	// if branch has no children, just a value, turn it into a leaf
	if bitmap == 0 && p.value != nil {
		nn = &leaf{key: key[:length], value: p.value}
	} else if p.numChildren() == 1 && p.value == nil {
		// there is only 1 child and no value, combine the child branch with this branch
		// find index of child
		var i int
		for i = 0; i < 16; i++ {
			bitmap = bitmap >> 1
			if bitmap == 0 {
				break
			}
		}

		child := p.children[i]
		switch c := child.(type) {
		case *leaf:
			nn = &leaf{key: append(append(p.key, []byte{byte(i)}...), c.key...), value: c.value}
		case *branch:
			br := new(branch)
			br.key = append(p.key, append([]byte{byte(i)}, c.key...)...)

			// adopt the grandchildren
			for i, grandchild := range c.children {
				if grandchild != nil {
					br.children[i] = grandchild
				}
			}

			br.value = c.value
			nn = br
		default:
			// do nothing
		}

		ok = true
	}

	return ok, nn, err
}

// lenCommonPrefix returns the length of the common prefix between two keys
func lenCommonPrefix(a, b []byte) int {
	var length, max = 0, len(a)

	if len(a) > len(b) {
		max = len(b)
	}

	for ; length < max; length++ {
		if a[length] != b[length] {
			break
		}
	}

	return length
}
