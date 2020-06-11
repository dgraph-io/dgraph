/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
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

package resolve

import (
	"testing"

	"github.com/dgraph-io/dgraph/graphql/test"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
)

func TestQueriesPropagateExtensions(t *testing.T) {
	gqlSchema := test.LoadSchemaFromString(t, testGQLSchema)
	query := `
	query {
      getAuthor(id: "0x1") {
        name
      }
    }`

	resp := resolveWithClient(gqlSchema, query, nil,
		&executor{
			queryTouched:    2,
			mutationTouched: 5,
		})

	require.NotNil(t, resp)
	require.Nil(t, resp.Errors)
	require.NotNil(t, resp.Extensions)

	require.Equal(t, uint64(2), resp.Extensions.TouchedUids)
	require.NotNil(t, resp.Extensions.Tracing)

	require.Equal(t, resp.Extensions.Tracing.Version, x.Version())
	require.True(t, len(resp.Extensions.Tracing.StartTime.String()) > 0)
	require.True(t, len(resp.Extensions.Tracing.EndTime.String()) > 0)
	require.True(t, resp.Extensions.Tracing.Duration > 0)

	require.Len(t, resp.Extensions.Tracing.Execution, 1)
	require.Equal(t, resp.Extensions.Tracing.Execution[0].Path, []interface{}{"getAuthor"})
	require.Equal(t, resp.Extensions.Tracing.Execution[0].ParentType, "Query")
	require.Equal(t, resp.Extensions.Tracing.Execution[0].FieldName, "getAuthor")
	require.Equal(t, resp.Extensions.Tracing.Execution[0].ReturnType, "Author")
	require.True(t, resp.Extensions.Tracing.Execution[0].StartOffset > 0)
	require.True(t, resp.Extensions.Tracing.Execution[0].Duration > 0)

	require.Len(t, resp.Extensions.Tracing.Execution[0].Dgraph, 1)
	require.Equal(t, resp.Extensions.Tracing.Execution[0].Dgraph[0].Label, "query")
	require.True(t, resp.Extensions.Tracing.Execution[0].Dgraph[0].StartOffset > 0)
	require.True(t, resp.Extensions.Tracing.Execution[0].Dgraph[0].Duration > 0)

}

func TestMultipleQueriesPropagateExtensionsCorrectly(t *testing.T) {
	gqlSchema := test.LoadSchemaFromString(t, testGQLSchema)
	query := `
	query {
      a: getAuthor(id: "0x1") {
        name
      }
      b: getAuthor(id: "0x2") {
        name
      }
      c: getAuthor(id: "0x3") {
        name
      }
    }`

	resp := resolveWithClient(gqlSchema, query, nil,
		&executor{
			queryTouched:    2,
			mutationTouched: 5,
		})

	require.NotNil(t, resp)
	require.Nil(t, resp.Errors)
	require.NotNil(t, resp.Extensions)

	require.Equal(t, uint64(6), resp.Extensions.TouchedUids)
	require.NotNil(t, resp.Extensions.Tracing)

	require.Equal(t, resp.Extensions.Tracing.Version, x.Version())
	require.True(t, len(resp.Extensions.Tracing.StartTime.String()) > 0)
	require.True(t, len(resp.Extensions.Tracing.EndTime.String()) > 0)
	require.True(t, resp.Extensions.Tracing.Duration > 0)

	require.Len(t, resp.Extensions.Tracing.Execution, 3)
	aliases := []string{"a", "b", "c"}
	for i, execution := range resp.Extensions.Tracing.Execution {
		require.Equal(t, execution.Path, []interface{}{aliases[i]})
		require.Equal(t, execution.ParentType, "Query")
		require.Equal(t, execution.FieldName, aliases[i])
		require.Equal(t, execution.ReturnType, "Author")
		require.True(t, execution.StartOffset > 0)
		require.True(t, execution.Duration > 0)
		require.Len(t, execution.Dgraph, 1)
		require.Equal(t, execution.Dgraph[0].Label, "query")
		require.True(t, execution.Dgraph[0].StartOffset > 0)
		require.True(t, execution.Dgraph[0].Duration > 0)
	}
}

func TestMutationsPropagateExtensions(t *testing.T) {
	gqlSchema := test.LoadSchemaFromString(t, testGQLSchema)
	mutation := `mutation {
		addPost(input: [{title: "A Post", author: {id: "0x1"}}]) {
			post {
				title
			}
		}
	}`

	resp := resolveWithClient(gqlSchema, mutation, nil,
		&executor{
			queryTouched:    2,
			mutationTouched: 5,
		})

	require.NotNil(t, resp)
	require.Nil(t, resp.Errors)
	require.NotNil(t, resp.Extensions)

	// as both .Mutate() and .Query() should get called, so we should get their merged result
	require.Equal(t, uint64(7), resp.Extensions.TouchedUids)
	require.NotNil(t, resp.Extensions.Tracing)

	require.Equal(t, resp.Extensions.Tracing.Version, x.Version())
	require.True(t, len(resp.Extensions.Tracing.StartTime.String()) > 0)
	require.True(t, len(resp.Extensions.Tracing.EndTime.String()) > 0)
	require.True(t, resp.Extensions.Tracing.Duration > 0)

	require.Len(t, resp.Extensions.Tracing.Execution, 1)
	require.Equal(t, resp.Extensions.Tracing.Execution[0].Path, []interface{}{"addPost"})
	require.Equal(t, resp.Extensions.Tracing.Execution[0].ParentType, "Mutation")
	require.Equal(t, resp.Extensions.Tracing.Execution[0].FieldName, "addPost")
	require.Equal(t, resp.Extensions.Tracing.Execution[0].ReturnType, "AddPostPayload")
	require.True(t, resp.Extensions.Tracing.Execution[0].StartOffset > 0)
	require.True(t, resp.Extensions.Tracing.Execution[0].Duration > 0)

	require.Len(t, resp.Extensions.Tracing.Execution[0].Dgraph, 2)
	labels := []string{"mutation", "query"}
	for i, dgraphTrace := range resp.Extensions.Tracing.Execution[0].Dgraph {
		require.Equal(t, dgraphTrace.Label, labels[i])
		require.True(t, dgraphTrace.StartOffset > 0)
		require.True(t, dgraphTrace.Duration > 0)
	}
}

func TestMultipleMutationsPropagateExtensionsCorrectly(t *testing.T) {
	gqlSchema := test.LoadSchemaFromString(t, testGQLSchema)
	mutation := `mutation {
		a: addPost(input: [{title: "A Post", author: {id: "0x1"}}]) {
			post {
				title
			}
		}
		b: addPost(input: [{title: "A Post", author: {id: "0x2"}}]) {
			post {
				title
			}
		}
	}`

	resp := resolveWithClient(gqlSchema, mutation, nil,
		&executor{
			queryTouched:    2,
			mutationTouched: 5,
		})

	require.NotNil(t, resp)
	require.Nil(t, resp.Errors)
	require.NotNil(t, resp.Extensions)

	// as both .Mutate() and .Query() should get called, so we should get their merged result
	require.Equal(t, uint64(14), resp.Extensions.TouchedUids)
	require.NotNil(t, resp.Extensions.Tracing)

	require.Equal(t, resp.Extensions.Tracing.Version, x.Version())
	require.True(t, len(resp.Extensions.Tracing.StartTime.String()) > 0)
	require.True(t, len(resp.Extensions.Tracing.EndTime.String()) > 0)
	require.True(t, resp.Extensions.Tracing.Duration > 0)

	require.Len(t, resp.Extensions.Tracing.Execution, 2)
	aliases := []string{"a", "b"}
	for i, execution := range resp.Extensions.Tracing.Execution {
		require.Equal(t, execution.Path, []interface{}{aliases[i]})
		require.Equal(t, execution.ParentType, "Mutation")
		require.Equal(t, execution.FieldName, aliases[i])
		require.Equal(t, execution.ReturnType, "AddPostPayload")
		require.True(t, execution.StartOffset > 0)
		require.True(t, execution.Duration > 0)

		require.Len(t, execution.Dgraph, 2)
		labels := []string{"mutation", "query"}
		for j, dgraphTrace := range execution.Dgraph {
			require.Equal(t, dgraphTrace.Label, labels[j])
			require.True(t, dgraphTrace.StartOffset > 0)
			require.True(t, dgraphTrace.Duration > 0)
		}
	}
}
