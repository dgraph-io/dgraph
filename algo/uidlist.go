package algo

import (
	"container/heap"
	"sort"

	"github.com/dgraph-io/dgraph/task"
)

const blockSize = 100

// ListIterator is used to read through the task.List.
type ListIterator struct {
	list     *task.List
	curBlock *task.Block
	bidx     int  // Block index
	lidx     int  // List index
	isEnd    bool // To indicate reaching the end
}

// WriteIterator is used to append UIDs to a task.List.
type WriteIterator struct {
	list     *task.List
	curBlock *task.Block
	bidx     int // Block index
	lidx     int // List index

}

func NewWriteIterator(l *task.List, whence int) WriteIterator {
	blen := len(l.Blocks)
	var cur *task.Block
	if whence == 1 && blen != 0 {
		// Set the iterator to the end of the list.
		llen := len(l.Blocks[blen-1].List)
		cur = l.Blocks[blen-1]
		// Set the list to its actual size as we want to append.
		cur.List = cur.List[:blockSize]
		return WriteIterator{
			list:     l,
			curBlock: cur,
			bidx:     blen - 1,
			lidx:     llen,
		}
	}
	// Initialise and allocate some memory.
	if len(l.Blocks) == 0 {
		l.Blocks = make([]*task.Block, 0, 2)
	}

	if len(l.Blocks) > 0 {
		cur = l.Blocks[0]
	}
	return WriteIterator{
		list:     l,
		curBlock: cur,
		bidx:     0,
		lidx:     0,
	}
}

// Append appends an UID to the end of task.List following the blockSize specified.
func (l *WriteIterator) Append(uid uint64) {
	if l.lidx == blockSize {
		l.curBlock.MaxInt = l.curBlock.List[blockSize-1]
		l.lidx = 0
		l.bidx++
	}
	if l.bidx == len(l.list.Blocks) {
		// If we reached the end of blocks, add a new one.
		l.list.Blocks = append(l.list.Blocks, &task.Block{List: make([]uint64, blockSize)})
	}
	l.curBlock = l.list.Blocks[l.bidx]
	l.curBlock.List[l.lidx] = uid
	l.lidx++
}

// End is called after the write is over to update the MaxInt of the last block.
func (l *WriteIterator) End() {
	l.list.Blocks = l.list.Blocks[:l.bidx+1]
	if l.lidx == 0 {
		l.list.Blocks = l.list.Blocks[:l.bidx]
		return
	}
	l.curBlock.List = l.curBlock.List[:l.lidx]
	l.curBlock.MaxInt = l.curBlock.List[l.lidx-1]
}

// Seek seeks to the index whose value is greater than or equal to the given UID.
// It uses binary search to move around.
func Seek(ul *task.List, bidx, lidx int, uid uint64, typ int) (int, int) {
	i, ii := bidx, lidx
	if typ == 1 {
		// Seek the current list first.
		for ii < len(ul.Blocks[i].List) && ul.Blocks[i].List[ii] < uid {
			ii++
		}
		if ii < len(ul.Blocks[i].List) {
			return i, ii
		}

		// linear seek over blocks.
		for i < len(ul.Blocks) && uid > ul.Blocks[i].MaxInt {
			i++
		}
		if i == len(ul.Blocks) {
			return i, ii
		}
		ii = 0
		// Seek the current list first.
		for ii < len(ul.Blocks[i].List) && ul.Blocks[i].List[ii] < uid {
			ii++
		}
		return i, ii
	}
	// Binary seek.
	// TODO(Ashwin): Do a benchmark to see if linear scan is better than binary if whence is 1
	idx := sort.Search(len(ul.Blocks), func(i int) bool { return ul.Blocks[i].MaxInt >= uid })
	if idx >= len(ul.Blocks) {
		return idx, ii
	}
	j := sort.Search(len(ul.Blocks[idx].List), func(j int) bool { return ul.Blocks[idx].List[j] >= uid })
	return idx, j
}

// Slice returns a new task.List with the elements between start index and end index
// of  the list passed to it.
func Slice(ul *task.List, start, end int) {
	out := NewWriteIterator(ul, 0)
	i, ii := ridx(ul, start)
	if i < 0 {
		return
	}
	k := 0
	var stop bool
	blockLen := len(ul.Blocks)
	for ; i < blockLen; i++ {
		ulist := ul.Blocks[i].List
		listLen := len(ulist)
		for ; ii < listLen; ii++ {
			if k == end-start {
				stop = true
				break
			}
			out.Append(ulist[ii])
			k++
		}
		if stop {
			break
		}
		ii = 0
	}
	out.End()
}

func SortedListToBlock(l []uint64) *task.List {
	b := new(task.List)
	if len(l) == 0 {
		return b
	}
	wit := NewWriteIterator(b, 0)
	for _, it := range l {
		wit.Append(it)
	}

	wit.End()
	return b
}

func ListLen(l *task.List) int {
	if l == nil || len(l.Blocks) == 0 {
		return 0
	}

	n := len(l.Blocks) - 1
	length := n*blockSize + len(l.Blocks[n].List)
	return length
}

func IntersectWith(u, v *task.List) {
	i, ii := 0, 0 //itu := NewListIterator(u)
	j, jj := 0, 0 //itv := NewListIterator(v)
	out := NewWriteIterator(u, 0)
	m := len(u.Blocks)
	n := len(v.Blocks)
	for i < m && j < n {
		ulist := u.Blocks[i].List
		vlist := v.Blocks[j].List
		ubMax := u.Blocks[i].MaxInt
		vbMax := v.Blocks[j].MaxInt
		ulen := len(ulist)
		vlen := len(vlist)

		for ii < ulen && jj < vlen {
			uid := ulist[ii]
			vid := vlist[jj]

			if uid == vid {
				out.Append(uid)
				ii++
				jj++
			} else if ubMax < vid {
				ii = ulen
			} else if vbMax < uid {
				jj = vlen
			} else if uid < vid {
				for ; ii < ulen && ulist[ii] < vid; ii++ {
				}
			} else if uid > vid {
				for ; jj < vlen && vlist[jj] < uid; jj++ {
				}
			}
		}
		if ii == ulen {
			i++
			ii = 0
		}
		if jj == vlen {
			j++
			jj = 0
		}
	}
	out.End()
}

func Difference(u, v *task.List) {
	i, ii := 0, 0 //itu := NewListIterator(u)
	j, jj := 0, 0 //itv := NewListIterator(v)
	out := NewWriteIterator(u, 0)
	m := len(u.Blocks)
	n := len(v.Blocks)
	for i < m && j < n {
		ulist := u.Blocks[i].List
		vlist := v.Blocks[j].List
		vbMax := v.Blocks[j].MaxInt
		ulen := len(ulist)
		vlen := len(vlist)

		for ii < ulen && jj < vlen {
			uid := ulist[ii]
			vid := vlist[jj]

			if uid == vid {
				ii++
				jj++
			} else if vbMax < uid {
				jj = vlen
			} else if uid < vid {
				out.Append(uid)
				ii++
			} else if uid > vid {
				for ; jj < vlen && vlist[jj] < uid; jj++ {
				}
			}
		}
		if ii == ulen {
			i++
			ii = 0
		}
		if jj == vlen {
			j++
			jj = 0
		}
	}
	out.End()
}

// ApplyFilter applies a filter to our UIDList.
func ApplyFilter(u *task.List, f func(uint64, int) bool) {
	out := NewWriteIterator(u, 0)
	for i, block := range u.Blocks {
		for j, uid := range block.List {
			if f(uid, Idx(u, i, j)) {
				out.Append(uid)
			}
		}
	}
	out.End()
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
	o := new(task.List)
	if len(lists) == 0 {
		return o
	}
	out := NewWriteIterator(o, 0)
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

	// lptrs[j] is the element we are looking at for lists[j].
	lptrs := make([]int, len(lists))
	bptrs := make([]int, len(lists))
	for i := range lists {
		lptrs[i] = 0 //NewListIterator(l)
		bptrs[i] = 0
	}
	shortList := lists[minLenIdx]
	elemsLeft := true // If some list has no elems left, we can't intersect more.
	blen := len(lists[minLenIdx].Blocks)
	for i := 0; i < blen; i++ {
		ulist := shortList.Blocks[i].List
		llen := len(ulist)
		for ii := 0; ii < llen; ii++ {
			val := ulist[ii]
			var skip bool                     // Should we skip val in output?
			for j := 0; j < len(lists); j++ { // For each other list in lists.
				if j == minLenIdx {
					// No point checking yourself.
					continue
				}

				k, kk := bptrs[j], lptrs[j]
				plen := len(lists[j].Blocks)
				for k < plen {
					qlist := lists[j].Blocks[k].List
					qlen := len(qlist)
					for ; kk < qlen && qlist[kk] < val; kk++ {
					}
					if kk == qlen {
						kk = 0
						k++
					}
					if qlist[kk] >= val {
						break
					}
				}
				bptrs[j], lptrs[j] = k, kk
				if k == plen || lists[j].Blocks[k].List[kk] > val {
					elemsLeft = k < plen //lists[j].Blocks[k].List[kk]
					skip = true
					break
				}
				if !elemsLeft {
					break
				}
				// Otherwise, lj.Get(ljp) = val and we continue checking other lists.
			}
			if !skip {
				out.Append(val)
			}
			if !elemsLeft {
				break
			}
		}
	}
	out.End()
	return o
}

// MergeSorted merges sorted lists.
func MergeSorted(lists []*task.List) *task.List {
	o := new(task.List)
	if len(lists) == 0 {
		return o
	}
	out := NewWriteIterator(o, 0)
	h := &uint64Heap{}
	heap.Init(h)
	var lIt []int // ListIterator
	var bIt []int // ListIterator
	for i, l := range lists {
		lIt = append(lIt, 0)
		bIt = append(bIt, 0)
		if len(l.Blocks) > 0 && len(l.Blocks[0].List) > 0 {
			heap.Push(h, elem{
				val:     l.Blocks[0].List[0],
				listIdx: i,
			})
		}
	}

	var last uint64   // Last element added to sorted / final output.
	for h.Len() > 0 { // While heap is not empty.
		me := (*h)[0] // Peek at the top element in heap.
		if (out.lidx == 0 && out.bidx == 0) || me.val != last {
			out.Append(me.val) // Add if unique.
			last = me.val
		}
		lIt[me.listIdx]++
		if lIt[me.listIdx] == len(lists[me.listIdx].Blocks[bIt[me.listIdx]].List) {
			bIt[me.listIdx]++
			lIt[me.listIdx] = 0
		}
		if bIt[me.listIdx] >= len(lists[me.listIdx].Blocks) || lIt[me.listIdx] >= len(lists[me.listIdx].Blocks[0].List) {
			//if !lIt[me.listIdx].Valid() {
			heap.Pop(h)
		} else {
			val := lists[me.listIdx].Blocks[bIt[me.listIdx]].List[lIt[me.listIdx]] // lIt[me.listIdx].Val()
			(*h)[0].val = val
			heap.Fix(h, 0) // Faster than Pop() followed by Push().
		}
	}
	out.End()
	return o
}

// IndexOf performs a binary search on the uids slice and returns the index at
// which it finds the uid, else returns -1
func IndexOf(u *task.List, uid uint64) (int, int) {
	i := sort.Search(len(u.Blocks), func(i int) bool { return u.Blocks[i].MaxInt >= uid })
	if i < len(u.Blocks) {
		j := sort.Search(len(u.Blocks[i].List), func(j int) bool { return u.Blocks[i].List[j] >= uid })
		if j < len(u.Blocks[i].List) && u.Blocks[i].List[j] == uid {
			return i, j
		}
	}
	return -1, -1
}

func Swap(ul *task.List, i, j int) {
	i1, i2 := ridx(ul, i)
	j1, j2 := ridx(ul, j)
	ul.Blocks[i1].List[i2], ul.Blocks[j1].List[j2] = ul.Blocks[j1].List[j2], ul.Blocks[i1].List[i2]
}

func ItemAtIndex(ul *task.List, i int) uint64 {
	i, j := ridx(ul, i)
	return ul.Blocks[i].List[j]
}

func ridx(ul *task.List, i int) (int, int) {
	if i >= ListLen(ul) {
		return -1, -1
	}

	return i / blockSize, i % blockSize
}

func Idx(ul *task.List, i, j int) int {
	return i*blockSize + j
}

// ToUintsListForTest converts to list of uints for testing purpose only.
func ToUintsListForTest(ul []*task.List) [][]uint64 {
	out := make([][]uint64, 0, len(ul))
	for _, u := range ul {
		out = append(out, BlockToList(u))
	}
	return out
}

func BlockToList(b *task.List) []uint64 {
	res := make([]uint64, 0, 5)
	for _, it := range b.Blocks {
		for _, el := range it.List {
			res = append(res, el)
		}
	}
	return res
}
