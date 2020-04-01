package blocktree

import (
	"bytes"
	"encoding/binary"
	"io"
	"math/big"

	"github.com/ChainSafe/gossamer/lib/common"
)

// Store stores the blocktree in the underlying db
func (bt *BlockTree) Store() error {
	if bt.db == nil {
		return ErrNilDatabase
	}

	enc, err := bt.Encode()
	if err != nil {
		return err
	}

	return bt.db.Put(common.BlockTreeKey, enc)
}

// Load loads the blocktree from the underlying db
func (bt *BlockTree) Load() error {
	if bt.db == nil {
		return ErrNilDatabase
	}

	enc, err := bt.db.Get(common.BlockTreeKey)
	if err != nil {
		return err
	}

	return bt.Decode(enc)
}

// Encode recursively encodes the block tree
// enc(node) = [32B block hash + 8B arrival time + 8B num children n] | enc(children[0]) | ... | enc(children[n-1])
func (bt *BlockTree) Encode() ([]byte, error) {
	return encodeRecursive(bt.head, []byte{})
}

// encode recursively encodes the blocktree by depth-first traversal
func encodeRecursive(n *node, enc []byte) ([]byte, error) {
	if n == nil {
		return enc, nil
	}

	// encode hash and arrival time
	enc = append(enc, n.hash[:]...)
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, n.arrivalTime)
	enc = append(enc, buf...)

	binary.LittleEndian.PutUint64(buf, uint64(len(n.children)))
	enc = append(enc, buf...)

	var err error
	for _, child := range n.children {
		enc, err = encodeRecursive(child, enc)
		if err != nil {
			return nil, err
		}
	}

	return enc, nil
}

// Decode recursively decodes an encoded block tree
func (bt *BlockTree) Decode(in []byte) error {
	r := &bytes.Buffer{}
	_, err := r.Write(in)
	if err != nil {
		return err
	}

	hash, err := common.ReadHash(r)
	if err != nil {
		return err
	}
	arrivalTime, err := common.ReadUint64(r)
	if err != nil {
		return err
	}
	numChildren, err := common.ReadUint64(r)
	if err != nil {
		return err
	}

	bt.head = &node{
		hash:        hash,
		parent:      nil,
		children:    make([]*node, numChildren),
		depth:       big.NewInt(0),
		arrivalTime: arrivalTime,
	}

	bt.leaves = newLeafMap(bt.head)

	return bt.decodeRecursive(r, bt.head)
}

// decode recursively decodes the blocktree
func (bt *BlockTree) decodeRecursive(r io.Reader, parent *node) error {
	for i := range parent.children {
		hash, err := common.ReadHash(r)
		if err != nil {
			return err
		}
		arrivalTime, err := common.ReadUint64(r)
		if err != nil {
			return err
		}
		numChildren, err := common.ReadUint64(r)
		if err != nil {
			return err
		}

		parent.children[i] = &node{
			hash:        hash,
			parent:      parent,
			children:    make([]*node, numChildren),
			depth:       big.NewInt(0).Add(parent.depth, big.NewInt(1)),
			arrivalTime: arrivalTime,
		}

		bt.leaves.replace(parent, parent.children[i])

		err = bt.decodeRecursive(r, parent.children[i])
		if err != nil {
			return err
		}
	}

	return nil
}
