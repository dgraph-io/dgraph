package posting

import (
	"container/heap"
	"os"
	"testing"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/google/flatbuffers/go"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/x"
)

func TestPush(t *testing.T) {
	h := &elemHeap{}
	heap.Init(h)

	e := elem{Uid: 5}
	heap.Push(h, e)
	e.Uid = 3
	heap.Push(h, e)
	e.Uid = 4
	heap.Push(h, e)

	if h.Len() != 3 {
		t.Errorf("Expected len 3. Found: %v", h.Len())
	}
	if (*h)[0].Uid != 3 {
		t.Errorf("Expected min 3. Found: %+v", (*h)[0])
	}
	e.Uid = 10
	(*h)[0] = e
	heap.Fix(h, 0)
	if (*h)[0].Uid != 4 {
		t.Errorf("Expected min 4. Found: %+v", (*h)[0])
	}
	e.Uid = 11
	(*h)[0] = e
	heap.Fix(h, 0)
	if (*h)[0].Uid != 5 {
		t.Errorf("Expected min 5. Found: %+v", (*h)[0])
	}

	e = heap.Pop(h).(elem)
	if e.Uid != 5 {
		t.Errorf("Expected min 5. Found %+v", e)
	}

	e = heap.Pop(h).(elem)
	if e.Uid != 10 {
		t.Errorf("Expected min 10. Found: %+v", e)
	}
	e = heap.Pop(h).(elem)
	if e.Uid != 11 {
		t.Errorf("Expected min 11. Found: %+v", e)
	}

	if h.Len() != 0 {
		t.Errorf("Expected len 0. Found: %v, values: %+v", h.Len(), h)
	}
}

func addEdge(t *testing.T, edge x.DirectedEdge, l *List) {
	if err := l.AddMutation(edge, Set); err != nil {
		t.Error(err)
	}
}

func TestProcessTask(t *testing.T) {
	logrus.SetLevel(logrus.DebugLevel)

	pdir := NewStore(t)
	defer os.RemoveAll(pdir)
	ps := new(store.Store)
	ps.Init(pdir)

	mdir := NewStore(t)
	defer os.RemoveAll(mdir)
	ms := new(store.Store)
	ms.Init(mdir)
	Init(ps, ms)

	edge := x.DirectedEdge{
		ValueId:   23,
		Source:    "author0",
		Timestamp: time.Now(),
	}
	addEdge(t, edge, Get(Key(10, "friend")))
	addEdge(t, edge, Get(Key(11, "friend")))
	addEdge(t, edge, Get(Key(12, "friend")))

	edge.ValueId = 25
	addEdge(t, edge, Get(Key(12, "friend")))

	edge.ValueId = 26
	addEdge(t, edge, Get(Key(12, "friend")))

	edge.ValueId = 31
	addEdge(t, edge, Get(Key(10, "friend")))
	addEdge(t, edge, Get(Key(12, "friend")))

	edge.Value = "photon"
	addEdge(t, edge, Get(Key(12, "friend")))

	query := NewQuery("friend", []uint64{10, 11, 12})
	result, err := ProcessTask(query)
	if err != nil {
		t.Error(err)
	}

	ro := flatbuffers.GetUOffsetT(result)
	r := new(task.Result)
	r.Init(result, ro)

	if r.UidsLength() != 4 {
		t.Errorf("Expected 4. Got uids length: %v", r.UidsLength())
	}
	if r.Uids(0) != 23 {
		t.Errorf("Expected 23. Got: %v", r.Uids(0))
	}
	if r.Uids(1) != 25 {
		t.Errorf("Expected 25. Got: %v", r.Uids(0))
	}
	if r.Uids(2) != 26 {
		t.Errorf("Expected 26. Got: %v", r.Uids(0))
	}
	if r.Uids(3) != 31 {
		t.Errorf("Expected 31. Got: %v", r.Uids(0))
	}
	if r.ValuesLength() != 3 {
		t.Errorf("Expected 3. Got values length: %v", r.ValuesLength())
	}

	var tval task.Value
	if ok := r.Values(&tval, 0); !ok {
		t.Errorf("Unable to retrieve value")
	}
	if tval.ValLength() != 1 ||
		tval.ValBytes()[0] != 0x00 {
		t.Errorf("Invalid byte value at index 0")
	}

	if ok := r.Values(&tval, 2); !ok {
		t.Errorf("Unable to retrieve value")
	}

	var v string
	if err := ParseValue(&v, tval.ValBytes()); err != nil {
		t.Error(err)
	}
	if v != "photon" {
		t.Errorf("Expected photon. Got: %q", v)
	}
}
