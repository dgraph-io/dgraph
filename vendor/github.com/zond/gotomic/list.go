package gotomic

import (
	"bytes"
	"fmt"
	"sync/atomic"
	"unsafe"
)

var deletedElement = "deleted"

type ListIterator func(t Thing) bool

type hit struct {
	left    *element
	element *element
	right   *element
}

func (self *hit) String() string {
	return fmt.Sprintf("&hit{%v,%v,%v}", self.left.val(), self.element.val(), self.right.val())
}

/*
 Comparable types can be kept sorted in a List.
*/
type Comparable interface {
	Compare(Thing) int
}

type Thing interface{}

var list_head = "LIST_HEAD"

/*
 List is a singly linked list based on "A Pragmatic Implementation of Non-Blocking Linked-Lists" by Timothy L. Harris <http://www.timharris.co.uk/papers/2001-disc.pdf>

 It is thread safe and non-blocking, and supports ordered elements by using List#inject with values implementing Comparable.
*/
type List struct {
	*element
	size int64
}

func NewList() *List {
	return &List{&element{nil, &list_head}, 0}
}

/*
 Push adds t to the top of the List.
*/
func (self *List) Push(t Thing) {
	self.element.add(t)
	atomic.AddInt64(&self.size, 1)
}

/*
 Pop removes and returns the top of the List.
*/
func (self *List) Pop() (rval Thing, ok bool) {
	if rval, ok := self.element.remove(); ok {
		atomic.AddInt64(&self.size, -1)
		return rval, true
	}
	return nil, false
}

/*
 Each will run i on each element.

 It returns true if the iteration was interrupted.
 This is the case when one of the ListIterator calls returned true, indicating
 the iteration should be stopped.
*/
func (self *List) Each(i ListIterator) bool {
	n := self.element.next()
	return n != nil && n.each(i)
}

func (self *List) String() string {
	return fmt.Sprint(self.ToSlice())
}

/*
 ToSlice returns a []Thing that is logically identical to the List.
*/
func (self *List) ToSlice() []Thing {
	return self.element.next().ToSlice()
}

/*
 Search return the first element in the list that matches c (c.Compare(element) == 0)
*/
func (self *List) Search(c Comparable) Thing {
	if hit := self.element.search(c); hit.element != nil {
		return hit.element.val()
	}
	return nil
}
func (self *List) Size() int {
	return int(atomic.LoadInt64(&self.size))
}

/*
 Inject c into the List at the first place where it is <= to all elements before it.
*/
func (self *List) Inject(c Comparable) {
	self.element.inject(c)
	atomic.AddInt64(&self.size, 1)
}

type element struct {
	/*
	 The next element in the list. If this pointer has the deleted flag set it means THIS element, not the next one, is deleted.
	*/
	Pointer unsafe.Pointer
	value   Thing
}

func (self *element) next() *element {
	next := atomic.LoadPointer(&self.Pointer)
	for next != nil {
		nextElement := (*element)(next)
		/*
		 If our next element contains &deletedElement that means WE are deleted, and
		 we can just return the next-next element. It will make it impossible to add
		 stuff to us, since we will always lie about our next(), but then again, deleted
		 elements shouldn't get new children anyway.
		*/
		if sp, ok := nextElement.value.(*string); ok && sp == &deletedElement {
			return nextElement.next()
		}
		/*
		 If our next element is itself deleted (by the same criteria) then we will just replace
		 it with its next() (which should be the first thing behind it that isn't itself deleted
		 (the power of recursion compels you) and then check again.
		*/
		if nextElement.isDeleted() {
			atomic.CompareAndSwapPointer(&self.Pointer, next, unsafe.Pointer(nextElement.next()))
			next = atomic.LoadPointer(&self.Pointer)
		} else {
			/*
			 If it isn't deleted then we just return it.
			*/
			return nextElement
		}
	}
	/*
	 And if our next is nil, then we are at the end of the list and can just return nil for next()
	*/
	return nil
}

func (self *element) each(i ListIterator) bool {
	n := self

	for n != nil {
		if i(n.value) {
			return true
		}
		n = n.next()
	}

	return false
}

func (self *element) val() Thing {
	if self == nil {
		return nil
	}
	return self.value
}
func (self *element) String() string {
	return fmt.Sprint(self.ToSlice())
}
func (self *element) Describe() string {
	if self == nil {
		return fmt.Sprint(nil)
	}
	deleted := ""
	if sp, ok := self.value.(*string); ok && sp == &deletedElement {
		deleted = " (x)"
	}
	return fmt.Sprintf("%#v%v -> %v", self, deleted, self.next().Describe())
}
func (self *element) isDeleted() bool {
	next := atomic.LoadPointer(&self.Pointer)
	if next == nil {
		return false
	}
	if sp, ok := (*element)(next).value.(*string); ok && sp == &deletedElement {
		return true
	}
	return false
}
func (self *element) add(c Thing) (rval bool) {
	alloc := &element{}
	for {
		/*
		 If we are deleted then we do not allow adding new children.
		*/
		if self.isDeleted() {
			break
		}
		/*
		 If we succeed in adding before our perceived next, just return true.
		*/
		if self.addBefore(c, alloc, self.next()) {
			rval = true
			break
		}
	}
	return
}
func (self *element) addBefore(t Thing, allocatedElement, before *element) bool {
	if self.next() != before {
		return false
	}
	allocatedElement.value = t
	allocatedElement.Pointer = unsafe.Pointer(before)
	return atomic.CompareAndSwapPointer(&self.Pointer, unsafe.Pointer(before), unsafe.Pointer(allocatedElement))
}

/*
 inject c into self either before the first matching value (c.Compare(value) == 0), before the first value
 it should be before (c.Compare(value) < 0) or after the first value it should be after (c.Compare(value) > 0).
*/
func (self *element) inject(c Comparable) {
	alloc := &element{}
	for {
		hit := self.search(c)
		if hit.left != nil {
			if hit.element != nil {
				if hit.left.addBefore(c, alloc, hit.element) {
					break
				}
			} else {
				if hit.left.addBefore(c, alloc, hit.right) {
					break
				}
			}
		} else if hit.element != nil {
			if hit.element.addBefore(c, alloc, hit.right) {
				break
			}
		} else {
			panic(fmt.Errorf("Unable to inject %v properly into %v, it ought to be first but was injected into the first element of the list!", c, self))
		}
	}
}
func (self *element) ToSlice() []Thing {
	rval := make([]Thing, 0)
	current := self
	for current != nil {
		rval = append(rval, current.value)
		current = current.next()
	}
	return rval
}

/*
 search for c in self.

 Will stop searching when finding nil or an element that should be after c (c.Compare(element) < 0).

 Will return a hit containing the last elementRef and element before a match (if no match, the last elementRef and element before
 it stops searching), the elementRef and element for the match (if a match) and the last elementRef and element after the match
 (if no match, the first elementRef and element, or nil/nil if at the end of the list).
*/
func (self *element) search(c Comparable) (rval *hit) {
	rval = &hit{nil, self, nil}
	for {
		if rval.element == nil {
			return
		}
		rval.right = rval.element.next()
		if rval.element.value != &list_head {
			switch cmp := c.Compare(rval.element.value); {
			case cmp < 0:
				rval.right = rval.element
				rval.element = nil
				return
			case cmp == 0:
				return
			}
		}
		rval.left = rval.element
		rval.element = rval.left.next()
		rval.right = nil
	}
	panic(fmt.Sprint("Unable to search for ", c, " in ", self))
}

/*
 Verify that all Comparable values in this list are after values they should be after (c.Compare(last) >= 0).
*/
func (self *element) verify() (err error) {
	current := self
	var last Thing
	var bad [][]Thing
	seen := make(map[*element]bool)
	for current != nil {
		if _, ok := seen[current]; ok {
			return fmt.Errorf("%#v is circular!", self)
		}
		value := current.value
		if last != &list_head {
			if comp, ok := value.(Comparable); ok {
				if comp.Compare(last) < 0 {
					bad = append(bad, []Thing{last, value})
				}
			}
		}
		seen[current] = true
		last = value
		current = current.next()
	}
	if len(bad) == 0 {
		return nil
	}
	buffer := new(bytes.Buffer)
	for index, pair := range bad {
		fmt.Fprint(buffer, pair[0], ",", pair[1])
		if index < len(bad)-1 {
			fmt.Fprint(buffer, "; ")
		}
	}
	return fmt.Errorf("%v is badly ordered. The following elements are in the wrong order: %v", self, string(buffer.Bytes()))

}

/*
 Just a shorthand to hide the inner workings of our removal mechanism.
*/
func (self *element) doRemove() bool {
	return self.add(&deletedElement)
}
func (self *element) remove() (rval Thing, ok bool) {
	n := self.next()
	for {
		/*
		 No children to remove.
		*/
		if n == nil {
			break
		}
		/*
		 We managed to remove next!
		*/
		if n.doRemove() {
			self.next()
			rval = n.value
			ok = true
			break
		}
		n = self.next()
	}
	return
}
