package algo

import (
	"bytes"
	"container/heap"
	"log"
	"sort"
	"strconv"

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
	x.Assert(u != nil && u.uints == nil && u.list == nil)
	u.uints = data
}

// FromUints initialize UIDList from task.UidList.
func (u *UIDList) FromTask(data *task.UidList) {
	x.Assert(u != nil && u.uints == nil && u.list == nil)
	u.list = data
}

// FromTaskResult parses task.Result and extracts a []*UIDList.
func FromTaskResult(r *task.Result) []*UIDList {
	out := make([]*UIDList, r.UidmatrixLength())
	for i := 0; i < r.UidmatrixLength(); i++ {
		tl := new(task.UidList)
		x.Assert(r.Uidmatrix(tl, i))
		ul := new(UIDList)
		ul.FromTask(tl)
		out[i] = ul
	}
	return out
}

// FromSortResult parses task.Result and extracts a []*UIDList.
func FromSortResult(r *task.SortResult) []*UIDList {
	out := make([]*UIDList, r.UidmatrixLength())
	for i := 0; i < r.UidmatrixLength(); i++ {
		tl := new(task.UidList)
		x.Assert(r.Uidmatrix(tl, i))
		ul := new(UIDList)
		ul.FromTask(tl)
		out[i] = ul
	}
	return out
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
		// In a subgraph node, in processGraph, sometimes we might fan out to zero
		// nodes, i.e., sg.destUIDs is empty. In this case, child subgraph might not
		// have its srcUIDs initialized.
		return 0
	}
	if u.list != nil {
		return u.list.UidsLength()
	}
	return len(u.uints)
}

// Reslice selects a slice of the data.
func (u *UIDList) ApplyFilter(f func(uint64, int) bool) {
	x.Assert(u != nil && (u.uints != nil || u.list != nil))
	var out []uint64
	if u.uints != nil {
		out = u.uints[:0]
	} else {
		out = make([]uint64, 0, 30)
	}
	n := u.Size()
	for i := 0; i < n; i++ {
		uid := u.Get(i)
		if f(uid, i) {
			out = append(out, uid)
		}
	}
	u.uints = out
	u.list = nil
}

// Slice selects a slice of the data.
func (u *UIDList) Slice(start, end int) {
	x.Assert(u != nil && (u.uints != nil || u.list != nil))
	if u.uints != nil {
		u.uints = u.uints[start:end]
		return
	}
	// This is a task list. Let's copy what we want and convert to a []uint64.
	x.Assert(start >= 0)
	x.Assert(end <= u.Size())
	x.Assert(start <= end)
	output := make([]uint64, 0, end-start)
	for i := start; i < end; i++ {
		output = append(output, u.list.Uids(i))
	}
	u.uints = output
	u.list = nil
}

// Intersect intersects with another list and updates this list.
func (u *UIDList) Intersect(v *UIDList) {
	x.Assert(u != nil && (u.uints != nil || u.list != nil))
	var out []uint64
	if u.uints != nil {
		out = u.uints[:0]
	} else {
		n := u.Size()
		if v.Size() < n {
			n = v.Size()
		}
		out = make([]uint64, 0, n)
	}

	n := u.Size()
	m := v.Size()
	var k int
	for i := 0; i < n; i++ {
		uid := u.Get(i)
		for ; k < m && v.Get(k) < uid; k++ {
		}
		if k < m && v.Get(k) == uid {
			out = append(out, uid)
		}
	}
	u.uints = out
	u.list = nil
}

// IntersectLists intersect a list of UIDLists. An alternative is to do
// pairwise intersections n-1 times where n=number of lists. This is less
// efficient:
// Let p be length of shortest list. Let q be average length of lists. So
// nq = total number of elements.
// There are many possible cases. Consider the case where the shortest list
// is the answer (or close to the answer). The following method requires nq
// reads (each element is read only once) whereas pairwise intersections can
// require np + nq - p reads, which can be up to ~twice as many.
func IntersectLists(lists []*UIDList) *UIDList {
	if len(lists) == 0 {
		return NewUIDList([]uint64{})
	}

	// Scan through the smallest list. Denote as A.
	// For each x in A,
	//   For each other list B,
	//     Keep popping elements until we get a y >= x.
	//     If y > x, mark x as "skipped". Break out of loop.
	//   If x is not marked as "skipped", append x to result.
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
	// lptrs[j] is the element we are looking at for lists[j].
	lptrs := make([]int, len(lists))
	shortList := lists[minLenIdx]
	for i := 0; i < shortList.Size(); i++ {
		val := shortList.Get(i)
		if i > 0 && val == shortList.Get(i-1) {
			log.Fatal("We shouldn't have duplicates in UIDLists")
		}

		var skip bool                     // Should we skip val in output?
		for j := 0; j < len(lists); j++ { // For each other list in lists.
			if j == minLenIdx {
				// No point checking yourself.
				continue
			}

			lj := lists[j]
			ljp := lptrs[j]
			lsz := lj.Size()
			for ; ljp < lsz && lj.Get(ljp) < val; ljp++ {
			}

			lptrs[j] = ljp
			if ljp >= lsz || lj.Get(ljp) > val {
				skip = true
				break
			}
			// Otherwise, lj.Get(ljp) = val and we continue checking other lists.
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
		return NewUIDList([]uint64{})
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
	x.Assert(u != nil && (u.uints != nil || u.list != nil))
	i := sort.Search(u.Size(), func(i int) bool { return u.Get(i) >= uid })
	if i < u.Size() && u.Get(i) == uid {
		return i
	}
	return -1
}

// UidlistOffset adds a UidList to buffer and returns the offset.
func (u *UIDList) AddTo(b *flatbuffers.Builder) flatbuffers.UOffsetT {
	x.Assert(u != nil && (u.uints != nil || u.list != nil))
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

// DebugString returns a debug string for UIDList.
func (u *UIDList) DebugString() string {
	var b bytes.Buffer
	for i := 0; i < u.Size(); i++ {
		b.WriteRune('[')
		b.WriteString(strconv.FormatUint(u.Get(i), 10))
		b.WriteString("] ")
	}
	return b.String()
}

// ToUintsForTest converts to uints for testing purpose only.
func (u *UIDList) ToUintsForTest() []uint64 {
	out := make([]uint64, 0, u.Size())
	for i := 0; i < u.Size(); i++ {
		out = append(out, u.Get(i))
	}
	return out
}

// ToUintsListForTest converts to list of uints for testing purpose only.
func ToUintsListForTest(ul []*UIDList) [][]uint64 {
	out := make([][]uint64, 0, len(ul))
	for _, u := range ul {
		out = append(out, u.ToUintsForTest())
	}
	return out
}

// Swap swaps two elements. Logs fatal if UIDList is not stored as []uint64.
func (u *UIDList) Swap(i, j int) {
	x.Assert(u.uints != nil)
	u.uints[i], u.uints[j] = u.uints[j], u.uints[i]
}
