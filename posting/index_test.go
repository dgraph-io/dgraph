package posting

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"sort"
	"testing"
	"time"

	"github.com/boltdb/bolt"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/schema"
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
	schema.ParseBytes([]byte(schemaStr))
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

	var slice []byte
	err := ps.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("data"))
		if b == nil {
			return fmt.Errorf("Bucket not found")
		}
		slice = b.Get(key)
		return nil
	})
	require.NoError(t, err)

	var pl types.PostingList
	x.Check(pl.Unmarshal(slice))

	require.EqualValues(t, []string{"david"}, tokensForTest("name"))

	CommitLists(10)
	time.Sleep(time.Second)

	err = ps.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("data"))
		if b == nil {
			return fmt.Errorf("Bucket not found")
		}
		slice = b.Get(key)
		return nil
	})
	require.NoError(t, err)
	x.Check(pl.Unmarshal(slice))

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

	var out []string
	pstore.View(func(tx *bolt.Tx) error {
		c := tx.Bucket([]byte("data")).Cursor()
		for k, _ := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, _ = c.Next() {
			key := x.Parse(k)
			x.AssertTrue(key.IsIndex())
			out = append(out, key.Term)
		}
		return nil
	})

	return out
}

// addEdgeToValue adds edge without indexing.
func addEdgeToValue(t *testing.T, attr string, src uint64,
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

func populateGraph(t *testing.T) *bolt.DB {
	schema.ParseBytes([]byte(schemaStrAlt))
	Init(ps)

	addEdgeToValue(t, "name", 1, "Michonne")
	addEdgeToValue(t, "name", 20, "David")
	return ps
}

func TestRebuildIndex(t *testing.T) {
	populateGraph(t)
	// RebuildIndex requires the data to be committed to data store.
	CommitLists(10)
	for len(syncCh) > 0 {
		time.Sleep(100 * time.Millisecond)
	}

	// Create some fake wrong entries for data store.
	ps.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("data"))
		b.Put(x.IndexKey("name", "wrongname1"), []byte("nothing"))
		b.Put(x.IndexKey("name", "wrongname2"), []byte("nothing"))
		return nil
	})

	require.NoError(t, RebuildIndex(context.Background(), "name"))

	// Let's force a commit.
	CommitLists(10)
	for len(syncCh) > 0 {
		time.Sleep(100 * time.Millisecond)
	}

	// Check index entries in data store.
	pk := x.ParsedKey{Attr: "name"}
	prefix := pk.IndexPrefix()
	var idxKeys []string
	var idxVals []*types.PostingList

	ps.View(func(tx *bolt.Tx) error {
		c := tx.Bucket([]byte("data")).Cursor()
		for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
			idxKeys = append(idxKeys, string(k))
			pl := new(types.PostingList)
			require.NoError(t, pl.Unmarshal(v))
			idxVals = append(idxVals, pl)
		}
		return nil
	})

	require.Len(t, idxKeys, 2)
	require.Len(t, idxVals, 2)
	require.EqualValues(t, x.IndexKey("name", "david"), idxKeys[0])
	require.EqualValues(t, x.IndexKey("name", "michonne"), idxKeys[1])
	require.Len(t, idxVals[0].Postings, 1)
	require.Len(t, idxVals[1].Postings, 1)
	require.EqualValues(t, idxVals[0].Postings[0].Uid, 20)
	require.EqualValues(t, idxVals[1].Postings[0].Uid, 1)
}

type encL struct {
	ints   []int32
	tokens []string
}

type byEnc struct{ encL }

func (o byEnc) Less(i, j int) bool {
	return o.ints[i] < o.ints[j]
}

func (o byEnc) Len() int { return len(o.ints) }

func (o byEnc) Swap(i, j int) {
	o.ints[i], o.ints[j] = o.ints[j], o.ints[i]
	o.tokens[i], o.tokens[j] = o.tokens[j], o.tokens[i]
}

func TestIntEncoding(t *testing.T) {
	a := int32(2<<24 + 10)
	b := int32(-2<<24 - 1)
	c := int32(math.MaxInt32)
	d := int32(math.MinInt32)
	enc := encL{}
	arr := []int32{a, b, c, d, 1, 2, 3, 4, -1, -2, -3, 0, 234, 10000, 123, -1543}
	enc.ints = arr
	for _, it := range arr {
		encoded, err := encodeInt(int32(it))
		require.NoError(t, err)
		enc.tokens = append(enc.tokens, encoded[0])
	}
	sort.Sort(byEnc{enc})
	for i := 1; i < len(enc.tokens); i++ {
		// The corresponding string tokens should be greater.
		require.True(t, enc.tokens[i-1] < enc.tokens[i])
	}
}
