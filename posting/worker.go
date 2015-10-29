package posting

import (
	"container/heap"

	"github.com/google/flatbuffers/go"
	"github.com/dgraph-io/dgraph/task"
)

type elem struct {
	Uid   uint64
	Chidx int // channel index
}

type elemHeap []elem

func (h elemHeap) Len() int           { return len(h) }
func (h elemHeap) Less(i, j int) bool { return h[i].Uid < h[j].Uid }
func (h elemHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *elemHeap) Push(x interface{}) {
	*h = append(*h, x.(elem))
}
func (h *elemHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

func addUids(b *flatbuffers.Builder, sorted []uint64) flatbuffers.UOffsetT {
	// Invert the sorted uids to maintain same order in flatbuffers.
	task.ResultStartUidsVector(b, len(sorted))
	for i := len(sorted) - 1; i >= 0; i-- {
		b.PrependUint64(sorted[i])
	}
	return b.EndVector(len(sorted))
}

func ProcessTask(query []byte) (result []byte, rerr error) {
	uo := flatbuffers.GetUOffsetT(query)
	q := new(task.Query)
	q.Init(query, uo)

	b := flatbuffers.NewBuilder(0)
	var voffsets []flatbuffers.UOffsetT

	var channels []chan uint64
	attr := string(q.Attr())
	for i := 0; i < q.UidsLength(); i++ {
		uid := q.Uids(i)
		key := Key(uid, attr)
		pl := Get(key)

		task.ValueStart(b)
		var valoffset flatbuffers.UOffsetT
		if val, err := pl.Value(); err != nil {
			valoffset = b.CreateByteVector(nilbyte)
		} else {
			valoffset = b.CreateByteVector(val)
		}
		task.ValueAddVal(b, valoffset)
		voffsets = append(voffsets, task.ValueEnd(b))

		ch := make(chan uint64, 1000)
		go pl.StreamUids(ch)
		channels = append(channels, ch)
	}
	task.ResultStartValuesVector(b, len(voffsets))
	for i := len(voffsets) - 1; i >= 0; i-- {
		b.PrependUOffsetT(voffsets[i])
	}
	valuesVent := b.EndVector(len(voffsets))

	h := &elemHeap{}
	heap.Init(h)
	for i, ch := range channels {
		e := elem{Chidx: i}
		if uid, ok := <-ch; ok {
			e.Uid = uid
			heap.Push(h, e)
		}
	}

	var last uint64
	var ruids []uint64
	last = 0
	for h.Len() > 0 {
		// Pick the minimum uid.
		me := (*h)[0]
		if me.Uid != last {
			// We're iterating over sorted streams of uint64s. Avoid adding duplicates.
			ruids = append(ruids, me.Uid)
			last = me.Uid
		}

		// Now pick the next element from the channel which had the min uid.
		ch := channels[me.Chidx]
		if uid, ok := <-ch; !ok {
			heap.Pop(h)

		} else {
			me.Uid = uid
			(*h)[0] = me
			heap.Fix(h, 0) // Faster than Pop() followed by Push().
		}
	}
	uidsVend := addUids(b, ruids)

	task.ResultStart(b)
	task.ResultAddValues(b, valuesVent)
	task.ResultAddUids(b, uidsVend)
	rend := task.ResultEnd(b)
	b.Finish(rend)
	return b.Bytes[b.Head():], nil
}

func NewQuery(attr string, uids []uint64) []byte {
	b := flatbuffers.NewBuilder(0)
	task.QueryStartUidsVector(b, len(uids))
	for i := len(uids) - 1; i >= 0; i-- {
		b.PrependUint64(uids[i])
	}
	vend := b.EndVector(len(uids))

	ao := b.CreateString(attr)
	task.QueryStart(b)
	task.QueryAddAttr(b, ao)
	task.QueryAddUids(b, vend)
	qend := task.QueryEnd(b)
	b.Finish(qend)
	return b.Bytes[b.Head():]
}

var nilbyte []byte

func init() {
	nilbyte = make([]byte, 1)
	nilbyte[0] = 0x00
}
