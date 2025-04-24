//go:build integration

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hypermodeinc/dgraph/v25/posting"
	"github.com/hypermodeinc/dgraph/v25/protos/pb"
	"github.com/hypermodeinc/dgraph/v25/schema"
	"github.com/hypermodeinc/dgraph/v25/x"
)

func TestPopulateMutationMap(t *testing.T) {
	edges := []*pb.DirectedEdge{{
		Value: []byte("set edge"),
		Attr:  x.AttrInRootNamespace(""),
	}}
	schema := []*pb.SchemaUpdate{{
		Predicate: x.AttrInRootNamespace("name"),
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
	s1 := &pb.SchemaUpdate{Predicate: x.AttrInRootNamespace("name"), ValueType: pb.Posting_UID}
	require.NoError(t, checkSchema(s1))

	// uid to non uid
	require.NoError(t, schema.ParseBytes([]byte("name:uid ."), 1))
	s1 = &pb.SchemaUpdate{Predicate: x.AttrInRootNamespace("name"), ValueType: pb.Posting_STRING}
	require.NoError(t, checkSchema(s1))

	// string to password
	require.NoError(t, schema.ParseBytes([]byte("name:string ."), 1))
	s1 = &pb.SchemaUpdate{Predicate: x.AttrInRootNamespace("name"), ValueType: pb.Posting_PASSWORD}
	require.Error(t, checkSchema(s1))

	// password to string
	require.NoError(t, schema.ParseBytes([]byte("name:password ."), 1))
	s1 = &pb.SchemaUpdate{Predicate: x.AttrInRootNamespace("name"), ValueType: pb.Posting_STRING}
	require.Error(t, checkSchema(s1))

	// int to password
	require.NoError(t, schema.ParseBytes([]byte("name:int ."), 1))
	s1 = &pb.SchemaUpdate{Predicate: x.AttrInRootNamespace("name"), ValueType: pb.Posting_PASSWORD}
	require.Error(t, checkSchema(s1))

	// password to password
	require.NoError(t, schema.ParseBytes([]byte("name:password ."), 1))
	s1 = &pb.SchemaUpdate{Predicate: x.AttrInRootNamespace("name"), ValueType: pb.Posting_PASSWORD}
	require.NoError(t, checkSchema(s1))

	// string to int
	require.NoError(t, schema.ParseBytes([]byte("name:string ."), 1))
	s1 = &pb.SchemaUpdate{Predicate: x.AttrInRootNamespace("name"), ValueType: pb.Posting_FLOAT}
	require.NoError(t, checkSchema(s1))

	// index on uid type
	s1 = &pb.SchemaUpdate{Predicate: x.AttrInRootNamespace("name"), ValueType: pb.Posting_UID, Directive: pb.SchemaUpdate_INDEX}
	require.Error(t, checkSchema(s1))

	// reverse on non-uid type
	s1 = &pb.SchemaUpdate{
		Predicate: x.AttrInRootNamespace("name"),
		ValueType: pb.Posting_STRING,
		Directive: pb.SchemaUpdate_REVERSE,
	}
	require.Error(t, checkSchema(s1))

	s1 = &pb.SchemaUpdate{
		Predicate: x.AttrInRootNamespace("name"),
		ValueType: pb.Posting_FLOAT,
		Directive: pb.SchemaUpdate_INDEX,
		Tokenizer: []string{"term"},
	}
	require.NoError(t, checkSchema(s1))

	s1 = &pb.SchemaUpdate{
		Predicate: x.AttrInRootNamespace("friend"),
		ValueType: pb.Posting_UID,
		Directive: pb.SchemaUpdate_REVERSE,
	}
	require.NoError(t, checkSchema(s1))

	// Schema with internal predicate.
	s1 = &pb.SchemaUpdate{Predicate: x.AttrInRootNamespace("uid"), ValueType: pb.Posting_STRING}
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
