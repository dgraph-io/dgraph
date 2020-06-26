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
	"testing"
)

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
