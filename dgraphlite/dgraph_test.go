package dgraphlite_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v230/protos/api"
	"github.com/dgraph-io/dgraph/v24/dgraphlite"
)

// TODO tests
// 1. custom tokenizer tests
// 2. encryption tests

func TestDgraphLite(t *testing.T) {
	dg, err := dgraphlite.New(dgraphlite.NewConfig().WithDataDir(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer dg.Close()

	require.NoError(t, dg.Alter(context.Background(), &api.Operation{DropAll: true}))
	require.NoError(t, dg.Alter(context.Background(),
		&api.Operation{Schema: `name: string @index(term) .`}))

	_, err = dg.Query(context.Background(), &api.Request{
		Mutations: []*api.Mutation{
			{
				Set: []*api.NQuad{
					{
						Subject:     "0x5",
						Predicate:   "name",
						ObjectValue: &api.Value{Val: &api.Value_StrVal{StrVal: "A"}},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := dg.Query(context.Background(), &api.Request{
		Query: `{
			me(func: has(name)) {
				name
			}
		}`,
	})
	if err != nil {
		t.Fatal(err)
	}

	if string(resp.GetJson()) != `{"me":[{"name":"A"}]}` {
		t.Fatalf("got invalid response: %v", string(resp.GetJson()))
	}
}
