package bulk

import (
	"testing"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/stretchr/testify/require"
)

func TestUnmarsha(t *testing.T) {
	a := &pb.MapEntry{}
	a.Key = []byte("helllo")
	b, _ := a.Marshal()
	key, _ := GetKeyForMapEntry(b)
	require.Equal(t, a.Key, key)
}
