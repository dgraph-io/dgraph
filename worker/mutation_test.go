/*
 * Copyright 2016-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package worker

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/types"
)

func TestConvertEdgeType(t *testing.T) {
	var testEdges = []struct {
		input     *pb.DirectedEdge
		to        types.TypeID
		expectErr bool
		output    *pb.DirectedEdge
	}{
		{
			input: &pb.DirectedEdge{
				Value: []byte("set edge"),
				Label: "test-mutation",
				Attr:  "name",
			},
			to:        types.StringID,
			expectErr: false,
			output: &pb.DirectedEdge{
				Value:     []byte("set edge"),
				Label:     "test-mutation",
				Attr:      "name",
				ValueType: 9,
			},
		},
		{
			input: &pb.DirectedEdge{
				Value: []byte("set edge"),
				Label: "test-mutation",
				Attr:  "name",
				Op:    pb.DirectedEdge_DEL,
			},
			to:        types.StringID,
			expectErr: true,
		},
		{
			input: &pb.DirectedEdge{
				ValueId: 123,
				Label:   "test-mutation",
				Attr:    "name",
			},
			to:        types.StringID,
			expectErr: true,
		},
		{
			input: &pb.DirectedEdge{
				Value: []byte("set edge"),
				Label: "test-mutation",
				Attr:  "name",
			},
			to:        types.UidID,
			expectErr: true,
		},
	}

	for _, testEdge := range testEdges {
		err := ValidateAndConvert(testEdge.input,
			&pb.SchemaUpdate{
				ValueType: pb.Posting_ValType(testEdge.to),
			})
		if testEdge.expectErr {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			require.True(t, reflect.DeepEqual(testEdge.input, testEdge.output))
		}
	}

}

func TestValidateEdgeTypeError(t *testing.T) {
	edge := &pb.DirectedEdge{
		Value: []byte("set edge"),
		Label: "test-mutation",
		Attr:  "name",
	}

	err := ValidateAndConvert(edge,
		&pb.SchemaUpdate{
			ValueType: pb.Posting_ValType(types.DateTimeID),
		})
	require.Error(t, err)
}

func TestPopulateMutationMap(t *testing.T) {
	edges := []*pb.DirectedEdge{{
		Value: []byte("set edge"),
		Label: "test-mutation",
	}}
	schema := []*pb.SchemaUpdate{{
		Predicate: "name",
	}}
	m := &pb.Mutations{Edges: edges, Schema: schema}

	mutationsMap := populateMutationMap(m)
	mu := mutationsMap[1]
	require.NotNil(t, mu)
	require.NotNil(t, mu.Edges)
	require.NotNil(t, mu.Schema)
}

func TestCheckSchema(t *testing.T) {
	posting.DeleteAll()
	initTest(t, "name:string @index(term) .")
	// non uid to uid
	s1 := &pb.SchemaUpdate{Predicate: "name", ValueType: pb.Posting_UID}
	require.NoError(t, checkSchema(s1))

	// uid to non uid
	err := schema.ParseBytes([]byte("name:uid ."), 1)
	require.NoError(t, err)
	s1 = &pb.SchemaUpdate{Predicate: "name", ValueType: pb.Posting_STRING}
	require.NoError(t, checkSchema(s1))

	// string to password
	err = schema.ParseBytes([]byte("name:string ."), 1)
	require.NoError(t, err)
	s1 = &pb.SchemaUpdate{Predicate: "name", ValueType: pb.Posting_PASSWORD}
	require.Error(t, checkSchema(s1))

	// int to password
	err = schema.ParseBytes([]byte("name:int ."), 1)
	require.NoError(t, err)
	s1 = &pb.SchemaUpdate{Predicate: "name", ValueType: pb.Posting_PASSWORD}
	require.Error(t, checkSchema(s1))

	// password to password
	err = schema.ParseBytes([]byte("name:password ."), 1)
	require.NoError(t, err)
	s1 = &pb.SchemaUpdate{Predicate: "name", ValueType: pb.Posting_PASSWORD}
	require.NoError(t, checkSchema(s1))

	// string to int
	err = schema.ParseBytes([]byte("name:string ."), 1)
	require.NoError(t, err)
	s1 = &pb.SchemaUpdate{Predicate: "name", ValueType: pb.Posting_FLOAT}
	require.NoError(t, checkSchema(s1))

	// index on uid type
	s1 = &pb.SchemaUpdate{Predicate: "name", ValueType: pb.Posting_UID, Directive: pb.SchemaUpdate_INDEX}
	require.Error(t, checkSchema(s1))

	// reverse on non-uid type
	s1 = &pb.SchemaUpdate{Predicate: "name", ValueType: pb.Posting_STRING, Directive: pb.SchemaUpdate_REVERSE}
	require.Error(t, checkSchema(s1))

	s1 = &pb.SchemaUpdate{Predicate: "name", ValueType: pb.Posting_FLOAT, Directive: pb.SchemaUpdate_INDEX, Tokenizer: []string{"term"}}
	require.NoError(t, checkSchema(s1))

	s1 = &pb.SchemaUpdate{Predicate: "friend", ValueType: pb.Posting_UID, Directive: pb.SchemaUpdate_REVERSE}
	require.NoError(t, checkSchema(s1))

	s := `jobs: string @upsert .`
	su, err := schema.Parse(s)
	require.NoError(t, err)
	err = checkSchema(su[0])
	require.Error(t, err)
	require.Equal(t, "Index tokenizer is mandatory for: [jobs] when specifying @upsert directive", err.Error())

	s = `
		jobs : string @index(exact) @upsert .
		age  : int @index(int) @upsert .
	`
	su, err = schema.Parse(s)
	require.NoError(t, err)
	err = checkSchema(su[0])
	require.NoError(t, err)
	err = checkSchema(su[1])
	require.NoError(t, err)
}

func TestNeedReindexing(t *testing.T) {
	s1 := pb.SchemaUpdate{ValueType: pb.Posting_UID}
	s2 := pb.SchemaUpdate{ValueType: pb.Posting_UID}
	require.False(t, needReindexing(s1, s2))

	s1 = pb.SchemaUpdate{ValueType: pb.Posting_STRING, Directive: pb.SchemaUpdate_INDEX, Tokenizer: []string{"exact"}}
	s2 = pb.SchemaUpdate{ValueType: pb.Posting_STRING, Directive: pb.SchemaUpdate_INDEX, Tokenizer: []string{"exact"}}
	require.False(t, needReindexing(s1, s2))

	s1 = pb.SchemaUpdate{ValueType: pb.Posting_STRING, Directive: pb.SchemaUpdate_INDEX, Tokenizer: []string{"term"}}
	s2 = pb.SchemaUpdate{ValueType: pb.Posting_STRING, Directive: pb.SchemaUpdate_INDEX}
	require.True(t, needReindexing(s1, s2))

	s1 = pb.SchemaUpdate{ValueType: pb.Posting_STRING, Directive: pb.SchemaUpdate_INDEX, Tokenizer: []string{"exact"}}
	s2 = pb.SchemaUpdate{ValueType: pb.Posting_FLOAT, Directive: pb.SchemaUpdate_INDEX, Tokenizer: []string{"exact"}}
	require.True(t, needReindexing(s1, s2))

	s1 = pb.SchemaUpdate{ValueType: pb.Posting_STRING, Directive: pb.SchemaUpdate_INDEX, Tokenizer: []string{"exact"}}
	s2 = pb.SchemaUpdate{ValueType: pb.Posting_FLOAT, Directive: pb.SchemaUpdate_NONE}
	require.True(t, needReindexing(s1, s2))
}
