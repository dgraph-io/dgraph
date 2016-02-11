package posting

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/commit"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/x"
	"github.com/google/flatbuffers/go"
)

func addEdge(t *testing.T, edge x.DirectedEdge, l *List) {
	if err := l.AddMutation(edge, Set); err != nil {
		t.Error(err)
	}
}

func check(r *task.Result, idx int, expected []uint64) error {
	var m task.UidList
	if ok := r.Uidmatrix(&m, idx); !ok {
		return fmt.Errorf("Unable to retrieve uidlist")
	}

	if m.UidsLength() != len(expected) {
		return fmt.Errorf("Expected length: %v. Got: %v",
			len(expected), m.UidsLength())
	}
	for i, uid := range expected {
		if m.Uids(i) != uid {
			return fmt.Errorf("Uid mismatch at index: %v. Expected: %v. Got: %v",
				i, uid, m.Uids(i))
		}
	}
	return nil
}

func TestProcessTask(t *testing.T) {
	// logrus.SetLevel(logrus.DebugLevel)

	dir, err := ioutil.TempDir("", "storetest_")
	if err != nil {
		t.Error(err)
		return
	}

	defer os.RemoveAll(dir)
	ps := new(store.Store)
	ps.Init(dir)

	clog := commit.NewLogger(dir, "mutations", 50<<20)
	clog.Init()
	defer clog.Close()

	Init(clog)

	edge := x.DirectedEdge{
		ValueId:   23,
		Source:    "author0",
		Timestamp: time.Now(),
	}
	addEdge(t, edge, GetOrCreate(Key(10, "friend"), ps))
	addEdge(t, edge, GetOrCreate(Key(11, "friend"), ps))
	addEdge(t, edge, GetOrCreate(Key(12, "friend"), ps))

	edge.ValueId = 25
	addEdge(t, edge, GetOrCreate(Key(12, "friend"), ps))

	edge.ValueId = 26
	addEdge(t, edge, GetOrCreate(Key(12, "friend"), ps))

	edge.ValueId = 31
	addEdge(t, edge, GetOrCreate(Key(10, "friend"), ps))
	addEdge(t, edge, GetOrCreate(Key(12, "friend"), ps))

	edge.Value = "photon"
	addEdge(t, edge, GetOrCreate(Key(12, "friend"), ps))

	query := NewQuery("friend", []uint64{10, 11, 12})
	result, err := ProcessTask(query, ps)
	if err != nil {
		t.Error(err)
	}

	ro := flatbuffers.GetUOffsetT(result)
	r := new(task.Result)
	r.Init(result, ro)

	if r.UidmatrixLength() != 3 {
		t.Errorf("Expected 3. Got uidmatrix length: %v", r.UidmatrixLength())
	}
	if err := check(r, 0, []uint64{23, 31}); err != nil {
		t.Error(err)
	}
	if err := check(r, 1, []uint64{23}); err != nil {
		t.Error(err)
	}
	if err := check(r, 2, []uint64{23, 25, 26, 31}); err != nil {
		t.Error(err)
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
	if ok := r.Values(&tval, 1); !ok {
		t.Errorf("Unable to retrieve value")
	}
	if tval.ValLength() != 1 ||
		tval.ValBytes()[0] != 0x00 {
		t.Errorf("Invalid byte value at index 0")
	}

	if ok := r.Values(&tval, 2); !ok {
		t.Errorf("Unable to retrieve value")
	}
	var iout interface{}
	if err := ParseValue(&iout, tval.ValBytes()); err != nil {
		t.Error(err)
	}
	v := iout.(string)
	if v != "photon" {
		t.Errorf("Expected photon. Got: %q", v)
	}
}
