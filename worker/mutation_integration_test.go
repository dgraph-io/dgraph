//go:build integration

/*
 * Copyright 2016-2023 Dgraph Labs, Inc. and Contributors
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

package worker

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v24/posting"
	"github.com/dgraph-io/dgraph/v24/protos/pb"
	"github.com/dgraph-io/dgraph/v24/schema"
	"github.com/dgraph-io/dgraph/v24/x"
)

func TestPopulateMutationMap(t *testing.T) {
	edges := []*pb.DirectedEdge{{
		Value: []byte("set edge"),
		Attr:  x.GalaxyAttr(""),
	}}
	schema := []*pb.SchemaUpdate{{
		Predicate: x.GalaxyAttr("name"),
	}}
	m := &pb.Mutations{Edges: edges, Schema: schema}

	mutationsMap, err := populateMutationMap(m)
	require.NoError(t, err)
	mu := mutationsMap[1]
	require.NotNil(t, mu)
	require.NotNil(t, mu.Edges)
	require.NotNil(t, mu.Schema)
}

func TestCheckSchema(t *testing.T) {
	require.NoError(t, posting.DeleteAll())
	initTest(t, "name:string @index(term) .")
	// non uid to uid
	s1 := &pb.SchemaUpdate{Predicate: x.GalaxyAttr("name"), ValueType: pb.Posting_UID}
	require.NoError(t, checkSchema(s1))

	// uid to non uid
	require.NoError(t, schema.ParseBytes([]byte("name:uid ."), 1))
	s1 = &pb.SchemaUpdate{Predicate: x.GalaxyAttr("name"), ValueType: pb.Posting_STRING}
	require.NoError(t, checkSchema(s1))

	// string to password
	require.NoError(t, schema.ParseBytes([]byte("name:string ."), 1))
	s1 = &pb.SchemaUpdate{Predicate: x.GalaxyAttr("name"), ValueType: pb.Posting_PASSWORD}
	require.Error(t, checkSchema(s1))

	// password to string
	require.NoError(t, schema.ParseBytes([]byte("name:password ."), 1))
	s1 = &pb.SchemaUpdate{Predicate: x.GalaxyAttr("name"), ValueType: pb.Posting_STRING}
	require.Error(t, checkSchema(s1))

	// int to password
	require.NoError(t, schema.ParseBytes([]byte("name:int ."), 1))
	s1 = &pb.SchemaUpdate{Predicate: x.GalaxyAttr("name"), ValueType: pb.Posting_PASSWORD}
	require.Error(t, checkSchema(s1))

	// password to password
	require.NoError(t, schema.ParseBytes([]byte("name:password ."), 1))
	s1 = &pb.SchemaUpdate{Predicate: x.GalaxyAttr("name"), ValueType: pb.Posting_PASSWORD}
	require.NoError(t, checkSchema(s1))

	// string to int
	require.NoError(t, schema.ParseBytes([]byte("name:string ."), 1))
	s1 = &pb.SchemaUpdate{Predicate: x.GalaxyAttr("name"), ValueType: pb.Posting_FLOAT}
	require.NoError(t, checkSchema(s1))

	// index on uid type
	s1 = &pb.SchemaUpdate{Predicate: x.GalaxyAttr("name"), ValueType: pb.Posting_UID, Directive: pb.SchemaUpdate_INDEX}
	require.Error(t, checkSchema(s1))

	// reverse on non-uid type
	s1 = &pb.SchemaUpdate{
		Predicate: x.GalaxyAttr("name"),
		ValueType: pb.Posting_STRING,
		Directive: pb.SchemaUpdate_REVERSE,
	}
	require.Error(t, checkSchema(s1))

	s1 = &pb.SchemaUpdate{
		Predicate: x.GalaxyAttr("name"),
		ValueType: pb.Posting_FLOAT,
		Directive: pb.SchemaUpdate_INDEX,
		Tokenizer: []string{"term"},
	}
	require.NoError(t, checkSchema(s1))

	s1 = &pb.SchemaUpdate{
		Predicate: x.GalaxyAttr("friend"),
		ValueType: pb.Posting_UID,
		Directive: pb.SchemaUpdate_REVERSE,
	}
	require.NoError(t, checkSchema(s1))

	// Schema with internal predicate.
	s1 = &pb.SchemaUpdate{Predicate: x.GalaxyAttr("uid"), ValueType: pb.Posting_STRING}
	require.Error(t, checkSchema(s1))

	s := `jobs: string @upsert .`
	result, err := schema.Parse(s)
	require.NoError(t, err)
	err = checkSchema(result.Preds[0])
	require.Error(t, err)
	require.Equal(t, "Index tokenizer is mandatory for: [jobs] when specifying @upsert directive", err.Error())

	s = `
		jobs : string @index(exact) @upsert .
		age  : int @index(int) @upsert .
	`
	result, err = schema.Parse(s)
	require.NoError(t, err)
	require.NoError(t, checkSchema(result.Preds[0]))
	require.NoError(t, checkSchema(result.Preds[1]))
}
