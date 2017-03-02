package posting

import (
	"context"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

const schemaStr = `
scalar name:string @index
`

func TestIndexingInt(t *testing.T) {
	schema.ParseBytes([]byte("scalar age:int @index"))
	a, err := IndexTokens("age", types.Val{types.StringID, []byte("10")})
	require.NoError(t, err)
	require.EqualValues(t, []byte{0x1, 0x0, 0x0, 0x0, 0xa}, []byte(a[0]))
}

func TestIndexingIntNegative(t *testing.T) {
	schema.ParseBytes([]byte("scalar age:int @index"))
	a, err := IndexTokens("age", types.Val{types.StringID, []byte("-10")})
	require.NoError(t, err)
	require.EqualValues(t, []byte{0x0, 0xff, 0xff, 0xff, 0xf6}, []byte(a[0]))
}

func TestIndexingFloat(t *testing.T) {
	schema.ParseBytes([]byte("scalar age:float @index"))
	a, err := IndexTokens("age", types.Val{types.StringID, []byte("10.43")})
	require.NoError(t, err)
	require.EqualValues(t, []byte{0x1, 0x0, 0x0, 0x0, 0xa}, []byte(a[0]))
}

func TestIndexingDate(t *testing.T) {
	schema.ParseBytes([]byte("scalar age:date @index"))
	a, err := IndexTokens("age", types.Val{types.StringID, []byte("0010-01-01")})
	require.NoError(t, err)
	require.EqualValues(t, []byte{0x1, 0x0, 0x0, 0x0, 0xa}, []byte(a[0]))
}

func TestIndexingTime(t *testing.T) {
	schema.ParseBytes([]byte("scalar age:datetime @index"))
	a, err := IndexTokens("age", types.Val{types.StringID, []byte("0010-01-01T01:01:01.000000001")})
	require.NoError(t, err)
	require.EqualValues(t, []byte{0x1, 0x0, 0x0, 0x0, 0xa}, []byte(a[0]))
}

func TestIndexing(t *testing.T) {
	schema.ParseBytes([]byte("scalar name:string @index"))
	a, err := IndexTokens("name", types.Val{types.StringID, []byte("abc")})
	require.NoError(t, err)
	require.EqualValues(t, "abc", string(a[0]))
}

func addMutationWithIndex(t *testing.T, l *List, edge *task.DirectedEdge, op uint32) {
	if op == Del {
		edge.Op = task.DirectedEdge_DEL
	} else if op == Set {
		edge.Op = task.DirectedEdge_SET
	} else {
		x.Fatalf("Unhandled op: %v", op)
	}
	require.NoError(t, l.AddMutationWithIndex(context.Background(), edge))
}

func TestTokensTable(t *testing.T) {
	dir, err := ioutil.TempDir("", "storetest_")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	schema.ParseBytes([]byte(schemaStr))

	ps, err := store.NewStore(dir)
	defer ps.Close()
	defer Stop()
	require.NoError(t, err)
	Init(ps)

	key := x.DataKey("name", 1)
	l := getNew(key, ps)

	edge := &task.DirectedEdge{
		Value:  []byte("david"),
		Label:  "testing",
		Attr:   "name",
		Entity: 157,
	}
	addMutationWithIndex(t, l, edge, Set)

	key = x.IndexKey("name", "david")
	slice, err := ps.Get(key)
	require.NoError(t, err)

	var pl types.PostingList
	x.Check(pl.Unmarshal(slice.Data()))

	require.EqualValues(t, []string{"david"}, tokensForTest("name"))

	CommitLists(10)
	time.Sleep(time.Second)

	slice, err = ps.Get(key)
	require.NoError(t, err)
	x.Check(pl.Unmarshal(slice.Data()))

	require.EqualValues(t, []string{"david"}, tokensForTest("name"))
}

const schemaStrAlt = `
scalar name:string @index
scalar dob:date @index
`

// tokensForTest returns keys for a table. This is just for testing / debugging.
func tokensForTest(attr string) []string {
	pk := x.ParsedKey{Attr: attr}
	prefix := pk.IndexPrefix()
	it := pstore.NewIterator()
	defer it.Close()

	var out []string
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		k := x.Parse(it.Key().Data())
		x.AssertTrue(k.IsIndex())
		out = append(out, k.Term)
	}
	return out
}

// addEdgeToValue adds edge without indexing.
func addEdgeToValue(t *testing.T, ps *store.Store, attr string, src uint64,
	value string) {
	edge := &task.DirectedEdge{
		Value:  []byte(value),
		Label:  "testing",
		Attr:   attr,
		Entity: src,
		Op:     task.DirectedEdge_SET,
	}
	l, _ := GetOrCreate(x.DataKey(attr, src), 0)
	// No index entries added here as we do not call AddMutationWithIndex.
	ok, err := l.AddMutation(context.Background(), edge)
	require.NoError(t, err)
	require.True(t, ok)
}

func populateGraph(t *testing.T) (string, *store.Store) {
	dir, err := ioutil.TempDir("", "storetest_")
	require.NoError(t, err)

	ps, err := store.NewStore(dir)
	require.NoError(t, err)

	schema.ParseBytes([]byte(schemaStrAlt))
	Init(ps)

	addEdgeToValue(t, ps, "name", 1, "Michonne")
	addEdgeToValue(t, ps, "name", 20, "David")
	return dir, ps
}

func TestRebuildIndex(t *testing.T) {
	dir, ps := populateGraph(t)
	defer os.RemoveAll(dir)
	defer ps.Close()
	defer Stop() // stop posting.

	// RebuildIndex requires the data to be committed to data store.
	CommitLists(10)
	for len(syncCh) > 0 {
		time.Sleep(100 * time.Millisecond)
	}

	// Create some fake wrong entries for data store.
	ps.SetOne(x.IndexKey("name", "wrongname1"), []byte("nothing"))
	ps.SetOne(x.IndexKey("name", "wrongname2"), []byte("nothing"))

	require.NoError(t, RebuildIndex(context.Background(), "name"))

	// Let's force a commit.
	CommitLists(10)
	for len(syncCh) > 0 {
		time.Sleep(100 * time.Millisecond)
	}

	// Check index entries in data store.
	it := ps.NewIterator()
	defer it.Close()
	pk := x.ParsedKey{Attr: "name"}
	prefix := pk.IndexPrefix()
	var idxKeys []string
	var idxVals []*types.PostingList
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		idxKeys = append(idxKeys, string(it.Key().Data()))
		pl := new(types.PostingList)
		require.NoError(t, pl.Unmarshal(it.Value().Data()))
		idxVals = append(idxVals, pl)
	}
	require.Len(t, idxKeys, 2)
	require.Len(t, idxVals, 2)
	require.EqualValues(t, x.IndexKey("name", "david"), idxKeys[0])
	require.EqualValues(t, x.IndexKey("name", "michonne"), idxKeys[1])
	require.Len(t, idxVals[0].Postings, 1)
	require.Len(t, idxVals[1].Postings, 1)
	require.EqualValues(t, idxVals[0].Postings[0].Uid, 20)
	require.EqualValues(t, idxVals[1].Postings[0].Uid, 1)
}
