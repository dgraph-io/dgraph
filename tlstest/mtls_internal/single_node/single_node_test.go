package single_node

import (
	"context"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/v210"
	"github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func runTests(t *testing.T, client *dgo.Dgraph) {
	type testCase struct {
		query      string
		wantResult string
	}
	suite := func(initialSchema string, setJSON string, cases []testCase) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		require.NoError(t, testutil.RetryAlter(client, &api.Operation{
			DropAll: true,
		}))
		require.NoError(t, testutil.RetryAlter(client, &api.Operation{
			Schema: initialSchema,
		}))

		txn := client.NewTxn()
		_, err := txn.Mutate(ctx, &api.Mutation{SetJson: []byte(setJSON)})
		require.NoError(t, err)
		require.NoError(t, txn.Commit(ctx))

		for _, test := range cases {
			txn := client.NewTxn()
			reply, err := txn.Query(ctx, test.query)
			require.NoError(t, err)
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

func TestClusterSetup(t *testing.T) {
	c := &x.TLSHelperConfig{
		CertRequired:     true,
		Cert:             "../tls/alpha1/client.alpha1.crt",
		Key:              "../tls/alpha1/client.alpha1.key",
		ServerName:       "alpha1",
		RootCACert:       "../tls/alpha1/ca.crt",
		UseSystemCACerts: true,
	}
	tlsConf, err := x.GenerateClientTLSConfig(c)
	require.NoError(t, err)
	dgConn, err := grpc.Dial(testutil.SockAddr, grpc.WithTransportCredentials(credentials.NewTLS(tlsConf)))
	require.NoError(t, err)
	client := dgo.NewDgraphClient(api.NewDgraphClient(dgConn))
	runTests(t, client)
}
