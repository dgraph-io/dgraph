/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
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

package test

import (
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/gqlparser/v2/ast"
	"github.com/dgraph-io/gqlparser/v2/parser"
	"github.com/dgraph-io/gqlparser/v2/validator"
)

// Various helpers used in GQL testing

// LoadSchema parses and validates the given schema string and requires
// no errors.
func LoadSchema(t *testing.T, gqlSchema string) schema.Schema {

	doc, gqlErr := parser.ParseSchemas(validator.Prelude, &ast.Source{Input: gqlSchema})
	requireNoGQLErrors(t, gqlErr)

	gql, gqlErr := validator.ValidateSchemaDocument(doc)
	requireNoGQLErrors(t, gqlErr)

	schema, err := schema.AsSchema(gql, x.GalaxyNamespace)
	requireNoGQLErrors(t, err)
	return schema
}

// LoadSchemaFromFile reads a graphql schema file as would be the initial schema
// definition.  It runs all validation, generates the completed schema and
// returns that.
func LoadSchemaFromFile(t *testing.T, gqlFile string) schema.Schema {
	gql, err := ioutil.ReadFile(gqlFile)
	require.NoError(t, err, "Unable to read schema file")

	return LoadSchemaFromString(t, string(gql))
}

func LoadSchemaFromString(t *testing.T, sch string) schema.Schema {
	handler, err := schema.NewHandler(sch, false)
	requireNoGQLErrors(t, err)

	schema := LoadSchema(t, handler.GQLSchema())
	schema.SetMeta(handler.MetaInfo())

	return schema
}

// GetMutation gets a single schema.Mutation from a schema.Operation.
// It will fail if op is not a mutation or there's more than one mutation in
// op.
func GetMutation(t *testing.T, op schema.Operation) schema.Mutation {
	require.NotNil(t, op)

	mutations := op.Mutations()
	require.Len(t, mutations, 1)

	return mutations[0]
}

// GetQuery gets a single schema.Query from a schema.Operation.
// It will fail if op is not a query or there's more than one query in
// op.
func GetQuery(t *testing.T, op schema.Operation) schema.Query {
	require.NotNil(t, op)

	queries := op.Queries()
	require.Len(t, queries, 1)

	return queries[0]
}

// RequireJSONEq converts to JSON and tests JSON equality.
// It's easier to understand the diff, when a test fails, with json than
// require.Equal on for example GraphQL error lists.
func RequireJSONEq(t *testing.T, expected, got interface{}) {
	jsonExpected, err := json.Marshal(expected)
	require.NoError(t, err)

	jsonGot, err := json.Marshal(got)
	require.NoError(t, err)

	require.JSONEq(t, string(jsonExpected), string(jsonGot))
}

// RequireJSONEqStr converts to JSON and tests JSON equality.
// It's easier to understand the diff, when a test fails, with json than
// require.Equal on for example GraphQL error lists.
func RequireJSONEqStr(t *testing.T, expected string, got interface{}) {
	jsonGot, err := json.Marshal(got)
	require.NoError(t, err)

	require.JSONEq(t, expected, string(jsonGot))
}

func requireNoGQLErrors(t *testing.T, err error) {
	require.Nil(t, err,
		"required no GraphQL errors, but received :\n%s", serializeOrError(err))
}

func serializeOrError(toSerialize interface{}) string {
	byts, err := json.Marshal(toSerialize)
	if err != nil {
		return "unable to serialize because " + err.Error()
	}
	return string(byts)
}
