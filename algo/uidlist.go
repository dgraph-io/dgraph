package algo

import (
	"container/heap"
	"sort"

	"github.com/dgraph-io/dgraph/task"
)

type ListIterator struct {
	list *task.List
	bIdx int // Block index
	idx  int // List index
}

func NewListIterator(l *task.List) ListIterator {
	return ListIterator{
		list: l,
		bIdx: 0,
		idx:  0,
	}
}

func (l *ListIterator) Valid() bool {
	if l == nil || l.list.Blocks == nil || len(l.list.Blocks) == 0 {
		return false
	}
	if l.bIdx >= len(l.list.Blocks) {
		return false
	}
	if l.list.Blocks[l.bIdx].List == nil || l.idx >= len(l.list.Blocks[l.bIdx].List) {
		return false
	}
	return true
}

func (l *ListIterator) Val() uint64 {
	if !l.Valid() {
		return 0
	}
	return l.list.Blocks[l.bIdx].List[l.idx]
}

func (l *ListIterator) Next() bool {
	if !l.Valid() {
		return false
	}
	l.idx++
	if l.idx >= len(l.list.Blocks[l.bIdx].List) {
		l.idx = 0
		l.bIdx++
		if l.bIdx >= len(l.list.Blocks) {
			return false
		}
	}
	return true
}

func SortedListToBlock(l []uint64) *task.List {
	b := new(task.List)
	b.Blocks = make([]*task.Block, 1, 2)
	bIdx := 0
	b.Blocks[0] = new(task.Block)
	b.Blocks[0].List = make([]uint64, 0, 100)

	if len(l) == 0 {
		return b
	}

	for _, it := range l {
		if len(b.Blocks[bIdx].List) > 100 {
			b.Blocks[bIdx].MaxInt = b.Blocks[bIdx].List[100]
			b.Blocks = append(b.Blocks, &task.Block{List: make([]uint64, 0, 100)})
			bIdx++
		}
		b.Blocks[bIdx].List = append(b.Blocks[bIdx].List, it)
	}

	b.Blocks[bIdx].MaxInt = b.Blocks[bIdx].List[len(b.Blocks[bIdx].List)-1]
	return b
}

func BlockToList(b *task.List) []uint64 {
	var res []uint64
	for _, it := range b.Blocks {
		for _, el := range it.List {
			res = append(res, el)
		}
	}
	return res
}

func ListLen(l *task.List) int {
	var n int
	if l == nil || l.Blocks == nil {
		return 0
	}

	for _, it := range l.Blocks {
		n += len(it.List)
	}
	return n
}

func IntersectWith(u, v *task.List) {
	out := u.Blocks

	i := 0
	j := 0

	ii := 0
	jj := 0
	kk := 0

	m := len(u.Blocks)
	n := len(v.Blocks)

	for i < m && j < n {

		ulist := u.Blocks[i].List
		vlist := v.Blocks[j].List
		ub := u.Blocks[i].MaxInt
		vb := v.Blocks[j].MaxInt
		ulen := len(u.Blocks[i].List)
		vlen := len(v.Blocks[j].List)

	L:
		for ii < ulen && jj < vlen {
			uid := ulist[ii]
			vid := vlist[jj]

			if uid == vid {
				out[i].List[kk] = uid
				kk++
				ii++
				jj++
				if ii == ulen {
					out[i].List = out[i].List[:kk]
					i++
					ii = 0
					kk = 0
					break L
				}
				if jj == vlen {
					j++
					jj = 0
					break L
				}
			} else if ub < vid {
				out[i].List = out[i].List[:kk]
				i++
				ii = 0
				kk = 0
				break L
			} else if vb < uid {
				j++
				jj = 0
				break L
			} else if uid < vid {
				for ; ii < ulen && ulist[ii] < vid; ii++ {
				}
				if ii == ulen {
					out[i].List = out[i].List[:kk]
					i++
					ii = 0
					kk = 0
					break L
				}
			} else if uid > vid {
				for ; jj < vlen && vlist[jj] < uid; jj++ {
				}
				if jj == vlen {
					j++
					jj = 0
					break L
				}
			}
		}

		if ii == ulen {
			out[i].List = out[i].List[:kk]
			i++
			ii = 0
			kk = 0
		}
		if jj == vlen {
			j++
			jj = 0
		}
	}

	if i < m {
		out = out[:i+1]
		out[i].List = out[i].List[:kk]
	}
}

// ApplyFilter applies a filter to our UIDList.
func ApplyFilter(u *task.List, f func(uint64, int) bool) {
	out := u.Blocks
	for i, block := range u.Blocks {
		kk := 0
		for j, uid := range block.List {
			if f(uid, Idx(u, i, j)) {
				out[i].List[kk] = uid
				kk++
			}
		}
		out[i].List = out[i].List[:kk]
	}
}

// IntersectSorted intersect a list of UIDLists. An alternative is to do
// pairwise intersections n-1 times where n=number of lists. This is less
// efficient:
// Let p be length of shortest list. Let q be average length of lists. So
// nq = total number of elements.
// There are many possible cases. Consider the case where the shortest list
// is the answer (or close to the answer). The following method requires nq
// reads (each element is read only once) whereas pairwise intersections can
// require np + nq - p reads, which can be up to ~twice as many.
func IntersectSorted(lists []*task.List) *task.List {
	if len(lists) == 0 {
		return new(task.List)
	}

	// Scan through the smallest list. Denote as A.
	// For each x in A,
	//   For each other list B,
	//     Keep popping elements until we get a y >= x.
	//     If y > x, mark x as "skipped". Break out of loop.
	//   If x is not marked as "skipped", append x to result.
	var minLenIdx int
	minLen := ListLen(lists[0])
	for i := 1; i < len(lists); i++ { // Start from 1.
		l := lists[i]
		n := ListLen(l)
		if n < minLen {
			minLen = n
			minLenIdx = i
		}
	}

	// Our final output. Give it some capacity.
	output := make([]uint64, 0, minLen)
	// lptrs[j] is the element we are looking at for lists[j].
	lptrs := make([]ListIterator, len(lists))
	for i, l := range lists {
		lptrs[i] = NewListIterator(l)
	}
	shortListIt := lptrs[minLenIdx]
	elemsLeft := true // If some list has no elems left, we can't intersect more.

	for ; shortListIt.Valid() && elemsLeft; shortListIt.Next() { //for i := 0; i < len(shortList.Uids) && elemsLeft; i++ {
		val := shortListIt.Val()
		// We dont need this
		/*
			if i > 0 && val == shortList.Uids[i-1] {
				x.AssertTruef(false, "We shouldn't have duplicates in UIDLists")
			}
		*/

		var skip bool                     // Should we skip val in output?
		for j := 0; j < len(lists); j++ { // For each other list in lists.
			if j == minLenIdx {
				// No point checking yourself.
				continue
			}

			for ; lptrs[j].Valid() && lptrs[j].Val() < val; lptrs[j].Next() {
			}

			if !lptrs[j].Valid() || lptrs[j].Val() > val {
				elemsLeft = lptrs[j].Valid()
				skip = true
				break
			}
			// Otherwise, lj.Get(ljp) = val and we continue checking other lists.
		}
		if !skip {
			output = append(output, val)
		}
	}
	return SortedListToBlock(output)
}

func Difference(u, v *task.List) {
	out := u.Blocks

	i := 0
	j := 0

	ii := 0
	jj := 0
	kk := 0

	m := len(u.Blocks)
	n := len(v.Blocks)

	for i < m && j < n {

		ulist := u.Blocks[i].List
		vlist := v.Blocks[j].List
		ub := u.Blocks[i].MaxInt
		vb := v.Blocks[j].MaxInt
		ulen := len(u.Blocks[i].List)
		vlen := len(v.Blocks[j].List)

	L:
		for ii < ulen && jj < vlen {
			uid := ulist[ii]
			vid := vlist[jj]

			if uid == vid {
				ii++
				jj++
				if ii == ulen {
					out[i].List = out[i].List[:kk]
					i++
					ii = 0
					kk = 0
					break L
				}
				if jj == vlen {
					j++
					jj = 0
					break L
				}
			} else if ub < vid {
				i++
				ii = 0
				kk = 0
				break L
			} else if vb < uid {
				j++
				jj = 0
				break L
			} else if uid < vid {
				for ; ii < ulen && ulist[ii] < vid; ii++ {
					out[i].List[kk] = ulist[ii]
					kk++
				}
				if ii == ulen {
					out[i].List = out[i].List[:kk]
					i++
					ii = 0
					kk = 0
					break L
				}
			} else if uid > vid {
				for ; jj < vlen && vlist[jj] < uid; jj++ {
				}
				if jj == vlen {
					j++
					jj = 0
					break L
				}
			}
		}

		if ii == ulen {
			out[i].List = out[i].List[:kk]
			i++
			ii = 0
			kk = 0
		}
		if jj == vlen {
			j++
			jj = 0
		}
	}

	if i < m {
		out = out[:i+1]
		out[i].List = out[i].List[:kk]
	}
}

// MergeSorted merges sorted lists.
func MergeSorted(lists []*task.List) *task.List {
	if len(lists) == 0 {
		return new(task.List)
	}

	h := &uint64Heap{}
	heap.Init(h)
	var lIt []ListIterator
	for i, l := range lists {
		it := NewListIterator(l)
		lIt = append(lIt, it)
		if it.Valid() {
			heap.Push(h, elem{
				val:     it.Val(),
				listIdx: i,
			})
		}
	}

	// Our final output. Give it some capacity.
	output := make([]uint64, 0, 100)
	var last uint64   // Last element added to sorted / final output.
	for h.Len() > 0 { // While heap is not empty.
		me := (*h)[0] // Peek at the top element in heap.
		if len(output) == 0 || me.val != last {
			output = append(output, me.val) // Add if unique.
			last = me.val
		}
		if !lIt[me.listIdx].Next() {
			heap.Pop(h)
		} else {
			val := lIt[me.listIdx].Val()
			(*h)[0].val = val
			heap.Fix(h, 0) // Faster than Pop() followed by Push().
		}
	}
	return SortedListToBlock(output)
}

// IndexOf performs a binary search on the uids slice and returns the index at
// which it finds the uid, else returns -1
func IndexOf(u *task.List, uid uint64) (int, int) {
	i := sort.Search(len(u.Blocks), func(i int) bool { return u.Blocks[i].MaxInt >= uid })
	if i < len(u.Blocks) {
		j := sort.Search(len(u.Blocks[i].List), func(j int) bool { return u.Blocks[i].List[j] >= uid })
		if j < len(u.Blocks) && u.Blocks[i].List[j] == uid {
			return i, j
		}
	}
	return -1, -1
}

func Idx(ul *task.List, i, j int) int {
	res := 0
	for k := 0; k < i; k++ {
		res += len(ul.Blocks[k].List)
	}
	return res + j
}

// ToUintsListForTest converts to list of uints for testing purpose only.
func ToUintsListForTest(ul []*task.List) [][]uint64 {
	out := make([][]uint64, 0, len(ul))
	for _, u := range ul {
		out = append(out, BlockToList(u))
	}
	return out
}
