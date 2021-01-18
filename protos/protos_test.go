package protos

import (
	"testing"

	"github.com/dgraph-io/dgraph/testutil"
	"github.com/stretchr/testify/require"
)

func TestProtosRegenerate(t *testing.T) {
	err := testutil.Exec("make", "regenerate")
	require.NoError(t, err, "Got error while regenerating protos: %v\n", err)
}
