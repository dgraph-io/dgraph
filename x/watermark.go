package x

import (
	"container/heap"
	"fmt"
	"sync/atomic"
)

type uint64Heap []uint64

func (u uint64Heap) Len() int               { return len(u) }
func (u uint64Heap) Less(i int, j int) bool { return u[i] < u[j] }
func (u uint64Heap) Swap(i int, j int)      { u[i], u[j] = u[j], u[i] }
func (u *uint64Heap) Push(x interface{})    { *u = append(*u, x.(uint64)) }
func (u *uint64Heap) Pop() interface{} {
	old := *u
	n := len(old)
	x := old[n-1]
	*u = old[0 : n-1]
	return x
}

type Mark struct {
	Index uint64
	Done  bool
}

type WaterMark struct {
	Name      string
	Ch        chan Mark
	doneUntil uint64
}

func (w *WaterMark) Init() {
	w.Ch = make(chan Mark, 1000)
	go w.Process()
}

func (w *WaterMark) DoneUntil() uint64 {
	return atomic.LoadUint64(&w.doneUntil)
}

func (w *WaterMark) Process() {
	var indices uint64Heap
	pending := make(map[uint64]bool)

	heap.Init(&indices)
	for mark := range w.Ch {
		// If not already done, then set. Otherwise, don't undo a done entry.
		done, present := pending[mark.Index]
		if !present {
			heap.Push(&indices, mark.Index)
		}
		if !done {
			pending[mark.Index] = mark.Done
		}

		if len(indices) > 0 {
			fmt.Printf("%s: Done entry %4d. Size: %4d Watermark: %-4d Looking for: %-4d\n", w.Name, mark.Index, len(indices), w.DoneUntil(), indices[0])
		}

		// Update mark by going through all indices in order; and checking if they have
		// been done. Stop at the first index, which isn't done.
		doneUntil := w.DoneUntil()
		until := doneUntil
		loops := 0
		for len(indices) > 0 {
			min := indices[0]
			if done := pending[min]; !done {
				break
			}
			heap.Pop(&indices)
			delete(pending, min)
			until = min
			loops++
		}
		if until != doneUntil {
			AssertTrue(atomic.CompareAndSwapUint64(&w.doneUntil, doneUntil, until))
			fmt.Printf("%s: Done until %d. Loops: %d\n", w.Name, until, loops)
		}
	}
}
