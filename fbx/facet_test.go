package fbx_test

import (
	"testing"

	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/fbx"
	"github.com/stretchr/testify/require"
)

func TestFacet(t *testing.T) {
	exp := &api.Facet{
		Key:     "key",
		Value:   []byte("value"),
		ValType: api.Facet_STRING,
		Tokens:  []string{"some", "tokens"},
		Alias:   "alias",
	}

	got := fbx.NewFacet().
		SetKey(exp.Key).
		SetValue(exp.Value).
		SetValueType(exp.ValType).
		SetTokens(exp.Tokens).
		SetAlias(exp.Alias).
		Build()

	require.Equal(t, exp.Key, string(got.Key()))
	require.Equal(t, exp.Value, got.ValueBytes())
	require.Equal(t, len(exp.Tokens), got.TokensLength())
	for i, token := range exp.Tokens {
		require.Equal(t, token, string(got.Tokens(i)))
	}
	require.Equal(t, exp.Alias, string(got.Alias()))
}
