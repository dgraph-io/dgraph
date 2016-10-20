package types

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/x"
)

func TestIndexKey(t *testing.T) {
	var tests = []struct {
		in  string
		out bool
	}{
		{":123", true},
		{":莊子", true},
		{"123", false},
		{"莊子", false},
	}

	for _, tt := range tests {
		require.EqualValues(t, tt.out, IsIndexKey([]byte(tt.in)), "%v %v", tt.in, tt.out)
		require.EqualValues(t, tt.out, IsIndexKeyStr(tt.in), "%v %v", tt.in, tt.out)
	}
}

func TestMain(m *testing.M) {
	x.Init()
	os.Exit(m.Run())
}
