/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package common

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/dgraph-io/dgo/v250"
	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/hypermodeinc/dgraph/v25/testutil"
)

const (
	expectedForInterface = `
	{ "__type": {
        "name": "Employee",
		"description": "GraphQL descriptions can be on interfaces.  They should work in the ` +
		`input\nschema and should make their way into the generated schema.",
        "fields": [
            {
                "name": "ename",
				"description": ""
            }
		],
		"enumValues":[]
	},
	  "__typename" : "Query"
	}`

	expectedForType = `
	{ "__type": {
        "name": "Author",
		"description": "GraphQL descriptions look like this.  They should work in the input\n` +
		`schema and should make their way into the generated schema.",
        "fields": [
            {
				"name": "id",
				"description": ""
            },
            {
                "name": "name",
		"description": "GraphQL descriptions can be on fields.  They should work in the input\n` +
		`schema and should make their way into the generated schema."
            },
            {
                "name": "dob",
				"description": ""
            },
            {
                "name": "reputation",
				"description": ""
            },
            {
                "name": "qualification",
				"description": ""
            },
            {
                "name": "country",
				"description": ""
            },
            {
                "name": "posts",
				"description": ""
            },
            {
                "name": "bio",
				"description": ""
            },
            {
                "name": "rank",
				"description": ""
            },
			{
				"name": "postsAggregate",
				"description": ""
			}
		],
		"enumValues":[]
	}, "__typename" : "Query" }`

	expectedForEnum = `
	{ "__type": {
        "name": "PostType",
		"description": "GraphQL descriptions can be on enums.  They should work in the input\n` +
		`schema and should make their way into the generated schema.",
        "enumValues": [
            {
                "name": "Fact",
				"description": ""
            },
            {
            	"name": "Question",
				"description": "GraphQL descriptions can be on enum values.  They should work in ` +
		`the input\nschema and should make their way into the generated schema."
            },
            {
                "name": "Opinion",
				"description": ""
            }
		],
		"fields":[]
    }, "__typename" : "Query" }`
)

func SchemaTest(t *testing.T, expectedDgraphSchema string) {
	d, err := grpc.NewClient(Alpha1gRPC, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)

	client := dgo.NewDgraphClient(api.NewDgraphClient(d))

	resp, err := client.NewReadOnlyTxn().Query(context.Background(), "schema {}")
	require.NoError(t, err)

	testutil.CompareJSON(t, expectedDgraphSchema, string(resp.GetJson()))
}

func graphQLDescriptions(t *testing.T) {

	testCases := map[string]struct {
		typeName string
		expected string
	}{
		"interface": {typeName: "Employee", expected: expectedForInterface},
		"type":      {typeName: "Author", expected: expectedForType},
		"enum":      {typeName: "PostType", expected: expectedForEnum},
	}

	query := `
	query TestDescriptions($name: String!) {
		__type(name: $name) {
			name
			description
			fields {
				name
			  	description
			}
			enumValues {
				name
				description
			}
		}
		__typename
	}`

	for testName, tCase := range testCases {
		t.Run(testName, func(t *testing.T) {
			introspect := &GraphQLParams{
				Query: query,
				Variables: map[string]interface{}{
					"name": tCase.typeName,
				},
			}

			introspectionResult := introspect.ExecuteAsPost(t, GraphqlURL)
			RequireNoGQLErrors(t, introspectionResult)

			require.JSONEq(t, tCase.expected, string(introspectionResult.Data))
		})
	}
}
