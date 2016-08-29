package bidx

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/commit"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
	"golang.org/x/net/context"
)

func arrayCompare(a []uint64, b []uint64) error {
	if len(a) != len(b) {
		return fmt.Errorf("Size mismatch %d vs %d", len(a), len(b))
	}
	for i := 0; i < len(a); i++ {
		if a[i] != b[i] {
			return fmt.Errorf("Element mismatch at index %d", i)
		}
	}
	return nil
}

func TestMergeResults1(t *testing.T) {
	l1 := &LookupResult{
		UID: []uint64{1, 3, 6, 8, 10},
	}
	l2 := &LookupResult{
		UID: []uint64{2, 4, 5, 7, 15},
	}
	lr := []*LookupResult{l1, l2}
	results := mergeResults(lr)
	arrayCompare(results.UID, []uint64{1, 2, 3, 4, 5, 6, 7, 8, 10, 15})
}

func TestMergeResults2(t *testing.T) {
	l1 := &LookupResult{
		UID: []uint64{1, 3, 6, 8, 10},
	}
	l2 := &LookupResult{
		UID: []uint64{},
	}
	lr := []*LookupResult{l1, l2}
	results := mergeResults(lr)
	arrayCompare(results.UID, []uint64{1, 3, 6, 8, 10})
}

func TestMergeResults3(t *testing.T) {
	l1 := &LookupResult{
		UID: []uint64{},
	}
	l2 := &LookupResult{
		UID: []uint64{1, 3, 6, 8, 10},
	}
	lr := []*LookupResult{l1, l2}
	results := mergeResults(lr)
	arrayCompare(results.UID, []uint64{1, 3, 6, 8, 10})
}

func TestMergeResults4(t *testing.T) {
	l1 := &LookupResult{
		UID: []uint64{},
	}
	l2 := &LookupResult{
		UID: []uint64{},
	}
	lr := []*LookupResult{l1, l2}
	results := mergeResults(lr)
	arrayCompare(results.UID, []uint64{})
}

func TestMergeResults5(t *testing.T) {
	l1 := &LookupResult{
		UID: []uint64{11, 13, 16, 18, 20},
	}
	l2 := &LookupResult{
		UID: []uint64{12, 14, 15, 17, 25},
	}
	l3 := &LookupResult{
		UID: []uint64{1, 2},
	}
	lr := []*LookupResult{l1, l2, l3}
	results := mergeResults(lr)
	arrayCompare(results.UID, []uint64{1, 2, 11, 12, 13, 14, 15, 16, 17, 18, 20, 25})
}

func TestMergeResults6(t *testing.T) {
	l1 := &LookupResult{
		UID: []uint64{5, 6, 7},
	}
	l2 := &LookupResult{
		UID: []uint64{3, 4},
	}
	l3 := &LookupResult{
		UID: []uint64{1, 2},
	}
	l4 := &LookupResult{
		UID: []uint64{},
	}
	lr := []*LookupResult{l1, l2, l3, l4}
	results := mergeResults(lr)
	arrayCompare(results.UID, []uint64{1, 2, 3, 4, 5, 6, 7})
}

func addEdge(t *testing.T, edge x.DirectedEdge, l *posting.List) {
	if err := l.AddMutation(context.Background(), edge, posting.Set); err != nil {
		t.Error(err)
	}
}

func getIndices(t *testing.T) (string, *Indices) {
	dir, err := ioutil.TempDir("", "storetest_")
	x.Check(err)

	ps := new(store.Store)
	ps.Init(dir)
	worker.Init(ps, ps, 0, 1)
	clog := commit.NewLogger(dir, "mutations", 50<<20)
	clog.Init()
	posting.Init(clog)

	// So, user we're interested in has uid: 1.
	// She has 2 friends: 23, 24, 25, 31, and 101
	edge := x.DirectedEdge{
		ValueId:   23,
		Source:    "testing",
		Timestamp: time.Now(),
	}
	addEdge(t, edge, posting.GetOrCreate(posting.Key(1, "friend"), ps))

	edge.ValueId = 24
	addEdge(t, edge, posting.GetOrCreate(posting.Key(1, "friend"), ps))

	edge.ValueId = 25
	addEdge(t, edge, posting.GetOrCreate(posting.Key(1, "friend"), ps))

	edge.ValueId = 31
	addEdge(t, edge, posting.GetOrCreate(posting.Key(1, "friend"), ps))

	edge.ValueId = 101
	addEdge(t, edge, posting.GetOrCreate(posting.Key(1, "friend"), ps))

	// Now let's add a name for each of the friends, except 101.
	edge.Value = []byte("Rick Grimes")
	addEdge(t, edge, posting.GetOrCreate(posting.Key(23, "name"), ps))

	edge.Value = []byte("Glenn Rhee")
	addEdge(t, edge, posting.GetOrCreate(posting.Key(24, "name"), ps))

	edge.Value = []byte("Daryl Dixon")
	addEdge(t, edge, posting.GetOrCreate(posting.Key(25, "name"), ps))

	edge.Value = []byte("Andrea")
	addEdge(t, edge, posting.GetOrCreate(posting.Key(31, "name"), ps))

	edge.Value = []byte("mich")
	// Belongs to UID store actually!
	addEdge(t, edge, posting.GetOrCreate(posting.Key(1, "_xid_"), ps))

	// Remember to move data from hash to RocksDB.
	posting.MergeLists(10)

	// Create fake indices.
	reader := bytes.NewReader([]byte(
		`{"Config": [{"Type": "text", "Attribute": "name", "NumShards": 1}]}`))
	indicesConfig, err := NewIndicesConfig(reader)
	x.Check(err)
	x.Check(CreateIndices(indicesConfig, dir))
	indices := InitWorker(dir)
	x.Check(indices.Backfill(ps))

	return dir, indices
}

func TestLookup(t *testing.T) {
	dir, indices := getIndices(t)
	defer os.RemoveAll(dir)

	li := &LookupSpec{
		Attr:     "name",
		Param:    []string{"Glenn"},
		Category: LookupMatch,
	}
	lr := indices.Lookup(li)
	if lr.Err != nil {
		t.Error(lr.Err)
	}
	if len(lr.UID) != 1 {
		t.Error(fmt.Errorf("Expected 1 hit, got %d", len(lr.UID)))
		return
	}
	if lr.UID[0] != 24 {
		t.Error(fmt.Errorf("Expected UID 24, got %d", lr.UID[0]))
	}
}
