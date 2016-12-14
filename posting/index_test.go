package posting

import (
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/types"
	"github.com/stretchr/testify/require"
)

func TestIndexingInt(t *testing.T) {
	var v types.Int32
	v = 10

	schema.ParseBytes([]byte("scalar age:int @index"))
	a, err := indexTokens("age", types.Value(&v))
	require.NoError(t, err)
	require.EqualValues(t, []byte{0x0, 0x0, 0x0, 0xa}, a[0])
}

func TestIndexingFloat(t *testing.T) {
	var v types.Float
	v = 10.43

	schema.ParseBytes([]byte("scalar age:float @index"))
	a, err := indexTokens("age", types.Value(&v))
	require.NoError(t, err)
	require.EqualValues(t, []byte{0x0, 0x0, 0x0, 0xa}, a[0])
}

func TestIndexingDate(t *testing.T) {
	var v types.Date
	v.Time = time.Date(10, 1, 1, 1, 1, 1, 1, time.UTC)

	schema.ParseBytes([]byte("scalar age:date @index"))
	a, err := indexTokens("age", types.Value(&v))
	require.NoError(t, err)
	require.EqualValues(t, []byte{0x0, 0x0, 0x0, 0xa}, a[0])
}

func TestIndexingTime(t *testing.T) {
	var v types.Time
	v.Time = time.Date(10, 1, 1, 1, 1, 1, 1, time.UTC)

	schema.ParseBytes([]byte("scalar age:datetime @index"))
	a, err := indexTokens("age", types.Value(&v))
	require.NoError(t, err)
	require.EqualValues(t, []byte{0x0, 0x0, 0x0, 0xa}, a[0])
}

func TestIndexing(t *testing.T) {
	var v types.String
	v = "abc"

	schema.ParseBytes([]byte("scalar name:string @index"))
	a, err := indexTokens("name", types.Value(&v))
	require.NoError(t, err)
	require.EqualValues(t, "abc", string(a[0]))
}

func getTokensTable(t *testing.T) *TokensTable {
	tt := NewTokensTable()
	tt.Add("ccc")
	tt.Add("aaa")
	tt.Add("bbb")
	tt.Add("aaa")
	require.EqualValues(t, 3, tt.Size())
	return tt
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
