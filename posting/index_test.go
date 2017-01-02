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

func getTokensTable(t *testing.T) *TokensTable {
	table := &TokensTable{
		key: make([]string, 0, 50),
	}
	table.Add("ccc")
	table.Add("aaa")
	table.Add("bbb")
	table.Add("aaa")
	require.EqualValues(t, 3, table.Size())
	return table
}

func TestTokensTableIterate(t *testing.T) {
	tt := getTokensTable(t)
	require.EqualValues(t, "aaa", tt.GetFirst())
	require.EqualValues(t, "bbb", tt.GetNext("aaa"))
	require.EqualValues(t, "ccc", tt.GetNext("bbb"))
	require.EqualValues(t, "", tt.GetNext("ccc"))
}

func TestTokensTableIterateReverse(t *testing.T) {
	tt := getTokensTable(t)
	require.EqualValues(t, "ccc", tt.GetLast())
	require.EqualValues(t, "bbb", tt.GetPrev("ccc"))
	require.EqualValues(t, "aaa", tt.GetPrev("bbb"))
	require.EqualValues(t, "", tt.GetPrev("aaa"))
}

func TestTokensTableGetGeq(t *testing.T) {
	tt := getTokensTable(t)

	require.EqualValues(t, 1, tt.Get("bbb"))
	require.EqualValues(t, -1, tt.Get("zzz"))

	require.EqualValues(t, "aaa", tt.GetNextOrEqual("a"))
	require.EqualValues(t, "aaa", tt.GetNextOrEqual("aaa"))
	require.EqualValues(t, "bbb", tt.GetNextOrEqual("aab"))
	require.EqualValues(t, "ccc", tt.GetNextOrEqual("cc"))
	require.EqualValues(t, "ccc", tt.GetNextOrEqual("ccc"))
	require.EqualValues(t, "", tt.GetNextOrEqual("cccc"))

	require.EqualValues(t, "", tt.GetPrevOrEqual("a"))
	require.EqualValues(t, "aaa", tt.GetPrevOrEqual("aaa"))
	require.EqualValues(t, "aaa", tt.GetPrevOrEqual("aab"))
	require.EqualValues(t, "bbb", tt.GetPrevOrEqual("cc"))
	require.EqualValues(t, "ccc", tt.GetPrevOrEqual("ccc"))
	require.EqualValues(t, "ccc", tt.GetPrevOrEqual("cccc"))
}

const schemaStr = `
scalar name:string @index
scalar dob:date @index
`

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

	schema.ParseBytes([]byte(schemaStr))
	Init(ps)

	addEdgeToValue(t, ps, "name", 1, "Michonne")
	addEdgeToValue(t, ps, "name", 20, "David")
	return dir, ps
}

func TestRebuildIndex(t *testing.T) {
	dir, ps := populateGraph(t)
	defer ps.Close()
	defer os.RemoveAll(dir)

	// Create some fake wrong entries for TokensTable.
	tt := GetTokensTable("name")
	tt.Add("wronga")
	tt.Add("wrongb")
	tt.Add("wrongc")
	require.EqualValues(t, 3, tt.Size())

	// Create some fake wrong entries for data store.
	ps.SetOne(x.IndexKey("name", "wrongname1"), []byte("nothing"))
	ps.SetOne(x.IndexKey("name", "wrongname2"), []byte("nothing"))

	require.NoError(t, RebuildIndex(context.Background(), "name"))

	// Let's force a commit.
	CommitLists(10)
	for len(commitCh) > 0 {
		time.Sleep(100 * time.Millisecond)
	}

	// Check rebuilt TokensTable.
	tt = GetTokensTable("name")
	require.NotNil(t, tt)
	require.EqualValues(t, 2, tt.Size())
	require.EqualValues(t, "david", tt.GetFirst())
	require.EqualValues(t, "michonne", tt.GetNext("david"))

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
