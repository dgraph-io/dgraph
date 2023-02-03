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

package resolve

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/graphql/test"
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

	require.Equal(t, resp.Extensions.Tracing.Version, 1)
	_, err := time.Parse(time.RFC3339Nano, resp.Extensions.Tracing.StartTime)
	require.NoError(t, err)
	_, err = time.Parse(time.RFC3339Nano, resp.Extensions.Tracing.EndTime)
	require.NoError(t, err)
	require.True(t, resp.Extensions.Tracing.Duration > 0)
	require.NotNil(t, resp.Extensions.Tracing.Execution)

	require.Len(t, resp.Extensions.Tracing.Execution.Resolvers, 1)
	require.Equal(t, resp.Extensions.Tracing.Execution.Resolvers[0].Path, []interface{}{"getAuthor"})
	require.Equal(t, resp.Extensions.Tracing.Execution.Resolvers[0].ParentType, "Query")
	require.Equal(t, resp.Extensions.Tracing.Execution.Resolvers[0].FieldName, "getAuthor")
	require.Equal(t, resp.Extensions.Tracing.Execution.Resolvers[0].ReturnType, "Author")
	require.True(t, resp.Extensions.Tracing.Execution.Resolvers[0].StartOffset > 0)
	require.True(t, resp.Extensions.Tracing.Execution.Resolvers[0].Duration > 0)

	require.Len(t, resp.Extensions.Tracing.Execution.Resolvers[0].Dgraph, 1)
	require.Equal(t, resp.Extensions.Tracing.Execution.Resolvers[0].Dgraph[0].Label, "query")
	require.True(t, resp.Extensions.Tracing.Execution.Resolvers[0].Dgraph[0].StartOffset > 0)
	require.True(t, resp.Extensions.Tracing.Execution.Resolvers[0].Dgraph[0].Duration > 0)

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

	require.Equal(t, resp.Extensions.Tracing.Version, 1)
	_, err := time.Parse(time.RFC3339Nano, resp.Extensions.Tracing.StartTime)
	require.NoError(t, err)
	_, err = time.Parse(time.RFC3339Nano, resp.Extensions.Tracing.EndTime)
	require.NoError(t, err)
	require.True(t, resp.Extensions.Tracing.Duration > 0)
	require.NotNil(t, resp.Extensions.Tracing.Execution)

	require.Len(t, resp.Extensions.Tracing.Execution.Resolvers, 3)
	aliases := []string{"a", "b", "c"}
	for i, resolver := range resp.Extensions.Tracing.Execution.Resolvers {
		require.Equal(t, resolver.Path, []interface{}{aliases[i]})
		require.Equal(t, resolver.ParentType, "Query")
		require.Equal(t, resolver.FieldName, aliases[i])
		require.Equal(t, resolver.ReturnType, "Author")
		require.True(t, resolver.StartOffset > 0)
		require.True(t, resolver.Duration > 0)
		require.Len(t, resolver.Dgraph, 1)
		require.Equal(t, resolver.Dgraph[0].Label, "query")
		require.True(t, resolver.Dgraph[0].StartOffset > 0)
		require.True(t, resolver.Dgraph[0].Duration > 0)
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
			assigned:             map[string]string{"Post_2": "0x2"},
			existenceQueriesResp: `{ "Author_1": [{"uid":"0x1", "dgraph.type": ["Author"]}]}`,
			queryTouched:         2,
			mutationTouched:      5,
		})

	require.NotNil(t, resp)
	require.Nilf(t, resp.Errors, "%v", resp.Errors)
	require.NotNil(t, resp.Extensions)

	// as both .Mutate() and .Query() should get called, so we should get their merged result
	require.Equal(t, uint64(7), resp.Extensions.TouchedUids)
	require.NotNil(t, resp.Extensions.Tracing)

	require.Equal(t, resp.Extensions.Tracing.Version, 1)
	_, err := time.Parse(time.RFC3339Nano, resp.Extensions.Tracing.StartTime)
	require.NoError(t, err)
	_, err = time.Parse(time.RFC3339Nano, resp.Extensions.Tracing.EndTime)
	require.NoError(t, err)
	require.True(t, resp.Extensions.Tracing.Duration > 0)
	require.NotNil(t, resp.Extensions.Tracing.Execution)

	require.Len(t, resp.Extensions.Tracing.Execution.Resolvers, 1)
	require.Equal(t, resp.Extensions.Tracing.Execution.Resolvers[0].Path, []interface{}{"addPost"})
	require.Equal(t, resp.Extensions.Tracing.Execution.Resolvers[0].ParentType, "Mutation")
	require.Equal(t, resp.Extensions.Tracing.Execution.Resolvers[0].FieldName, "addPost")
	require.Equal(t, resp.Extensions.Tracing.Execution.Resolvers[0].ReturnType, "AddPostPayload")
	require.True(t, resp.Extensions.Tracing.Execution.Resolvers[0].StartOffset > 0)
	require.True(t, resp.Extensions.Tracing.Execution.Resolvers[0].Duration > 0)

	require.Len(t, resp.Extensions.Tracing.Execution.Resolvers[0].Dgraph, 3)
	labels := []string{"preMutationQuery", "mutation", "query"}
	for i, dgraphTrace := range resp.Extensions.Tracing.Execution.Resolvers[0].Dgraph {
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
			assigned:             map[string]string{"Post_2": "0x2"},
			existenceQueriesResp: `{ "Author_1": [{"uid":"0x1", "dgraph.type": ["Author"]}]}`,
			queryTouched:         2,
			mutationTouched:      5,
		})

	require.NotNil(t, resp)
	require.Nilf(t, resp.Errors, "%v", resp.Errors)
	require.NotNil(t, resp.Extensions)

	// as both .Mutate() and .Query() should get called, so we should get their merged result
	require.Equal(t, uint64(14), resp.Extensions.TouchedUids)
	require.NotNil(t, resp.Extensions.Tracing)

	require.Equal(t, resp.Extensions.Tracing.Version, 1)
	_, err := time.Parse(time.RFC3339Nano, resp.Extensions.Tracing.StartTime)
	require.NoError(t, err)
	_, err = time.Parse(time.RFC3339Nano, resp.Extensions.Tracing.EndTime)
	require.NoError(t, err)
	require.True(t, resp.Extensions.Tracing.Duration > 0)
	require.NotNil(t, resp.Extensions.Tracing.Execution)

	require.Len(t, resp.Extensions.Tracing.Execution.Resolvers, 2)
	aliases := []string{"a", "b"}
	for i, resolver := range resp.Extensions.Tracing.Execution.Resolvers {
		require.Equal(t, resolver.Path, []interface{}{aliases[i]})
		require.Equal(t, resolver.ParentType, "Mutation")
		require.Equal(t, resolver.FieldName, aliases[i])
		require.Equal(t, resolver.ReturnType, "AddPostPayload")
		require.True(t, resolver.StartOffset > 0)
		require.True(t, resolver.Duration > 0)

		require.Len(t, resolver.Dgraph, 3)
		labels := []string{"preMutationQuery", "mutation", "query"}
		for j, dgraphTrace := range resolver.Dgraph {
			require.Equal(t, dgraphTrace.Label, labels[j])
			require.True(t, dgraphTrace.StartOffset > 0)
			require.True(t, dgraphTrace.Duration > 0)
		}
	}
}
