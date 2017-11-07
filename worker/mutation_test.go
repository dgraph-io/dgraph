/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package worker

import (
	"io/ioutil"
	"os"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/types"
)

func TestConvertEdgeType(t *testing.T) {
	var testEdges = []struct {
		input     *protos.DirectedEdge
		to        types.TypeID
		expectErr bool
		output    *protos.DirectedEdge
	}{
		{
			input: &protos.DirectedEdge{
				Value: []byte("set edge"),
				Label: "test-mutation",
				Attr:  "name",
			},
			to:        types.StringID,
			expectErr: false,
			output: &protos.DirectedEdge{
				Value:     []byte("set edge"),
				Label:     "test-mutation",
				Attr:      "name",
				ValueType: 9,
			},
		},
		{
			input: &protos.DirectedEdge{
				ValueId: 123,
				Label:   "test-mutation",
				Attr:    "name",
			},
			to:        types.StringID,
			expectErr: true,
		},
		{
			input: &protos.DirectedEdge{
				Value: []byte("set edge"),
				Label: "test-mutation",
				Attr:  "name",
			},
			to:        types.UidID,
			expectErr: true,
		},
	}

	for _, testEdge := range testEdges {
		err := ValidateAndConvert(testEdge.input, testEdge.to)
		if testEdge.expectErr {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			require.True(t, reflect.DeepEqual(testEdge.input, testEdge.output))
		}
	}

}

func TestValidateEdgeTypeError(t *testing.T) {
	edge := &protos.DirectedEdge{
		Value: []byte("set edge"),
		Label: "test-mutation",
		Attr:  "name",
	}

	err := ValidateAndConvert(edge, types.DateTimeID)
	require.Error(t, err)
}

func TestAddToMutationArray(t *testing.T) {
	dir, err := ioutil.TempDir("", "storetest_")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	mutationsMap := make(map[uint32]*protos.Mutations)
	edges := []*protos.DirectedEdge{}
	schema := []*protos.SchemaUpdate{}

	edges = append(edges, &protos.DirectedEdge{
		Value: []byte("set edge"),
		Label: "test-mutation",
	})
	schema = append(schema, &protos.SchemaUpdate{
		Predicate: "name",
	})
	m := &protos.Mutations{Edges: edges, Schema: schema}

	err = addToMutationMap(mutationsMap, m)
	require.NoError(t, err)
	mu := mutationsMap[1]
	require.NotNil(t, mu)
	require.NotNil(t, mu.Edges)
}

func TestCheckSchema(t *testing.T) {
	dir, _ := initTest(t, "name:string @index(term) .")
	defer os.RemoveAll(dir)
	// non uid to uid
	s1 := &protos.SchemaUpdate{Predicate: "name", ValueType: protos.Posting_UID}
	require.NoError(t, checkSchema(s1))

	// uid to non uid
	err := schema.ParseBytes([]byte("name:uid ."), 1)
	require.NoError(t, err)
	s1 = &protos.SchemaUpdate{Predicate: "name", ValueType: protos.Posting_STRING}
	require.NoError(t, checkSchema(s1))

	// string to int
	err = schema.ParseBytes([]byte("name:string ."), 1)
	require.NoError(t, err)
	s1 = &protos.SchemaUpdate{Predicate: "name", ValueType: protos.Posting_FLOAT}
	require.NoError(t, checkSchema(s1))

	// index on uid type
	s1 = &protos.SchemaUpdate{Predicate: "name", ValueType: protos.Posting_UID, Directive: protos.SchemaUpdate_INDEX}
	require.Error(t, checkSchema(s1))

	// reverse on non-uid type
	s1 = &protos.SchemaUpdate{Predicate: "name", ValueType: protos.Posting_STRING, Directive: protos.SchemaUpdate_REVERSE}
	require.Error(t, checkSchema(s1))

	s1 = &protos.SchemaUpdate{Predicate: "name", ValueType: protos.Posting_FLOAT, Directive: protos.SchemaUpdate_INDEX, Tokenizer: []string{"term"}}
	require.NoError(t, checkSchema(s1))

	s1 = &protos.SchemaUpdate{Predicate: "friend", ValueType: protos.Posting_UID, Directive: protos.SchemaUpdate_REVERSE}
	require.NoError(t, checkSchema(s1))
}

func TestNeedReindexing(t *testing.T) {
	s1 := protos.SchemaUpdate{ValueType: protos.Posting_UID}
	s2 := protos.SchemaUpdate{ValueType: protos.Posting_UID}
	require.False(t, needReindexing(s1, s2))

	s1 = protos.SchemaUpdate{ValueType: protos.Posting_STRING, Directive: protos.SchemaUpdate_INDEX, Tokenizer: []string{"exact"}}
	s2 = protos.SchemaUpdate{ValueType: protos.Posting_STRING, Directive: protos.SchemaUpdate_INDEX, Tokenizer: []string{"exact"}}
	require.False(t, needReindexing(s1, s2))

	s1 = protos.SchemaUpdate{ValueType: protos.Posting_STRING, Directive: protos.SchemaUpdate_INDEX, Tokenizer: []string{"term"}}
	s2 = protos.SchemaUpdate{ValueType: protos.Posting_STRING, Directive: protos.SchemaUpdate_INDEX}
	require.True(t, needReindexing(s1, s2))

	s1 = protos.SchemaUpdate{ValueType: protos.Posting_STRING, Directive: protos.SchemaUpdate_INDEX, Tokenizer: []string{"exact"}}
	s2 = protos.SchemaUpdate{ValueType: protos.Posting_FLOAT, Directive: protos.SchemaUpdate_INDEX, Tokenizer: []string{"exact"}}
	require.True(t, needReindexing(s1, s2))

	s1 = protos.SchemaUpdate{ValueType: protos.Posting_STRING, Directive: protos.SchemaUpdate_INDEX, Tokenizer: []string{"exact"}}
	s2 = protos.SchemaUpdate{ValueType: protos.Posting_FLOAT, Directive: protos.SchemaUpdate_NONE}
	require.True(t, needReindexing(s1, s2))
}
