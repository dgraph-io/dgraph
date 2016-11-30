package x

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDataKey(t *testing.T) {
	var uid uint64
	for uid = 0; uid < 1001; uid++ {
		sattr := fmt.Sprintf("attr:%d", uid)
		key := DataKey(sattr, uid)
		attr, dst := ParseData(key)
		require.Equal(t, sattr, attr)
		require.Equal(t, uid, dst)

	}
}

func TestIndexKey(t *testing.T) {
	var uid uint64
	for uid = 0; uid < 1001; uid++ {
		sattr := fmt.Sprintf("attr:%d", uid)
		sterm := fmt.Sprintf("term:%d", uid)

		key := IndexKey(sattr, sterm)
		require.True(t, IsIndex(key))

		dattr := PredicateFrom(key)
		require.Equal(t, sattr, dattr)

		dterm := TermFromIndex(key)
		require.Equal(t, sterm, dterm)
	}

}
