package ha_6_node

import (
	"context"
	"github.com/dgraph-io/dgo/v200"
	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/testutil"
	"google.golang.org/grpc"
	"testing"
	"time"
)

func check(t *testing.T, err error) {
	if err != nil {
		t.Fatalf("%+v", err)
	}
}

func runTests(t *testing.T, client *dgo.Dgraph) {
	type testCase struct {
		query      string
		wantResult string
	}
	suite := func(initialSchema string, setJSON string, cases []testCase) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		check(t, client.Alter(ctx, &api.Operation{
			DropAll: true,
		}))
		check(t, client.Alter(ctx, &api.Operation{
			Schema: initialSchema,
		}))

		txn := client.NewTxn()
		_, err := txn.Mutate(ctx, &api.Mutation{SetJson: []byte(setJSON)})
		check(t, err)
		check(t, txn.Commit(ctx))

		for _, test := range cases {
			txn := client.NewTxn()
			reply, err := txn.Query(ctx, test.query)
			check(t, err)
			testutil.CompareJSON(t, test.wantResult, string(reply.GetJson()))
		}
	}

	suite(
		"name: string @index(term) .",
		`[
			{ "name": "Michael" },
			{ "name": "Amit" },
			{ "name": "Luke" },
			{ "name": "Darth" },
			{ "name": "Sarah" },
			{ "name": "Ricky" },
			{ "name": "Hugo" }
		]`,
		[]testCase{
			{`
				{
					q(func: eq(name, "Hugo")) {
						name
					}
				}`, `
				{
				"q": [
				  {
					"name": "Hugo"
				  }
				]
			  }`,
			},
		},
	)
}

func TestHAClusterSetup(t *testing.T) {
	dgConn, err := grpc.Dial(":7180", grpc.WithInsecure())
	if err != nil {
		t.Fatalf("%+v", err)
	}

	time.Sleep(time.Second * 6)
	client := dgo.NewDgraphClient(api.NewDgraphClient(dgConn))
	runTests(t, client)
}