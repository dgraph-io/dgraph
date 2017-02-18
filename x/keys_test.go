package x

import (
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDataKey(t *testing.T) {
	var uid uint64
	for uid = 0; uid < 1001; uid++ {
		sattr := fmt.Sprintf("attr:%d", uid)
		key := DataKey(sattr, uid)
		pk := Parse(key)

		require.True(t, pk.IsData())
		require.Equal(t, sattr, pk.Attr)
		require.Equal(t, uid, pk.Uid)
	}

	keys := make([]string, 0, 1024)
	for uid = 1024; uid >= 1; uid-- {
		key := DataKey("testing.key", uid)
		keys = append(keys, string(key))
	}
	// Test that sorting is as expected.
	sort.Strings(keys)
	require.True(t, sort.StringsAreSorted(keys))
	for i, key := range keys {
		exp := DataKey("testing.key", uint64(i+1))
		require.Equal(t, string(exp), key)
	}
}

func TestIndexKey(t *testing.T) {
	var uid uint64
	for uid = 0; uid < 1001; uid++ {
		sattr := fmt.Sprintf("attr:%d", uid)
		sterm := fmt.Sprintf("term:%d", uid)

		key := IndexKey(sattr, sterm)
		pk := Parse(key)

		require.True(t, pk.IsIndex())
		require.Equal(t, sattr, pk.Attr)
		require.Equal(t, sterm, pk.Term)
	}
}

func TestReverseKey(t *testing.T) {
	var uid uint64
	for uid = 0; uid < 1001; uid++ {
		sattr := fmt.Sprintf("attr:%d", uid)

		key := ReverseKey(sattr, uid)
		pk := Parse(key)

		require.True(t, pk.IsReverse())
		require.Equal(t, sattr, pk.Attr)
		require.Equal(t, uid, pk.Uid)
	}
}

func TestSchemaKey(t *testing.T) {
	var uid uint64
	for uid = 0; uid < 1001; uid++ {
		sattr := fmt.Sprintf("attr:%d", uid)

		key := SchemaKey(sattr)
		pk := Parse(key)

		require.True(t, pk.IsSchema())
		require.Equal(t, sattr, pk.Attr)
	}
}
