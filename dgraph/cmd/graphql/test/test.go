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

package test

import (
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/parser"
	"github.com/vektah/gqlparser/validator"
)

// Various helpers used in GQL testing

// LoadSchema parses and validates the given schema string and requires
// no errors.
func LoadSchema(t *testing.T, gqlSchema string) schema.Schema {

	doc, gqlErr := parser.ParseSchemas(validator.Prelude, &ast.Source{Input: gqlSchema})
	require.Nil(t, gqlErr)
	// ^^ We can't use NoError here because gqlErr is of type *gqlerror.Error,
	// so passing into something that just expects an error, will always be a
	// non-nil interface.

	gql, gqlErr := validator.ValidateSchemaDocument(doc)
	require.Nil(t, gqlErr)

	return schema.AsSchema(gql)
}

// LoadSchemaFromFile reads a graphql schema file as would be the initial schema
// definition.  It runs all validation, generates the completed schema and
// returns that.
func LoadSchemaFromFile(t *testing.T, gqlFile string) schema.Schema {
	gql, err := ioutil.ReadFile(gqlFile)
	require.NoError(t, err, "Unable to read schema file")

	handler, err := schema.NewHandler(string(gql))
	require.NoError(t, err, "input schema contained errors")

	return LoadSchema(t, handler.GQLSchema())
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

// GetQuery gets a single schema.Mutation from a schema.Operation.
// It will fail if op is not a mutation or there's more than one mutation in
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

// replaceWith replaces a given key with a new value in a map.
func replaceWith(vars map[string]interface{}, key string, newVal interface{}) {
	for k, val := range vars {
		if k == key {
			vars[k] = newVal
		}
		switch val := val.(type) {
		case map[string]interface{}:
			replaceWith(val, key, newVal)
		case []map[string]interface{}:
			for _, v := range val {
				replaceWith(v, key, newVal)
			}
		}
	}
}

// ReplaceJSON takes a []byte slice (expected to be json data), replaces all keys
// in toReplace with the corresponding values, and then re-marshals to a []byte.
// Often in GraphQL e2e testing there are values we don't care about for correctness
// comparisions and which we can't predict before the test is run: e.g. uids (which
// will be different for each test run) and requestIDs (which will be different for
// every GraphQL request).
func ReplaceJSON(input []byte, toReplace map[string]interface{}) ([]byte, error) {
	var inputJSON map[string]interface{}
	err := json.Unmarshal(input, &inputJSON)
	if err != nil {
		return nil, err
	}

	for key, val := range toReplace {
		replaceWith(inputJSON, key, val)
	}

	return json.Marshal(inputJSON)
}
