package x

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValueType(t *testing.T) {
	require.Equal(t, ValueType(false, false, false), ValueUid)
	require.Equal(t, ValueType(false, false, true), ValueEmpty)
	require.Equal(t, ValueType(false, true, false), ValueUid)
	require.Equal(t, ValueType(false, true, true), ValueEmpty)
	require.Equal(t, ValueType(true, false, false), ValueUntagged)
	require.Equal(t, ValueType(true, false, true), ValueUntagged)
	require.Equal(t, ValueType(true, true, false), ValueTagged)
	require.Equal(t, ValueType(true, true, true), ValueTagged)
}
