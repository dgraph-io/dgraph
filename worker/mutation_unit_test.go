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
	"context"
	"os"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/badger/v4"
	"github.com/dgraph-io/dgraph/v24/posting"
	"github.com/dgraph-io/dgraph/v24/protos/pb"
	"github.com/dgraph-io/dgraph/v24/schema"
	"github.com/dgraph-io/dgraph/v24/types"
	"github.com/dgraph-io/dgraph/v24/x"
)

func TestReverseEdge(t *testing.T) {
	dir, err := os.MkdirTemp("", "storetest_")
	x.Check(err)
	defer os.RemoveAll(dir)

	opt := badger.DefaultOptions(dir)
	ps, err := badger.OpenManaged(opt)
	x.Check(err)
	pstore = ps
	// Not using posting list cache
	posting.Init(ps, 0)
	Init(ps)
	err = schema.ParseBytes([]byte("revc: [uid] @reverse @count ."), 1)
	require.NoError(t, err)

	ctx := context.Background()
	txn := posting.Oracle().RegisterStartTs(5)
	attr := x.GalaxyAttr("revc")

	edge := &pb.DirectedEdge{
		ValueId: 2,
		Attr:    attr,
		Entity:  1,
		Op:      pb.DirectedEdge_DEL,
	}

	x.Check(runMutation(ctx, edge, txn))
	x.Check(runMutation(ctx, edge, txn))

	pl, err := txn.Get(x.DataKey(attr, 1))
	require.NoError(t, err)
	pl.RLock()
	c := pl.GetLength(5)
	pl.RUnlock()
	require.Equal(t, c, 0)
}

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
				Attr:  x.GalaxyAttr("name"),
			},
			to:        types.StringID,
			expectErr: false,
			output: &pb.DirectedEdge{
				Value:     []byte("set edge"),
				Attr:      x.GalaxyAttr("name"),
				ValueType: 9,
			},
		},
		{
			input: &pb.DirectedEdge{
				Value: []byte("set edge"),
				Attr:  x.NamespaceAttr(0xf2, "name"),
				Op:    pb.DirectedEdge_DEL,
			},
			to:        types.StringID,
			expectErr: false,
			output: &pb.DirectedEdge{
				Value:     []byte("set edge"),
				Attr:      x.NamespaceAttr(0xf2, "name"),
				Op:        pb.DirectedEdge_DEL,
				ValueType: 9,
			},
		},
		{
			input: &pb.DirectedEdge{
				ValueId: 123,
				Attr:    x.GalaxyAttr("name"),
			},
			to:        types.StringID,
			expectErr: true,
		},
		{
			input: &pb.DirectedEdge{
				Value: []byte("set edge"),
				Attr:  x.GalaxyAttr("name"),
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
		Attr:  x.GalaxyAttr("name"),
	}

	err := ValidateAndConvert(edge,
		&pb.SchemaUpdate{
			ValueType: pb.Posting_ValType(types.DateTimeID),
		})
	require.Error(t, err)
}

func TestTypeSanityCheck(t *testing.T) {
	// Empty field name check.
	typeDef := &pb.TypeUpdate{
		Fields: []*pb.SchemaUpdate{
			{
				Predicate: x.GalaxyAttr(""),
			},
		},
	}
	err := typeSanityCheck(typeDef)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Field in type definition must have a name")

	// Object type without object name.
	typeDef = &pb.TypeUpdate{
		Fields: []*pb.SchemaUpdate{
			{
				Predicate: x.GalaxyAttr("name"),
				ValueType: pb.Posting_OBJECT,
			},
		},
	}
	err = typeSanityCheck(typeDef)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Field with value type OBJECT must specify the name")

	// Field with directive.
	typeDef = &pb.TypeUpdate{
		Fields: []*pb.SchemaUpdate{
			{
				Predicate: x.GalaxyAttr("name"),
				Directive: pb.SchemaUpdate_REVERSE,
			},
		},
	}
	err = typeSanityCheck(typeDef)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Field in type definition cannot have a directive")

	// Field with tokenizer.
	typeDef = &pb.TypeUpdate{
		Fields: []*pb.SchemaUpdate{
			{
				Predicate: x.GalaxyAttr("name"),
				Tokenizer: []string{"int"},
			},
		},
	}
	err = typeSanityCheck(typeDef)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Field in type definition cannot have tokenizers")
}
