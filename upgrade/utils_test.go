/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
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

package upgrade

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"

	"github.com/stretchr/testify/require"
)

func Test_askUserForNewName(t *testing.T) {
	want := "User"
	r := strings.NewReader("dgraph.Person\nPerson\n" + want + "\n")
	w := bytes.NewBuffer(nil)

	got := askUserForNewName(r, w, "draph.User", x.IsReservedType,
		map[string]struct{}{"Person": {}})
	require.Equal(t, want, got)

	require.Equal(t, "Enter new name for `draph.User`: new name can't start with `dgraph.`, "+
		"please try again! \nEnter new name for `draph.User`: new name can't be same as a name in "+
		"existing schema, please try again! \nEnter new name for `draph.User`: ", w.String())
}

func Test_getPredSchemaString(t *testing.T) {
	type args struct {
		newPredName string
		schemaNode  *pb.SchemaNode
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "only type in schema node",
			args: args{
				newPredName: "age",
				schemaNode:  &pb.SchemaNode{Type: "int"},
			},
			want: "age: int .\n",
		}, {
			name: "type with one index in schema node",
			args: args{
				newPredName: "name",
				schemaNode: &pb.SchemaNode{
					Type:      "string",
					Index:     true,
					Tokenizer: []string{"exact"},
				},
			},
			want: "name: string @index(exact) .\n",
		}, {
			name: "full schema node with multiple indexes",
			args: args{
				newPredName: "name",
				schemaNode: &pb.SchemaNode{
					Type:       "string",
					Index:      true,
					Tokenizer:  []string{"exact", "term", "hash"},
					Reverse:    true,
					Count:      true,
					List:       true,
					Upsert:     true,
					Lang:       true,
					NoConflict: true,
				},
			},
			want: "name: [string] @count @index(exact, term, " +
				"hash) @lang @noconflict @reverse @upsert .\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getPredSchemaString(tt.args.newPredName, tt.args.schemaNode); got != tt.want {
				t.Errorf("getPredSchemaString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getTypeSchemaString(t *testing.T) {
	type args struct {
		newTypeName   string
		typeNode      *schemaTypeNode
		newPredNames  map[string]string
		predsToRemove map[string]struct{}
	}

	testSchemaTypeNode := &schemaTypeNode{
		Name: "dgraph.User",
		Fields: []*schemaTypeField{
			{Name: "name"},
			{Name: "dgraph.xid"},
			{Name: "age"},
			{Name: "unnecessaryEdge"},
		},
	}

	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "only change type name",
			args: args{
				newTypeName:   "User",
				typeNode:      testSchemaTypeNode,
				newPredNames:  nil,
				predsToRemove: nil,
			},
			want: "type User {\n  name\n  dgraph.xid\n  age\n  unnecessaryEdge\n}\n",
		},
		{
			name: "change type name + change name for a predicate + remove another predicate",
			args: args{
				newTypeName:   "User",
				typeNode:      testSchemaTypeNode,
				newPredNames:  map[string]string{"dgraph.xid": "xid"},
				predsToRemove: map[string]struct{}{"unnecessaryEdge": {}},
			},
			want: "type User {\n  name\n  xid\n  age\n}\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getTypeSchemaString(tt.args.newTypeName, tt.args.typeNode,
				tt.args.newPredNames, tt.args.predsToRemove); got != tt.want {
				t.Errorf("getTypeSchemaString() = %v, want %v", got, tt.want)
			}
		})
	}
}
