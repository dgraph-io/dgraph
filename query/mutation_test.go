package query

import (
	"context"
	"testing"

	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/rdf"
	"github.com/stretchr/testify/require"
)

func TestConvertToEdges(t *testing.T) {
	q1 := `<0x01> <type> <0x02> .
	       <0x01> <character> <0x03> .`
	nquads, err := rdf.ConvertToNQuads(q1)
	require.NoError(t, err)

	ctx := context.Background()
	mr, err := ConvertToEdges(ctx, gql.WrapNQ(nquads, protos.DirectedEdge_SET), nil)
	require.NoError(t, err)

	require.EqualValues(t, len(mr.Edges), 2)
}
