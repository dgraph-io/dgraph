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
	// Dgraph schema should look like this if the GraphQL layer has started and
	// successfully connected
	initSchema = `{
    "schema": [
        {
            "predicate": "dgraph.graphql.schema",
            "type": "string"
        },
        {
            "predicate": "dgraph.type",
            "type": "string",
            "index": true,
            "tokenizer": [
                "exact"
            ],
            "list": true
        }
    ],
    "types": [
        {
            "fields": [
                {
                    "name": "dgraph.graphql.schema"
                }
            ],
            "name": "dgraph.graphql"
        }
    ]
}`

	firstTypes = `
	type A {
		b: String
	}`
	firstSchema = `{
    "schema": [
        {
            "predicate": "A.b",
            "type": "string"
        },
        {
            "predicate": "dgraph.graphql.schema",
            "type": "string"
        },
        {
            "predicate": "dgraph.type",
            "type": "string",
            "index": true,
            "tokenizer": [
                "exact"
            ],
            "list": true
        }
    ],
    "types": [
        {
            "fields": [
                {
                    "name": "A.b"
                }
            ],
            "name": "A"
        },
        {
            "fields": [
                {
                    "name": "dgraph.graphql.schema"
                }
            ],
            "name": "dgraph.graphql"
        }
    ]
}`
	firstGQLSchema = `{
    "__type": {
        "name": "A",
        "fields": [
            {
                "name": "b"
            }
        ]
    }
}`

	updatedTypes = `
	type A {
		b: String
		c: Int
	}`
	updatedSchema = `{
    "schema": [
        {
            "predicate": "A.b",
            "type": "string"
        },
        {
            "predicate": "A.c",
            "type": "int"
        },
        {
            "predicate": "dgraph.graphql.schema",
            "type": "string"
        },
        {
            "predicate": "dgraph.type",
            "type": "string",
            "index": true,
            "tokenizer": [
                "exact"
            ],
            "list": true
        }
    ],
    "types": [
        {
            "fields": [
                {
                    "name": "A.b"
                },
                {
                    "name": "A.c"
                }
            ],
            "name": "A"
        },
        {
            "fields": [
                {
                    "name": "dgraph.graphql.schema"
                }
            ],
            "name": "dgraph.graphql"
        }
    ]
}`
	updatedGQLSchema = `{
    "__type": {
        "name": "A",
        "fields": [
            {
                "name": "b"
            },
            {
                "name": "c"
            }
        ]
    }
}`
)

func admin(t *testing.T) {
	d, err := grpc.Dial(alphaAdminTestgRPC, grpc.WithInsecure())
	require.NoError(t, err)

	client := dgo.NewDgraphClient(api.NewDgraphClient(d))

	err = checkGraphQLHealth(graphqlAdminTestAdminURL, []string{"NoGraphQLSchema"})
	require.NoError(t, err)

	schemaIsInInitialState(t, client)
	addGQLSchema(t, client)
	updateSchema(t, client)
}

func schemaIsInInitialState(t *testing.T, client *dgo.Dgraph) {
	resp, err := client.NewReadOnlyTxn().Query(context.Background(), "schema {}")
	require.NoError(t, err)

	require.JSONEq(t, initSchema, string(resp.GetJson()))
}

func addGQLSchema(t *testing.T, client *dgo.Dgraph) {
	err := addSchema(graphqlAdminTestAdminURL, firstTypes)
	require.NoError(t, err)

	resp, err := client.NewReadOnlyTxn().Query(context.Background(), "schema {}")
	require.NoError(t, err)

	require.JSONEq(t, firstSchema, string(resp.GetJson()))

	introspect(t, firstGQLSchema)
}

func updateSchema(t *testing.T, client *dgo.Dgraph) {
	err := addSchema(graphqlAdminTestAdminURL, updatedTypes)
	require.NoError(t, err)

	resp, err := client.NewReadOnlyTxn().Query(context.Background(), "schema {}")
	require.NoError(t, err)

	require.JSONEq(t, updatedSchema, string(resp.GetJson()))

	introspect(t, updatedGQLSchema)
}

func introspect(t *testing.T, expected string) {
	queryParams := &GraphQLParams{
		Query: `query {
			__type(name: "A") {
				name
				fields {
					name
				}
			}
		}`,
	}

	gqlResponse := queryParams.ExecuteAsPost(t, graphqlAdminTestURL)
	requireNoGQLErrors(t, gqlResponse)

	require.JSONEq(t, expected, string(gqlResponse.Data))
}
