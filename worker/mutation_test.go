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
	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/types"
)

func TestConvertEdgeType(t *testing.T) {
	var testEdges = []struct {
		input     *intern.DirectedEdge
		to        types.TypeID
		expectErr bool
		output    *intern.DirectedEdge
	}{
		{
			input: &intern.DirectedEdge{
				Value: []byte("set edge"),
				Label: "test-mutation",
				Attr:  "name",
			},
			to:        types.StringID,
			expectErr: false,
			output: &intern.DirectedEdge{
				Value:     []byte("set edge"),
				Label:     "test-mutation",
				Attr:      "name",
				ValueType: 9,
			},
		},
		{
			input: &intern.DirectedEdge{
				Value: []byte("set edge"),
				Label: "test-mutation",
				Attr:  "name",
				Op:    intern.DirectedEdge_DEL,
			},
			to:        types.StringID,
			expectErr: true,
		},
		{
			input: &intern.DirectedEdge{
				ValueId: 123,
				Label:   "test-mutation",
				Attr:    "name",
			},
			to:        types.StringID,
			expectErr: true,
		},
		{
			input: &intern.DirectedEdge{
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
			&intern.SchemaUpdate{
				ValueType: intern.Posting_ValType(testEdge.to),
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
	edge := &intern.DirectedEdge{
		Value: []byte("set edge"),
		Label: "test-mutation",
		Attr:  "name",
	}

	err := ValidateAndConvert(edge,
		&intern.SchemaUpdate{
			ValueType: intern.Posting_ValType(types.DateTimeID),
		})
	require.Error(t, err)
}

func TestPopulateMutationMap(t *testing.T) {
	edges := []*intern.DirectedEdge{{
		Value: []byte("set edge"),
		Label: "test-mutation",
	}}
	schema := []*intern.SchemaUpdate{{
		Predicate: "name",
	}}
	m := &intern.Mutations{Edges: edges, Schema: schema}

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
	s1 := &intern.SchemaUpdate{Predicate: "name", ValueType: intern.Posting_UID}
	require.NoError(t, checkSchema(s1))

	// uid to non uid
	err := schema.ParseBytes([]byte("name:uid ."), 1)
	require.NoError(t, err)
	s1 = &intern.SchemaUpdate{Predicate: "name", ValueType: intern.Posting_STRING}
	require.NoError(t, checkSchema(s1))

	// string to password
	err = schema.ParseBytes([]byte("name:string ."), 1)
	require.NoError(t, err)
	s1 = &intern.SchemaUpdate{Predicate: "name", ValueType: intern.Posting_PASSWORD}
	require.Error(t, checkSchema(s1))

	// int to password
	err = schema.ParseBytes([]byte("name:int ."), 1)
	require.NoError(t, err)
	s1 = &intern.SchemaUpdate{Predicate: "name", ValueType: intern.Posting_PASSWORD}
	require.Error(t, checkSchema(s1))

	// password to password
	err = schema.ParseBytes([]byte("name:password ."), 1)
	require.NoError(t, err)
	s1 = &intern.SchemaUpdate{Predicate: "name", ValueType: intern.Posting_PASSWORD}
	require.NoError(t, checkSchema(s1))

	// string to int
	err = schema.ParseBytes([]byte("name:string ."), 1)
	require.NoError(t, err)
	s1 = &intern.SchemaUpdate{Predicate: "name", ValueType: intern.Posting_FLOAT}
	require.NoError(t, checkSchema(s1))

	// index on uid type
	s1 = &intern.SchemaUpdate{Predicate: "name", ValueType: intern.Posting_UID, Directive: intern.SchemaUpdate_INDEX}
	require.Error(t, checkSchema(s1))

	// reverse on non-uid type
	s1 = &intern.SchemaUpdate{Predicate: "name", ValueType: intern.Posting_STRING, Directive: intern.SchemaUpdate_REVERSE}
	require.Error(t, checkSchema(s1))

	s1 = &intern.SchemaUpdate{Predicate: "name", ValueType: intern.Posting_FLOAT, Directive: intern.SchemaUpdate_INDEX, Tokenizer: []string{"term"}}
	require.NoError(t, checkSchema(s1))

	s1 = &intern.SchemaUpdate{Predicate: "friend", ValueType: intern.Posting_UID, Directive: intern.SchemaUpdate_REVERSE}
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
	s1 := intern.SchemaUpdate{ValueType: intern.Posting_UID}
	s2 := intern.SchemaUpdate{ValueType: intern.Posting_UID}
	require.False(t, needReindexing(s1, s2))

	s1 = intern.SchemaUpdate{ValueType: intern.Posting_STRING, Directive: intern.SchemaUpdate_INDEX, Tokenizer: []string{"exact"}}
	s2 = intern.SchemaUpdate{ValueType: intern.Posting_STRING, Directive: intern.SchemaUpdate_INDEX, Tokenizer: []string{"exact"}}
	require.False(t, needReindexing(s1, s2))

	s1 = intern.SchemaUpdate{ValueType: intern.Posting_STRING, Directive: intern.SchemaUpdate_INDEX, Tokenizer: []string{"term"}}
	s2 = intern.SchemaUpdate{ValueType: intern.Posting_STRING, Directive: intern.SchemaUpdate_INDEX}
	require.True(t, needReindexing(s1, s2))

	s1 = intern.SchemaUpdate{ValueType: intern.Posting_STRING, Directive: intern.SchemaUpdate_INDEX, Tokenizer: []string{"exact"}}
	s2 = intern.SchemaUpdate{ValueType: intern.Posting_FLOAT, Directive: intern.SchemaUpdate_INDEX, Tokenizer: []string{"exact"}}
	require.True(t, needReindexing(s1, s2))

	s1 = intern.SchemaUpdate{ValueType: intern.Posting_STRING, Directive: intern.SchemaUpdate_INDEX, Tokenizer: []string{"exact"}}
	s2 = intern.SchemaUpdate{ValueType: intern.Posting_FLOAT, Directive: intern.SchemaUpdate_NONE}
	require.True(t, needReindexing(s1, s2))
}
