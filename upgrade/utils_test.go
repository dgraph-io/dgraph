/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package upgrade

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v250/protos/api"
)

func TestGetTypeSchemaString(t *testing.T) {
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

func TestGetTypeNquad(t *testing.T) {
	uid := "0x1"
	typeName := "Post"

	wantNQuad := &api.NQuad{
		Subject:   uid,
		Predicate: "dgraph.type",
		ObjectValue: &api.Value{
			Val: &api.Value_StrVal{StrVal: typeName},
		},
	}

	require.Equal(t, wantNQuad, getTypeNquad(uid, typeName))
}
