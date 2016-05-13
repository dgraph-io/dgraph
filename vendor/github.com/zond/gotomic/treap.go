package gotomic

import (
	"bytes"
	"fmt"
	"math/rand"
	"sync/atomic"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

type match struct {
	previousOk    bool
	previousKey   Comparable
	previousValue Thing
	matchOk       bool
	matchKey      Comparable
	matchValue    Thing
	nextOk        bool
	nextKey       Comparable
	nextValue     Thing
}

type TreapIterator func(k Comparable, v Thing)

/*
 Transaction controlled treap
*/
type treap struct {
	root *nodeHandle
}

func (self *treap) Clone() Clonable {
	return &treap{self.root}
}

func merge(t *Transaction, left, right *nodeHandle) (result *nodeHandle, err error) {
	if left == nil {
		result = right
		return
	}
	if right == nil {
		result = left
		return
	}
	var leftNode, rightNode *node
	var subMerge *nodeHandle
	if left.weight < right.weight {
		leftNode, err = left.wopen(t)
		if err != nil {
			return
		}
		result = left
		tmp := leftNode.right
		subMerge, err = merge(t, tmp, right)
		if err != nil {
			return
		}
		leftNode.right = subMerge
		return
	}
	rightNode, err = right.wopen(t)
	if err != nil {
		return
	}
	result = right
	tmp := rightNode.left
	subMerge, err = merge(t, left, tmp)
	if err != nil {
		return
	}
	rightNode.left = subMerge
	return
}

/*
 Non-transaction controlled "user space" type
*/
type Treap struct {
	handle *Handle
	size   int64
}

func NewTreap() *Treap {
	return &Treap{NewHandle(&treap{}), 0}
}

/*
 Get a readable *treap from the Treap
*/
func (self *Treap) ropen(t *Transaction) (*treap, error) {
	r, err := t.Read(self.handle)
	if err != nil {
		return nil, err
	}
	return r.(*treap), nil
}

/*
 Get a writable *treap from the Treap
*/
func (self *Treap) wopen(t *Transaction) (*treap, error) {
	r, err := t.Write(self.handle)
	if err != nil {
		return nil, err
	}
	return r.(*treap), nil
}
func (treap *Treap) Describe() string {
	rval, err := treap.describe()
	for err != nil {
		rval, err = treap.describe()
	}
	return rval
}
func (treap *Treap) describe() (rval string, err error) {
	t := NewTransaction()
	self, err := treap.ropen(t)
	if err != nil {
		return
	}
	buf := bytes.NewBufferString(fmt.Sprintf("&Treap{%p size:%v}\n", treap, treap.size))
	if self.root != nil {
		err = self.root.describe(t, buf, 0)
		if err != nil {
			return
		}
	}
	return string(buf.Bytes()), nil
}
func (treap *Treap) Delete(k Comparable) (old Thing, ok bool) {
	old, ok, err := treap.del(k)
	for err != nil {
		old, ok, err = treap.del(k)
	}
	return
}
func (treap *Treap) del(k Comparable) (old Thing, ok bool, err error) {
	t := NewTransaction()
	self, err := treap.ropen(t)
	if err != nil {
		return
	}
	if self.root == nil {
		ok = false
		return
	}
	newRoot, old, ok, err := self.root.del(t, k)
	if err != nil {
		return
	}
	if newRoot != self.root {
		self, err = treap.wopen(t)
		if err != nil {
			return
		}
		self.root = newRoot
	}
	if t.Commit() {
		atomic.AddInt64(&treap.size, -1)
	} else {
		err = fmt.Errorf("%v changed during delete", treap)
	}
	return
}
func (treap *Treap) Put(k Comparable, v Thing) (old Thing, ok bool) {
	old, ok, err := treap.put(k, v)
	for err != nil {
		old, ok, err = treap.put(k, v)
	}
	return
}
func (treap *Treap) ToSlice() (keys []Comparable, values []Thing) {
	iter := func(k Comparable, v Thing) {
		keys = append(keys, k)
		values = append(values, v)
	}
	err := treap.Each(iter)
	for err != nil {
		keys = nil
		values = nil
		err = treap.Each(iter)
	}
	return
}
func (treap *Treap) Each(iter TreapIterator) (err error) {
	t := NewTransaction()
	self, err := treap.ropen(t)
	if err != nil {
		return
	}
	if self.root == nil {
		return
	}
	err = self.root.each(t, iter)
	return
}
func (treap *Treap) Next(k Comparable) (key Comparable, value Thing, ok bool) {
	key, value, ok, err := treap.next(k)
	for err != nil {
		key, value, ok, err = treap.next(k)
	}
	return
}
func (treap *Treap) next(k Comparable) (key Comparable, value Thing, ok bool, err error) {
	t := NewTransaction()
	self, err := treap.ropen(t)
	if err != nil {
		return
	}
	if self.root == nil {
		ok = true
		return
	}
	m := &match{}
	err = self.root.get(t, k, m, false, true)
	key = m.nextKey
	value = m.nextValue
	ok = m.nextOk
	return
}
func (treap *Treap) Previous(k Comparable) (key Comparable, value Thing, ok bool) {
	key, value, ok, err := treap.previous(k)
	for err != nil {
		key, value, ok, err = treap.previous(k)
	}
	return
}
func (treap *Treap) previous(k Comparable) (key Comparable, value Thing, ok bool, err error) {
	t := NewTransaction()
	self, err := treap.ropen(t)
	if err != nil {
		return
	}
	if self.root == nil {
		ok = true
		return
	}
	m := &match{}
	err = self.root.get(t, k, m, true, false)
	key = m.previousKey
	value = m.previousValue
	ok = m.previousOk
	return
}
func (treap *Treap) Get(k Comparable) (v Thing, ok bool) {
	v, ok, err := treap.get(k)
	for err != nil {
		v, ok, err = treap.get(k)
	}
	return
}
func (treap *Treap) get(k Comparable) (v Thing, ok bool, err error) {
	t := NewTransaction()
	self, err := treap.ropen(t)
	if err != nil {
		return
	}
	if self.root == nil {
		return
	}
	m := &match{}
	err = self.root.get(t, k, m, false, false)
	v = m.matchValue
	ok = m.matchOk
	return
}
func (treap *Treap) Min() (k Comparable, v Thing, ok bool) {
	k, v, ok, err := treap.min()
	for err != nil {
		k, v, ok, err = treap.min()
	}
	return
}
func (treap *Treap) min() (k Comparable, v Thing, ok bool, err error) {
	t := NewTransaction()
	self, err := treap.ropen(t)
	if err != nil {
		return
	}
	if self.root == nil {
		ok = false
		return
	}
	ok = true
	k, v, err = self.root.min(t)
	return
}
func (treap *Treap) Max() (k Comparable, v Thing, ok bool) {
	k, v, ok, err := treap.max()
	for err != nil {
		k, v, ok, err = treap.max()
	}
	return
}
func (treap *Treap) max() (k Comparable, v Thing, ok bool, err error) {
	t := NewTransaction()
	self, err := treap.ropen(t)
	if err != nil {
		return
	}
	if self.root == nil {
		ok = false
		return
	}
	ok = true
	k, v, err = self.root.max(t)
	return
}
func (treap *Treap) put(k Comparable, v Thing) (old Thing, ok bool, err error) {
	t := NewTransaction()
	self, err := treap.ropen(t)
	if err != nil {
		return
	}
	newNode := newNodeHandle(k, v)
	newRoot, old, ok, err := self.root.insert(t, newNode)
	if err != nil {
		return
	}
	if newRoot != self.root {
		self, err = treap.wopen(t)
		if err != nil {
			return
		}
		self.root = newRoot
	}
	if t.Commit() {
		atomic.AddInt64(&treap.size, 1)
	} else {
		err = fmt.Errorf("%v changed during put", treap)
	}
	return
}

type node struct {
	left  *nodeHandle
	right *nodeHandle
	value Thing
}

func (self *node) Clone() Clonable {
	rval := *self
	return &rval
}

type nodeHandle struct {
	*Handle
	key    Comparable
	weight int32
}

func (handle *nodeHandle) ropen(t *Transaction) (*node, error) {
	n, err := t.Read((*Handle)(handle.Handle))
	if err != nil {
		return nil, err
	}
	return n.(*node), nil
}
func (handle *nodeHandle) wopen(t *Transaction) (*node, error) {
	r, err := t.Write((*Handle)(handle.Handle))
	if err != nil {
		return nil, err
	}
	return r.(*node), nil
}
func (handle *nodeHandle) each(t *Transaction, iter TreapIterator) (err error) {
	if handle == nil {
		return
	}
	self, err := handle.ropen(t)
	if err != nil {
		return
	}
	err = self.left.each(t, iter)
	if err != nil {
		return
	}
	iter(handle.key, self.value)
	err = self.right.each(t, iter)
	return
}
func (handle *nodeHandle) get(t *Transaction, k Comparable, m *match, previous, next bool) (err error) {
	if handle == nil {
		return
	}
	self, err := handle.ropen(t)
	if err != nil {
		return
	}
	switch cmp := k.Compare(handle.key); {
	case cmp < 0:
		if next {
			m.nextKey = handle.key
			m.nextValue = self.value
			m.nextOk = true
		}
		err = self.left.get(t, k, m, previous, next)
	case cmp > 0:
		if previous {
			m.previousKey = handle.key
			m.previousValue = self.value
			m.previousOk = true
		}
		err = self.right.get(t, k, m, previous, next)
	default:
		m.matchKey = handle.key
		m.matchValue = self.value
		m.matchOk = true
		if previous {
			if self.left != nil {
				m.previousKey, m.previousValue, err = self.left.max(t)
				if err != nil {
					return
				}
				m.previousOk = true
			}
		}
		if next {
			if self.right != nil {
				m.nextKey, m.nextValue, err = self.right.min(t)
				if err != nil {
					return
				}
				m.nextOk = true
			}
		}
	}
	return
}
func (handle *nodeHandle) min(t *Transaction) (k Comparable, v Thing, err error) {
	self, err := handle.ropen(t)
	if err != nil {
		return
	}
	if self.left == nil {
		k = handle.key
		v = self.value
		return
	}
	return self.left.min(t)
}
func (handle *nodeHandle) max(t *Transaction) (k Comparable, v Thing, err error) {
	self, err := handle.ropen(t)
	if err != nil {
		return
	}
	if self.right == nil {
		k = handle.key
		v = self.value
		return
	}
	return self.right.max(t)
}
func (handle *nodeHandle) describe(t *Transaction, buf *bytes.Buffer, indent int) error {
	self, err := handle.ropen(t)
	if err != nil {
		return err
	}
	for i := 0; i < indent; i++ {
		fmt.Fprintf(buf, " ")
	}
	fmt.Fprintf(buf, "%v => %v (%v)\n", handle.key, self.value, handle.weight)
	if self.left != nil {
		fmt.Fprintf(buf, "l:")
		err = self.left.describe(t, buf, indent+1)
		if err != nil {
			return err
		}
	}
	if self.right != nil {
		fmt.Fprintf(buf, "r:")
		err = self.right.describe(t, buf, indent+1)
		if err != nil {
			return err
		}
	}
	return nil
}
func newNodeHandle(k Comparable, v Thing) *nodeHandle {
	return &nodeHandle{NewHandle(&node{nil, nil, v}), k, rand.Int31()}
}
func (handle *nodeHandle) rotateLeft(t *Transaction) (result *nodeHandle, err error) {
	self, err := handle.wopen(t)
	if err != nil {
		return
	}
	result = self.left
	resultNode, err := result.wopen(t)
	if err != nil {
		return
	}
	tmp := resultNode.right
	resultNode.right = handle
	self.left = tmp
	return
}
func (handle *nodeHandle) rotateRight(t *Transaction) (result *nodeHandle, err error) {
	self, err := handle.wopen(t)
	if err != nil {
		return
	}
	result = self.right
	resultNode, err := result.wopen(t)
	if err != nil {
		return
	}
	tmp := resultNode.left
	resultNode.left = handle
	self.right = tmp
	return
}
func (handle *nodeHandle) del(t *Transaction, k Comparable) (result *nodeHandle, old Thing, ok bool, err error) {
	if handle == nil {
		ok = false
		return
	}
	result = handle
	self, err := handle.ropen(t)
	if err != nil {
		return
	}
	switch cmp := k.Compare(handle.key); {
	case cmp < 0:
		var newLeft *nodeHandle
		newLeft, old, ok, err = self.left.del(t, k)
		if err != nil {
			return
		}
		if newLeft != self.left {
			self, err = handle.wopen(t)
			if err != nil {
				return
			}
			self.left = newLeft
		}
	case cmp > 0:
		var newRight *nodeHandle
		newRight, old, ok, err = self.right.del(t, k)
		if err != nil {
			return
		}
		if newRight != self.right {
			self, err = handle.wopen(t)
			if err != nil {
				return
			}
			self.right = newRight
		}
	default:
		ok = true
		old = self.value
		result, err = merge(t, self.left, self.right)
		if err != nil {
			return
		}
	}
	return
}
func (handle *nodeHandle) insert(t *Transaction, newHandle *nodeHandle) (result *nodeHandle, old Thing, ok bool, err error) {
	if handle == nil {
		ok = false
		result = newHandle
		return
	}
	result = handle
	self, err := handle.ropen(t)
	if err != nil {
		return
	}
	switch cmp := newHandle.key.Compare(handle.key); {
	case cmp < 0:
		var newLeft *nodeHandle
		newLeft, old, ok, err = self.left.insert(t, newHandle)
		if err != nil {
			return
		}
		if newLeft != self.left {
			self, err = handle.wopen(t)
			if err != nil {
				return
			}
			self.left = newLeft
			if newLeft.weight < handle.weight {
				result, err = handle.rotateLeft(t)
				if err != nil {
					return
				}
			}
		}
	case cmp > 0:
		var newRight *nodeHandle
		newRight, old, ok, err = self.right.insert(t, newHandle)
		if err != nil {
			return
		}
		if newRight != self.right {
			self, err = handle.wopen(t)
			if err != nil {
				return
			}
			self.right = newRight
			if newRight.weight < handle.weight {
				result, err = handle.rotateRight(t)
				if err != nil {
					return
				}
			}
		}
	default:
		if self, err = handle.wopen(t); err != nil {
			return
		}
		var newNode *node
		newNode, err = newHandle.ropen(t)
		if err != nil {
			return
		}
		old = self.value
		ok = true
		self.value = newNode.value
	}
	return
}
