/*
* Copyright 2016 DGraph Labs, Inc.
*
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You may obtain a copy of the License at
*
* 		http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
 */

package worker

import (
	"io/ioutil"
	"os"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/protos/graphp"
	"github.com/dgraph-io/dgraph/protos/taskp"
	"github.com/dgraph-io/dgraph/protos/typesp"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/types"
)

func TestConvertEdgeType(t *testing.T) {
	var testEdges = []struct {
		input     *taskp.DirectedEdge
		to        types.TypeID
		expectErr bool
		output    *taskp.DirectedEdge
	}{
		{
			input: &taskp.DirectedEdge{
				Value: []byte("set edge"),
				Label: "test-mutation",
				Attr:  "name",
			},
			to:        types.StringID,
			expectErr: false,
			output: &taskp.DirectedEdge{
				Value:     []byte("set edge"),
				Label:     "test-mutation",
				Attr:      "name",
				ValueType: 10,
			},
		},
		{
			input: &taskp.DirectedEdge{
				ValueId: 123,
				Label:   "test-mutation",
				Attr:    "name",
			},
			to:        types.StringID,
			expectErr: true,
		},
		{
			input: &taskp.DirectedEdge{
				Value: []byte("set edge"),
				Label: "test-mutation",
				Attr:  "name",
			},
			to:        types.UidID,
			expectErr: true,
		},
	}

	for _, testEdge := range testEdges {
		err := validateAndConvert(testEdge.input, testEdge.to)
		if testEdge.expectErr {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			require.True(t, reflect.DeepEqual(testEdge.input, testEdge.output))
		}
	}

}

func TestValidateEdgeTypeError(t *testing.T) {
	edge := &taskp.DirectedEdge{
		Value: []byte("set edge"),
		Label: "test-mutation",
		Attr:  "name",
	}

	err := validateAndConvert(edge, types.DateTimeID)
	require.Error(t, err)
}

func TestAddToMutationArray(t *testing.T) {
	group.ParseGroupConfig("")
	dir, err := ioutil.TempDir("", "storetest_")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	mutationsMap := make(map[uint32]*taskp.Mutations)
	edges := []*taskp.DirectedEdge{}
	schema := []*graphp.SchemaUpdate{}

	edges = append(edges, &taskp.DirectedEdge{
		Value: []byte("set edge"),
		Label: "test-mutation",
	})
	schema = append(schema, &graphp.SchemaUpdate{
		Predicate: "name",
	})
	m := &taskp.Mutations{Edges: edges, Schema: schema}

	addToMutationMap(mutationsMap, m)
	mu := mutationsMap[1]
	require.NotNil(t, mu)
	require.NotNil(t, mu.Edges)
}

func TestCheckSchema(t *testing.T) {
	// non uid to uid
	err := schema.ParseBytes([]byte("name:string @index"), 1)
	require.NoError(t, err)
	s1 := &graphp.SchemaUpdate{Predicate: "name", ValueType: uint32(types.UidID)}
	require.Error(t, checkSchema(s1))

	// uid to non uid
	err = schema.ParseBytes([]byte("name:uid"), 1)
	require.NoError(t, err)
	s1 = &graphp.SchemaUpdate{Predicate: "name", ValueType: uint32(types.StringID)}
	require.Error(t, checkSchema(s1))

	// string to int
	err = schema.ParseBytes([]byte("name:string"), 1)
	require.NoError(t, err)
	s1 = &graphp.SchemaUpdate{Predicate: "name", ValueType: uint32(types.FloatID)}
	require.NoError(t, checkSchema(s1))

	// index on uid type
	s1 = &graphp.SchemaUpdate{Predicate: "name", ValueType: uint32(types.UidID), Directive: graphp.SchemaUpdate_INDEX}
	require.Error(t, checkSchema(s1))

	// reverse on non-uid type
	s1 = &graphp.SchemaUpdate{Predicate: "name", ValueType: uint32(types.StringID), Directive: graphp.SchemaUpdate_REVERSE}
	require.Error(t, checkSchema(s1))

	s1 = &graphp.SchemaUpdate{Predicate: "name", ValueType: uint32(types.FloatID), Directive: graphp.SchemaUpdate_INDEX}
	require.NoError(t, checkSchema(s1))

	s1 = &graphp.SchemaUpdate{Predicate: "friend", ValueType: uint32(types.UidID), Directive: graphp.SchemaUpdate_REVERSE}
	require.NoError(t, checkSchema(s1))
}

func TestNeedReindexing(t *testing.T) {
	s1 := typesp.Schema{ValueType: uint32(types.UidID)}
	s2 := typesp.Schema{ValueType: uint32(types.UidID)}
	require.False(t, needReindexing(s1, s2))

	s1 = typesp.Schema{ValueType: uint32(types.StringID), Directive: typesp.Schema_INDEX, Tokenizer: []string{"exact"}}
	s2 = typesp.Schema{ValueType: uint32(types.StringID), Directive: typesp.Schema_INDEX, Tokenizer: []string{"exact"}}
	require.False(t, needReindexing(s1, s2))

	s1 = typesp.Schema{ValueType: uint32(types.StringID), Directive: typesp.Schema_INDEX, Tokenizer: []string{"term"}}
	s2 = typesp.Schema{ValueType: uint32(types.StringID), Directive: typesp.Schema_INDEX}
	require.True(t, needReindexing(s1, s2))

	s1 = typesp.Schema{ValueType: uint32(types.StringID), Directive: typesp.Schema_INDEX, Tokenizer: []string{"exact"}}
	s2 = typesp.Schema{ValueType: uint32(types.FloatID), Directive: typesp.Schema_INDEX, Tokenizer: []string{"exact"}}
	require.True(t, needReindexing(s1, s2))

	s1 = typesp.Schema{ValueType: uint32(types.StringID), Directive: typesp.Schema_INDEX, Tokenizer: []string{"exact"}}
	s2 = typesp.Schema{ValueType: uint32(types.FloatID), Directive: typesp.Schema_NONE}
	require.True(t, needReindexing(s1, s2))
}
