/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package common

import (
	"context"
	"testing"

	"github.com/dgraph-io/dgo/v2"
	"github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
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
	} }`

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
                "name": "country",
				"description": ""
            },
            {
                "name": "posts",
				"description": ""
            }
		],
		"enumValues":[]
	} }`

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
    } }`
)

func SchemaTest(t *testing.T, expectedDgraphSchema string) {
	d, err := grpc.Dial(alphagRPC, grpc.WithInsecure())
	require.NoError(t, err)

	client := dgo.NewDgraphClient(api.NewDgraphClient(d))

	resp, err := client.NewReadOnlyTxn().Query(context.Background(), "schema {}")
	require.NoError(t, err)

	require.JSONEq(t, expectedDgraphSchema, string(resp.GetJson()))
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
	}`

	for testName, tCase := range testCases {
		t.Run(testName, func(t *testing.T) {
			introspect := &GraphQLParams{
				Query: query,
				Variables: map[string]interface{}{
					"name": tCase.typeName,
				},
			}

			introspectionResult := introspect.ExecuteAsPost(t, graphqlURL)
			require.Nil(t, introspectionResult.Errors)

			require.JSONEq(t, tCase.expected, string(introspectionResult.Data))
		})
	}
}
