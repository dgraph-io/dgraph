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
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/types"
)

func TestConvertEdgeType(t *testing.T) {
	var testEdges = []struct {
		input     *task.DirectedEdge
		to        types.TypeID
		expectErr bool
		output    *task.DirectedEdge
	}{
		{
			input: &task.DirectedEdge{
				Value: []byte("set edge"),
				Label: "test-mutation",
				Attr:  "name",
			},
			to:        types.StringID,
			expectErr: false,
			output: &task.DirectedEdge{
				Value:     []byte("set edge"),
				Label:     "test-mutation",
				Attr:      "name",
				ValueType: 10,
			},
		},
		{
			input: &task.DirectedEdge{
				ValueId: 123,
				Label:   "test-mutation",
				Attr:    "name",
			},
			to:        types.StringID,
			expectErr: true,
		},
		{
			input: &task.DirectedEdge{
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
	edge := &task.DirectedEdge{
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

	mutationsMap := make(map[uint32]*task.Mutations)
	edges := []*task.DirectedEdge{}

	edges = append(edges, &task.DirectedEdge{
		Value: []byte("set edge"),
		Label: "test-mutation",
	})

	addToMutationMap(mutationsMap, edges)
	mu := mutationsMap[1]
	require.NotNil(t, mu)
	require.NotNil(t, mu.Edges)
}
