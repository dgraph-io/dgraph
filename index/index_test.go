package index

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTokensTableBasic(t *testing.T) {
	keys := NewTokensTable()
	require.EqualValues(t, 0, keys.Size())

	// Query empty table.
	require.EqualValues(t, -1, keys.Get("hello"))

	// Add one element.
	keys.Add("world")
	require.EqualValues(t, 1, keys.Size())
	require.EqualValues(t, 0, keys.Get("world"))

	// Add the same element.
	keys.Add("world")
	require.EqualValues(t, 1, keys.Size())
	require.EqualValues(t, 0, keys.Get("world"))

	// Add a smaller element.
	keys.Add("aaa")
	require.EqualValues(t, 2, keys.Size())
	require.EqualValues(t, 0, keys.Get("aaa"))

	// Add a bigger element.
	keys.Add("zzz")
	require.EqualValues(t, 3, keys.Size())
	require.EqualValues(t, 2, keys.Get("zzz"))

	// Check previous elements.
	require.EqualValues(t, 0, keys.Get("aaa"))
	require.EqualValues(t, 1, keys.Get("world"))

	// Add to old element a few times.
	keys.Add("aaa")
	keys.Add("aaa")
	keys.Add("aaa")
	require.EqualValues(t, 3, keys.Size())
	require.EqualValues(t, 0, keys.Get("aaa"))

	// Test GetFirst.
	require.EqualValues(t, "aaa", keys.GetFirst())

	// Test GetNext.
	require.EqualValues(t, "aaa", keys.GetNext(""))
	require.EqualValues(t, "world", keys.GetNext("aaa"))
	require.EqualValues(t, "world", keys.GetNext("aab"))
	require.EqualValues(t, "zzz", keys.GetNext("world"))
	require.EqualValues(t, "", keys.GetNext("zzz"))
}
