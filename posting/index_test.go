package posting

import (
	"fmt"
	"testing"

	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/types"
	"github.com/stretchr/testify/require"
)

func TestIndexingInt(t *testing.T) {
	schema.ParseBytes([]byte("scalar age:int @index"))
	a, err := indexTokens("age", types.StringID, []byte("10"))
	fmt.Println(a)
	require.NoError(t, err)
	require.EqualValues(t, []byte{0x0, 0x0, 0x0, 0xa}, a[0])
}

func TestIndexingFloat(t *testing.T) {
	schema.ParseBytes([]byte("scalar age:float @index"))
	a, err := indexTokens("age", types.StringID, []byte("10.43"))
	require.NoError(t, err)
	require.EqualValues(t, []byte{0x0, 0x0, 0x0, 0xa}, a[0])
}

func TestIndexingDate(t *testing.T) {
	schema.ParseBytes([]byte("scalar age:date @index"))
	a, err := indexTokens("age", types.StringID, []byte("0010-01-01"))
	require.NoError(t, err)
	require.EqualValues(t, []byte{0x0, 0x0, 0x0, 0xa}, a[0])
}

func TestIndexingTime(t *testing.T) {
	schema.ParseBytes([]byte("scalar age:datetime @index"))
	a, err := indexTokens("age", types.StringID, []byte("0010-01-01T01:01:01.000000001"))
	require.NoError(t, err)
	require.EqualValues(t, []byte{0x0, 0x0, 0x0, 0xa}, a[0])
}

func TestIndexing(t *testing.T) {
	schema.ParseBytes([]byte("scalar name:string @index"))
	a, err := indexTokens("name", types.StringID, []byte("abc"))
	require.NoError(t, err)
	require.EqualValues(t, "abc", string(a[0]))
}
