package trie

import (
	"bytes"
	"errors"
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

// Put inserts a key with value into the trie
func (t *Trie) Put(key, value []byte) error {
	if err := t.tryPut(key, value); err != nil {
		return err
	}

	return nil
}

func (t *Trie) tryPut(key, value []byte) (err error) {
	k := keyToHex(key)
	var n node

	if len(value) > 0 {
		_, n, err = t.insert(t.root, k, &leaf{key: nil, value: value})
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
		// need to convert this into a branch
		br := new(branch)
		length := lenCommonPrefix(key, p.key)
		br.key = key[:length]

		switch v := value.(type) {
		case *leaf:
			v.key = key[length+1:]
		case *branch:
			v.key = key[length+1:]
		}

		if length == len(p.key) {
			// if leaf's key is covered by this branch, then make the leaf's
			// value the value at this branch
			br.value = p.value
			br.children[key[length]] = value
		} else {
			// otherwise, make the leaf a child of the branch and update its partial key
			parentKey := p.key
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
		case *branch:
			_, n, err = t.insert(c, key[length+1:], value)
			p.children[key[length]] = n
			n = p
		default: // nil or leaf
			// otherwise, add node as child of this branch
			value.(*leaf).key = key[length+1:]
			p.children[key[length]] = value
			n = p
		}

		return true, n, err
	}

	// we need to branch out at the point where the keys diverge
	br := new(branch)

	// update partial keys, new branch has key up to matching length
	br.key = key[:length]

	parentIndex := p.key[length]
	_, br.children[parentIndex], err = t.insert(nil, p.key[length+1:], p)
	if err != nil {
		return false, nil, err
	}

	if len(key) == 0 {
		br.value = value.(*leaf).value
	} else {
		nodeIndex := key[length]
		_, br.children[nodeIndex], err = t.insert(nil, key[length+1:], value)
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
// leaf includes both partial key and value
func (t *Trie) getLeaf(key []byte) (value *leaf, err error) {
	l, err := t.tryGet(key)
	return l, err
}

func (t *Trie) tryGet(key []byte) (value *leaf, err error) {
	k := keyToHex(key)

	value, err = t.retrieve(t.root, k)
	return value, err
}

func (t *Trie) retrieve(parent node, key []byte) (value *leaf, err error) {
	switch p := parent.(type) {
	case *branch:
		length := lenCommonPrefix(p.key, key)

		// found the value at this node
		if bytes.Equal(p.key, key) || len(key) == 0 {
			return &leaf{key: p.key, value: p.value}, nil
		}

		// if branch's child at the key is a leaf, return it
		switch v := p.children[key[length]].(type) {
		case *leaf:
			value = v
		default:
			value, err = t.retrieve(p.children[key[length]], key[length+1:])
		}
	case *leaf:
		value = p
	case nil:
		return nil, nil
	default:
		err = errors.New("get error: invalid node")
	}
	return value, err
}

// Delete removes any existing value for key from the trie.
func (t *Trie) Delete(key []byte) error {
	k := keyToHex(key)
	_, n, err := t.delete(t.root, k)
	if err != nil {
		return err
	}
	t.root = n
	return nil
}

func (t *Trie) delete(parent node, key []byte) (ok bool, n node, err error) {
	return ok, nil, nil
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
