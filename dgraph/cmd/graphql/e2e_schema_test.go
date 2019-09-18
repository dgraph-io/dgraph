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

package graphql

import (
	"context"
	"testing"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

const (
	expectedSchema = `
{ 
	"schema": [ 
		{
			"predicate": "Author.country",
			"type": "uid"
		},
		{
			"predicate": "Author.dob",
			"type": "datetime",
			"index": true,
			"tokenizer": ["year"]
		},
		{
			"predicate": "Author.name",
			"type": "string",
			"index": true,
			"tokenizer": ["hash"]
		},
		{
			"predicate": "Author.posts",
			"type": "uid",
			"list": true
		},
		{
			"predicate": "Author.reputation",
			"type": "float",
			"index": true,
			"tokenizer": ["float"]
		},
		{
			"predicate": "Country.name",
			"type": "string",
			"index": true,
			"tokenizer": ["trigram"]
		},
		{
			"predicate": "Post.author",
			"type": "uid"
		},
		{
			"predicate": "Post.isPublished",
			"type": "bool",
			"index": true,
			"tokenizer": ["bool"]
		},
		{
			"predicate": "Post.numLikes",
			"type": "int",
			"index": true,
			"tokenizer": ["int"]
		},
		{
			"predicate": "Post.postType",
			"type": "string",
			"index": true,
			"tokenizer": ["exact"]
		},
		{
			"predicate": "Post.tags",
			"type": "string",
			"index": true,
			"tokenizer": ["exact"],
			"list": true
		},
		{
			"predicate": "Post.text",
			"type": "string",
			"index": true,
			"tokenizer": ["fulltext"]
		},
		{
			"predicate": "Post.title",
			"type": "string",
			"index": true,
			"tokenizer": ["term"]
		},
		{
			"predicate": "dgraph.graphql.date",
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
			"tokenizer": ["exact"],
			"list": true
		}
	],
	"types": [
		{
			"name": "Author",
			"fields": [
				{"name": "Author.name", "type": "string"},
				{"name": "Author.dob", "type": "datetime"},
				{"name": "Author.reputation", "type": "float"},
				{"name": "Author.country", "type": "uid"},
				{"name": "Author.posts", "type": "[uid]"}
			]
		},
		{
			"name": "Country",
			"fields": [
				{"name": "Country.name", "type": "string"}
			]
		},
		{
			"name": "Post",
			"fields": [
				{"name": "Post.title", "type": "string"},
				{"name": "Post.text", "type": "string"},
				{"name": "Post.tags", "type": "[string]"},
				{"name": "Post.numLikes", "type": "int"},
				{"name": "Post.isPublished", "type": "bool"},
				{"name": "Post.postType", "type": "string"},
				{"name": "Post.author", "type": "uid"}
			]
		}
	]
}`
)

func TestDgraphSchema(t *testing.T) {

	d, err := grpc.Dial(alphagRPC, grpc.WithInsecure())
	require.NoError(t, err)

	client := dgo.NewDgraphClient(api.NewDgraphClient(d))

	resp, err := client.NewReadOnlyTxn().Query(context.Background(), "schema {}")
	require.NoError(t, err)

	require.JSONEq(t, expectedSchema, string(resp.GetJson()))
}
