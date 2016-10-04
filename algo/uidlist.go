package algo

import (
	"container/heap"
	"sort"

	"github.com/google/flatbuffers/go"

	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/x"
)

type UIDList struct {
	uints []uint64
	list  *task.UidList
}

// NewUIDList creates a new UIDList given uint64 array. If you want your list
// to be allocated on stack, do consider using FromUints instead. However, if you
// are certain you are allocating on heap, you can use this.
func NewUIDList(data []uint64) *UIDList {
	u := new(UIDList)
	u.FromUints(data)
	return u
}

// FromUints initialize UIDList from []uint64.
func (u *UIDList) FromUints(data []uint64) {
	x.Assert(u.uints == nil)
	x.Assert(u.list == nil)
	u.uints = data
}

// FromUints initialize UIDList from task.UidList.
func (u *UIDList) FromTask(data *task.UidList) {
	x.Assert(u.uints == nil)
	x.Assert(u.list == nil)
	u.list = data
}

// Get returns the i-th element of UIDList.
func (u *UIDList) Get(i int) uint64 {
	if u.list != nil {
		return u.list.Uids(i)
	}
	return u.uints[i]
}

// Size returns size of UIDList.
func (u *UIDList) Size() int {
	if u == nil {
		return 0
	}
	if u.list != nil {
		return u.list.UidsLength()
	}
	return len(u.uints)
}

// Reslice selects a slice of the data.
func (u *UIDList) ApplyFilterFn(f func(uid uint64) bool) {
	u.convertToUints()
	var idx int
	for _, uid := range u.uints {
		if f(uid) {
			u.uints[idx] = uid
			idx++
		}
	}
	u.uints = u.uints[:idx]
}

// Slice selects a slice of the data.
func (u *UIDList) Slice(start, end int) {
	u.convertToUints()
	u.uints = u.uints[start:end]
}

// Intersect intersects with another list and updates this list.
// TODO(jchiu): Consider binary search if n>>m or n<<m.
func (u *UIDList) Intersect(v *UIDList) {
	u.convertToUints()

	n := u.Size()
	m := v.Size()
	var uIdx, vIdx int
	for i := 0; i < n; i++ {
		val := u.Get(i)
		for ; vIdx < m && v.Get(vIdx) < val; vIdx++ {
		}
		if vIdx < m && v.Get(vIdx) == val {
			u.uints[uIdx] = val
			uIdx++
		}
	}
	u.uints = u.uints[:uIdx]
}

// convertToUints converts internal representation to uints which is easier to
// work with than flatbuffers.
func (u *UIDList) convertToUints() {
	if u.uints == nil {
		x.Assert(u.list != nil)
		n := u.Size()
		u.uints = make([]uint64, n)
		for i := 0; i < n; i++ {
			u.uints[i] = u.list.Uids(i)
		}
		u.list = nil
	}
}

// IntersectLists intersect a list of UIDLists.
func IntersectLists(lists []*UIDList) *UIDList {
	if len(lists) == 0 {
		output := new(UIDList)
		output.FromUints([]uint64{})
		return output
	}

	// Scan through the smallest list. Denote as A.
	// For each x in A,
	//   For each other list B,
	//     Keep popping elements until we get a y >= x.
	//     If y > x, we want to skip x. Break out of loop for B.
	//   If we reach here, append our output by x.
	// We also remove all duplicates.
	var minLenIdx int
	minLen := lists[0].Size()
	for i := 1; i < len(lists); i++ { // Start from 1.
		l := lists[i]
		n := l.Size()
		if n < minLen {
			minLen = n
			minLenIdx = i
		}
	}

	// Our final output. Give it some capacity.
	output := make([]uint64, 0, minLen)
	// idx[i] is the element we are looking at for lists[i].
	idx := make([]int, len(lists))
	shortList := lists[minLenIdx]
	for i := 0; i < shortList.Size(); i++ {
		val := shortList.Get(i)
		if i > 0 && val == shortList.Get(i-1) {
			continue // Avoid duplicates.
		}

		var skip bool                     // Should we skip val in output?
		for j := 0; j < len(lists); j++ { // For each other list in lists.
			if j == minLenIdx {
				// No point checking yourself.
				continue
			}
			l := lists[j]
			k := idx[j]
			n := l.Size()
			for ; k < n && l.Get(k) < val; k++ {
			}
			idx[j] = k
			if k >= n || l.Get(k) > val {
				skip = true
				break
			}
			// Otherwise, l[k] = val and we continue checking other lists.
		}
		if !skip {
			output = append(output, val)
		}
	}

	ul := new(UIDList)
	ul.FromUints(output)
	return ul
}

// MergeLists merges sorted uint64 lists. Only unique numbers are returned.
func MergeLists(lists []*UIDList) *UIDList {
	if len(lists) == 0 {
		output := new(UIDList)
		output.FromUints([]uint64{})
		return output
	}

	h := &uint64Heap{}
	heap.Init(h)

	for i, l := range lists {
		if l.Size() > 0 {
			heap.Push(h, elem{
				val:     l.Get(0),
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
		if idx[me.listIdx] >= l.Size()-1 {
			heap.Pop(h)
		} else {
			idx[me.listIdx]++
			val := l.Get(idx[me.listIdx])
			(*h)[0].val = val
			heap.Fix(h, 0) // Faster than Pop() followed by Push().
		}
	}
	ul := new(UIDList)
	ul.FromUints(output)
	return ul
}

// IndexOf performs a binary search on the uids slice and returns the index at
// which it finds the uid, else returns -1
func (u *UIDList) IndexOf(uid uint64) int {
	i := sort.Search(u.Size(), func(i int) bool { return u.Get(i) >= uid })
	if i < u.Size() && u.Get(i) == uid {
		return i
	}
	return -1
}

// UidlistOffset adds a UidList to buffer and returns the offset.
func (u *UIDList) UidlistOffset(b *flatbuffers.Builder) flatbuffers.UOffsetT {
	n := u.Size()
	task.UidListStartUidsVector(b, n)
	for i := n - 1; i >= 0; i-- {
		b.PrependUint64(u.Get(i))
	}
	ulist := b.EndVector(n)
	task.UidListStart(b)
	task.UidListAddUids(b, ulist)
	return task.UidListEnd(b)
}
