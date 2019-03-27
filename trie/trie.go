package trie

import (
	"bytes"
	"errors"

	"github.com/ChainSafeSystems/gossamer/polkadb"
)

// Trie is a Merkle Patricia Trie.
// The zero value is an empty trie with no database.
// Use NewTrie to create a trie that sits on top of a database.
type Trie struct {
	db         polkadb.Database
	root       node
	merkleRoot [32]byte
}

// NewEmptyTrie creates a trie with a nil root and merkleRoot
func NewEmptyTrie(db polkadb.Database) *Trie {
	return &Trie{
		db:         db,
		root:       nil,
		merkleRoot: [32]byte{},
	}
}

// NewTrie creates a trie with an existing root node from db
func NewTrie(db polkadb.Database, root node, merkleRoot [32]byte) *Trie {
	return &Trie{
		db:         db,
		root:       root,
		merkleRoot: merkleRoot,
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
	_, n, err := t.insert(t.root, nil, k, leaf(value))
	if err != nil {
		return err
	}
	t.root = n

	return nil
}

// TryPut attempts to insert a key with value into the trie
func (t *Trie) insert(parent node, prefix, key []byte, value node) (ok bool, n node, err error) {
	if len(key) == 0 {
		if v, ok := parent.(leaf); ok {
			return !bytes.Equal(v, value.(leaf)), value, nil
		}
		return true, value, nil
	}

	switch p := parent.(type) {
	case *extension:
		ok, n, err = t.updateExtension(p, prefix, key, value)
	case *branch:
		ok, n, err = t.updateBranch(p, prefix, key, value)
	case nil:
		n = &extension{key, value}
		ok = true
	default:
		err = errors.New("cannot put: invalid parent node")
	}

	return ok, n, err
}

// updateExtension attempts to update an extension node to have the value node
// if the keys match, then set the value of the extension to the value node
// otherwise, we need to branch out where the keys diverge. we make a branch and
// try to insert the value node at the diverging key index. if there's already a
// branch there, move any children to the new branch
func (t *Trie) updateExtension(p *extension, prefix, key []byte, value node) (ok bool, n node, err error) {
	length := lenCommonPrefix(key, p.key)

	// whole parent key matches, so just attach a node to the parent
	if length == len(p.key) {
		// add new node to branch
		ok, n, err = t.insert(p.value, append(prefix, key[:length]...), key[length:], value)
		if ok && err != nil {
			ok = true
			n = &extension{p.key, n}
		}
		return ok, n, err
	}

	// otherwise, we need to branch out at the point where the keys diverge
	br := new(branch)

	_, br.children[p.key[length]], err = t.insert(nil, append(prefix, p.key[:length+1]...), p.key[length+1:], p.value)
	if err != nil {
		return false, nil, err
	}

	_, br.children[key[length]], err = t.insert(nil, append(prefix, key[:length+1]...), key[length+1:], value)
	if err != nil {
		return false, nil, err
	}

	if length == 0 {
		// no matching prefix, replace this extension with a branch
		ok = true
		n = br
	} else {
		// some prefix matches, replace with extension that starts where the keys diverge
		ok = true
		n = &extension{key[:length], br}
	}

	return ok, n, err
}

// updateBranch attempts to add the value node to a branch
// inserts the value node as the branch's child at the index that's
// the first nibble of the key
func (t *Trie) updateBranch(p *branch, prefix, key []byte, value node) (ok bool, n node, err error) {
	// we need to add an extension to the child that's already there
	ok, n, err = t.insert(p.children[key[0]], append(prefix, key[0]), key[1:], value)
	if !ok || err != nil {
		return false, n, err
	}

	p.children[key[0]] = n
	return true, p, err
}

// Get returns the value for key stored in the trie.
func (t *Trie) Get(key []byte) (value []byte, err error) {
	value, err = t.tryGet(key)
	return value, err
}

func (t *Trie) tryGet(key []byte) (value []byte, err error) {
	k := keyToHex(key)
	value, err = t.retrieve(t.root, k, 0)
	return value, err
}

func (t *Trie) retrieve(parent node, key []byte, i int) (value []byte, err error) {
	switch p := parent.(type) {
	case *extension:
		if len(key)-i < len(p.key) { //|| !bytes.Equal(p.key, key[i:i+len(p.key)]) {
			return nil, nil
		}
		value, err = t.retrieve(p.value, key, i+len(p.key))
	case *branch:
		value, err = t.retrieve(p.children[key[i]], key, i+1)
	case leaf:
		value = p
	default:
		err = errors.New("get error: invalid node")
	}
	return value, err
}

// Delete removes any existing value for key from the trie.
func (t *Trie) Delete(key []byte) {
	// TODO
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
