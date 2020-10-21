package fbx_test

import (
	"fmt"
	"testing"

	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/fb"
	"github.com/dgraph-io/dgraph/fbx"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/stretchr/testify/require"
)

func TestPosting(t *testing.T) {
	uid := uint64(1)
	value := []byte("value")
	valueType := pb.PostingValType_BINARY
	langTag := []byte("langTag")
	label := "label"
	facets := make([]*api.Facet, 5)
	for i := range facets {
		facets[i] = &api.Facet{
			Key: fmt.Sprintf("key%d", i),
		}
	}
	op := uint32(2)
	startTs := uint64(3)
	commitTs := uint64(4)

	builder := fbx.NewPosting().
		SetUid(uid).
		SetValue(value).
		SetValueType(valueType).
		SetLangTag(langTag).
		SetLabel(label).
		SetFacets(facets).
		SetOp(op).
		SetStartTs(startTs).
		SetCommitTs(commitTs)

	p := builder.Build()
	require.Equal(t, uid, p.Uid())
	require.Equal(t, value, p.ValueBytes())
	require.Equal(t, valueType, pb.PostingValType(p.ValueType()))
	require.Equal(t, langTag, p.LangTagBytes())
	require.Equal(t, label, string(p.Label()))
	require.Equal(t, len(facets), p.FacetsLength())
	for i, expFacet := range facets {
		gotFacet := new(fb.Facet)
		require.True(t, p.Facets(gotFacet, i))
		require.Equal(t, expFacet.Key, string(gotFacet.Key()))
	}
	require.Equal(t, op, p.Op())
	require.Equal(t, startTs, p.StartTs())
	require.Equal(t, commitTs, p.CommitTs())
}
