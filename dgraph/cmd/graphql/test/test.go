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
func LoadSchema(t *testing.T, gqlSchema string) *ast.Schema {
	doc, gqlErr := parser.ParseSchema(&ast.Source{Input: gqlSchema})
	require.Nil(t, gqlErr)
	// ^^ We can't use NoError here because gqlErr is of type *gqlerror.Error,
	// so passing into something that just expects an error, will always be a
	// non-nil interface.

	gql, gqlErr := validator.ValidateSchemaDocument(doc)
	require.Nil(t, gqlErr)

	return gql
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
