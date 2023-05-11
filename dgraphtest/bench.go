package dgraphtest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func RunPerfTest(b *testing.B, conf interface{}, perfTest func(cluster interface{}, b *testing.B)) {
	if conf == nil {
		perfTest(nil, b)
		return
	}
	cluster, err := NewLocalCluster(conf.(ClusterConfig))
	require.NoError(b, err)
	defer cluster.Cleanup()
	require.NoError(b, cluster.Start())
	perfTest(cluster, b)
}
