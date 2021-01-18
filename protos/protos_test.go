package protos

import (
	"path/filepath"
	"testing"

	"github.com/dgraph-io/dgraph/testutil"
	"github.com/stretchr/testify/require"
)

func TestProtosRegenerate(t *testing.T) {
	err := testutil.Exec("make", "regenerate")
	require.NoError(t, err, "Got error while regenerating protos: %v\n", err)

	generatedProtos := filepath.Join("pb", "pb.pb.go")
	err = testutil.Exec("git", "diff", "--quiet", "--", generatedProtos)
	require.NoError(t, err, "pb.pb.go changed after regenerating")
}
