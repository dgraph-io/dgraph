package algo

import "github.com/dgraph-io/dgraph/task"

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
				for ; ii < ulen && u.Blocks[i].List[ii] < vid; ii++ {
				}
				if ii == ulen {
					out[i].List = out[i].List[:kk]
					i++
					ii = 0
					kk = 0
					break L
				}
			} else if uid > vid {
				for ; jj < vlen && v.Blocks[j].List[jj] < uid; jj++ {
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

/*
// ApplyFilter applies a filter to our UIDList.
func ApplyFilter(u *task.List, f func(uint64, int) bool) {
	out := u.Uids[:0]
	for i, uid := range u.Uids {
		if f(uid, i) {
			out = append(out, uid)
		}
	}
	u.Uids = out
}

// IntersectWith intersects u with v. The update is made to u.
// u, v should be sorted.
func IntersectWith(u, v *task.List) {
	out := u.Uids[:0]
	n := len(u.Uids)
	m := len(v.Uids)
	for i, k := 0, 0; i < n && k < m; {
		uid := u.Uids[i]
		vid := v.Uids[k]
		if uid > vid {
			for k = k + 1; k < m && v.Uids[k] < uid; k++ {
			}
		} else if uid == vid {
			out = append(out, uid)
			k++
			i++
		} else {
			for i = i + 1; i < n && u.Uids[i] < vid; i++ {
			}
		}
	}
	u.Uids = out
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
	minLen := len(lists[0].Uids)
	for i := 1; i < len(lists); i++ { // Start from 1.
		l := lists[i]
		n := len(l.Uids)
		if n < minLen {
			minLen = n
			minLenIdx = i
		}
	}

	// Our final output. Give it some capacity.
	output := make([]uint64, 0, minLen)
	// lptrs[j] is the element we are looking at for lists[j].
	lptrs := make([]int, len(lists))
	shortList := lists[minLenIdx]
	elemsLeft := true // If some list has no elems left, we can't intersect more.

	for i := 0; i < len(shortList.Uids) && elemsLeft; i++ {
		val := shortList.Uids[i]
		if i > 0 && val == shortList.Uids[i-1] {
			x.AssertTruef(false, "We shouldn't have duplicates in UIDLists")
		}

		var skip bool                     // Should we skip val in output?
		for j := 0; j < len(lists); j++ { // For each other list in lists.
			if j == minLenIdx {
				// No point checking yourself.
				continue
			}

			lj := lists[j]
			ljp := lptrs[j]
			lsz := len(lj.Uids)
			for ; ljp < lsz && lj.Uids[ljp] < val; ljp++ {
			}

			lptrs[j] = ljp
			if ljp >= lsz || lj.Uids[ljp] > val {
				elemsLeft = ljp < lsz
				skip = true
				break
			}
			// Otherwise, lj.Get(ljp) = val and we continue checking other lists.
		}
		if !skip {
			output = append(output, val)
		}
	}
	return &task.List{Uids: output}
}

func Difference(u, v *task.List) {
	if u == nil || v == nil {
		return
	}
	out := u.Uids[:0]
	n := len(u.Uids)
	m := len(v.Uids)
	for i, k := 0, 0; i < n && k < m; {
		uid := u.Uids[i]
		vid := v.Uids[k]
		if uid < vid {
			for i < n && u.Uids[i] < vid {
				out = append(out, u.Uids[i])
				i++
			}
		} else if uid == vid {
			i++
			k++
		} else {
			for k = k + 1; k < m && v.Uids[k] < uid; k++ {
			}
		}
	}
	u.Uids = out
}

// MergeSorted merges sorted lists.
func MergeSorted(lists []*task.List) *task.List {
	if len(lists) == 0 {
		return new(task.List)
	}

	h := &uint64Heap{}
	heap.Init(h)

	for i, l := range lists {
		if len(l.Uids) > 0 {
			heap.Push(h, elem{
				val:     l.Uids[0],
				listIdx: i,
			})
		}
	}

	// Our final output. Give it some capacity.
	output := make([]uint64, 0, 100)
	// idx[i] is the element we are looking at for lists[i].
	idx := make([]int, len(lists))
	var last uint64   // Last element added to sorted / final output.
	for h.Len() > 0 { // While heap is not empty.
		me := (*h)[0] // Peek at the top element in heap.
		if len(output) == 0 || me.val != last {
			output = append(output, me.val) // Add if unique.
			last = me.val
		}
		l := lists[me.listIdx]
		if idx[me.listIdx] >= len(l.Uids)-1 {
			heap.Pop(h)
		} else {
			idx[me.listIdx]++
			val := l.Uids[idx[me.listIdx]]
			(*h)[0].val = val
			heap.Fix(h, 0) // Faster than Pop() followed by Push().
		}
	}
	return &task.List{Uids: output}
}

// IndexOf performs a binary search on the uids slice and returns the index at
// which it finds the uid, else returns -1
func IndexOf(u *task.List, uid uint64) int {
	i := sort.Search(len(u.Uids), func(i int) bool { return u.Uids[i] >= uid })
	if i < len(u.Uids) && u.Uids[i] == uid {
		return i
	}
	return -1
}

// ToUintsListForTest converts to list of uints for testing purpose only.
func ToUintsListForTest(ul []*task.List) [][]uint64 {
	out := make([][]uint64, 0, len(ul))
	for _, u := range ul {
		out = append(out, u.Uids)
	}
	return out
}
*/
