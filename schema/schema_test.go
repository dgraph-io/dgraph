/*
 * Copyright 2016-2022 Dgraph Labs, Inc. and Contributors
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

package schema

import (
	"context"
	"reflect"
	"sync"
	"testing"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/tok"
	"golang.org/x/net/trace"
)

type fields struct {
	RWMutex   sync.RWMutex
	predicate map[string]*pb.SchemaUpdate
	types     map[string]*pb.TypeUpdate
	elog      trace.EventLog
	mutSchema map[string]*pb.SchemaUpdate
}

func TestGetWriteContext(t *testing.T) {
	type args struct {
		ctx context.Context
	}
	tests := []struct {
		name string
		args args
		want context.Context
	}{
		{
			name: "TestGetWriteContext",
			args: args{
				ctx: context.Context(context.Background()),
			},
			want: context.WithValue(context.Background(), isWrite, true),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetWriteContext(tt.args.ctx); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetWriteContext() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_state_DeleteAll(t *testing.T) {
	tests := []struct {
		name   string
		fields *state
	}{
		{
			name: "Test_state_DeleteAll",
			fields: &state{
				RWMutex: sync.RWMutex{},
				predicate: map[string]*pb.SchemaUpdate{
					"pred1": &pb.SchemaUpdate{},
				},
				types: map[string]*pb.TypeUpdate{
					"type1": &pb.TypeUpdate{},
				},
				elog: nil,
				mutSchema: map[string]*pb.SchemaUpdate{
					"mutSchema1": &pb.SchemaUpdate{},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.fields.DeleteAll()
		})
	}
}

// func Test_state_Delete(t *testing.T) {
// 	type args struct {
// 		attr string
// 		ts   uint64
// 	}
// 	tests := []struct {
// 		name    string
// 		fields  *state
// 		args    args
// 		wantErr bool
// 	}{
// 		{
// 			name: "Test_state_Delete",
// 			fields: &state{
// 				RWMutex: sync.RWMutex{},
// 				predicate: map[string]*pb.SchemaUpdate{
// 					"pred1": &pb.SchemaUpdate{},
// 					"pred2": &pb.SchemaUpdate{},
// 				},
// 				types:     map[string]*pb.TypeUpdate{},
// 				elog:      nil,
// 				mutSchema: map[string]*pb.SchemaUpdate{},
// 			},
// 			args: args{
// 				attr: "pred1",
// 				ts:   1,
// 			},
// 			wantErr: false,
// 		},
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			if err := tt.fields.Delete(tt.args.attr, tt.args.ts); (err != nil) != tt.wantErr {
// 				t.Errorf("state.Delete() error = %v, wantErr %v", err, tt.wantErr)
// 			}
// 		})
// 	}
// }

func Test_state_Set(t *testing.T) {
	type args struct {
		pred   string
		schema *pb.SchemaUpdate
	}
	tests := []struct {
		name   string
		fields *fields
		args   args
	}{
		{
			name:   "Test_state_Set",
			fields: &fields{},
			args: args{
				pred:   "",
				schema: nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &state{
				RWMutex:   tt.fields.RWMutex,
				predicate: tt.fields.predicate,
				types:     tt.fields.types,
				elog:      tt.fields.elog,
				mutSchema: tt.fields.mutSchema,
			}
			s.Set(tt.args.pred, tt.args.schema)
		})
	}
}

func Test_logUpdate(t *testing.T) {
	type args struct {
		schema *pb.SchemaUpdate
		pred   string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Test_logUpdate nil",
			args: args{
				schema: nil,
				pred:   "",
			},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := logUpdate(tt.args.schema, tt.args.pred); got != tt.want {
				t.Errorf("logUpdate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_logTypeUpdate(t *testing.T) {
	type args struct {
		typ      pb.TypeUpdate
		typeName string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Test_logTypeUpdate",
			args: args{
				typ: pb.TypeUpdate{
					TypeName: "test",
					Fields:   []*pb.SchemaUpdate{},
				},
				typeName: "test",
			},
			want: "test",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := logTypeUpdate(tt.args.typ, tt.args.typeName); got == tt.want {
				t.Errorf("logTypeUpdate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_state_SetMutSchema(t *testing.T) {
	type args struct {
		pred   string
		schema *pb.SchemaUpdate
	}
	tests := []struct {
		name   string
		fields *fields
		args   args
	}{
		{
			name: "Test_state_SetMutSchema",
			fields: &fields{
				RWMutex:   sync.RWMutex{},
				predicate: map[string]*pb.SchemaUpdate{},
				types:     map[string]*pb.TypeUpdate{},
				elog:      nil,
				mutSchema: map[string]*pb.SchemaUpdate{},
			},
			args: args{
				pred:   "",
				schema: &pb.SchemaUpdate{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &state{
				RWMutex:   tt.fields.RWMutex,
				predicate: tt.fields.predicate,
				types:     tt.fields.types,
				elog:      tt.fields.elog,
				mutSchema: tt.fields.mutSchema,
			}
			s.SetMutSchema(tt.args.pred, tt.args.schema)
		})
	}
}

func Test_state_DeleteMutSchema(t *testing.T) {
	type args struct {
		pred string
	}
	tests := []struct {
		name   string
		fields *fields
		args   args
	}{
		{
			name: "",
			fields: &fields{
				RWMutex:   sync.RWMutex{},
				predicate: map[string]*pb.SchemaUpdate{},
				types:     map[string]*pb.TypeUpdate{},
				elog:      nil,
				mutSchema: map[string]*pb.SchemaUpdate{},
			},
			args: args{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &state{
				RWMutex:   tt.fields.RWMutex,
				predicate: tt.fields.predicate,
				types:     tt.fields.types,
				elog:      tt.fields.elog,
				mutSchema: tt.fields.mutSchema,
			}
			s.DeleteMutSchema(tt.args.pred)
		})
	}
}

func TestGetIndexingPredicatesNil(t *testing.T) {
	tests := []struct {
		name string
		want []string
	}{
		{
			name: "",
			want: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetIndexingPredicates(); reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetIndexingPredicates() = %v, want %v", got, tt.want)
			}
		})
	}
}

// I haven't found a way to solve it
// func in schema.go
// ps := make([]string, 0, len(s.mutSchema))
// for p := range s.mutSchema {
// 	ps = append(ps, p)
// }

// func TestGetIndexingPredicates(t *testing.T) {
// 	tests := []struct {
// 		name   string
// 		fields *state
// 		want   []string
// 	}{
// 		{
// 			name: "test",
// 			fields: &state{
// 				RWMutex:   sync.RWMutex{},
// 				predicate: map[string]*pb.SchemaUpdate{},
// 				types:     map[string]*pb.TypeUpdate{},
// 				elog:      nil,
// 				mutSchema: map[string]*pb.SchemaUpdate{
// 					"mutSchema1": &pb.SchemaUpdate{},
// 				},
// 			},
// 			want: []string{},
// 		},
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			if got := GetIndexingPredicates(); !reflect.DeepEqual(got, tt.want) {
// 				t.Errorf("GetIndexingPredicates() = %v, want %v", got, tt.want)
// 			}
// 		})
// 	}
// }

// Begin: Predicate tests
func Test_state_Predicates(t *testing.T) {
	type fields struct {
		RWMutex   sync.RWMutex
		predicate map[string]*pb.SchemaUpdate
		types     map[string]*pb.TypeUpdate
		elog      trace.EventLog
		mutSchema map[string]*pb.SchemaUpdate
	}
	tests := []struct {
		name   string
		fields fields
		want   []string
	}{
		{
			name:   "",
			fields: fields{},
			want:   []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &state{
				RWMutex:   tt.fields.RWMutex,
				predicate: tt.fields.predicate,
				types:     tt.fields.types,
				elog:      tt.fields.elog,
				mutSchema: tt.fields.mutSchema,
			}
			if got := s.Predicates(); reflect.DeepEqual(got, tt.want) {
				t.Errorf("state.Predicates() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_state_PredicatesNil(t *testing.T) {
	type fields struct{}
	tests := []struct {
		name   string
		fields *state
		want   []string
	}{
		{
			name:   "Test_state_PredicatesNil",
			fields: nil,
			want:   nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fields.Predicates(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("state.Predicates() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_state_PredicatesForOut(t *testing.T) {
	type fields struct{}
	tests := []struct {
		name   string
		fields *state
		want   []string
	}{
		{
			name: "Test_state_PredicatesForOut",
			fields: &state{
				RWMutex: sync.RWMutex{},
				predicate: map[string]*pb.SchemaUpdate{
					"t1": &pb.SchemaUpdate{
						Predicate:       "test",
						ValueType:       0,
						Directive:       0,
						Tokenizer:       []string{},
						Count:           false,
						List:            false,
						Upsert:          false,
						Lang:            false,
						NonNullable:     false,
						NonNullableList: false,
						ObjectTypeName:  "",
						NoConflict:      false,
					},
				},
				types:     map[string]*pb.TypeUpdate{},
				elog:      nil,
				mutSchema: map[string]*pb.SchemaUpdate{},
			},
			want: []string{"t1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fields.Predicates(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("state.Predicates() = %v, want %v", got, tt.want)
			}
		})
	}
}

// End: Predicates tests

// Begin: Types tests
func Test_state_TypesNil(t *testing.T) {
	tests := []struct {
		name   string
		fields *state
		want   []string
	}{
		{
			name:   "Test_state_TypesNil",
			fields: nil,
			want:   nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fields.Types(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("state.Types() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_state_Types(t *testing.T) {
	tests := []struct {
		name   string
		fields *state
		want   []string
	}{
		{
			name: "Test_state_Types",
			fields: &state{
				RWMutex:   sync.RWMutex{},
				predicate: map[string]*pb.SchemaUpdate{},
				types: map[string]*pb.TypeUpdate{
					"t1": &pb.TypeUpdate{
						TypeName: "",
						Fields:   []*pb.SchemaUpdate{},
					},
				},
				elog:      nil,
				mutSchema: map[string]*pb.SchemaUpdate{},
			},
			want: []string{"t1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fields.Types(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("state.Types() = %v, want %v", got, tt.want)
			}
		})
	}
}

// End: Type tests

// Begin: GetType tests
func Test_state_GetType(t *testing.T) {
	type args struct {
		typeName string
	}
	tests := []struct {
		name   string
		fields *state
		args   args
		want   pb.TypeUpdate
		want1  bool
	}{
		{
			name: "",
			fields: &state{
				RWMutex:   sync.RWMutex{},
				predicate: map[string]*pb.SchemaUpdate{},
				types: map[string]*pb.TypeUpdate{
					"t1": &pb.TypeUpdate{
						TypeName: "type1",
						Fields:   []*pb.SchemaUpdate{},
					},
				},
				elog:      nil,
				mutSchema: map[string]*pb.SchemaUpdate{},
			},
			args: args{
				typeName: "type1",
			},
			want:  pb.TypeUpdate{},
			want1: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := tt.fields.GetType(tt.args.typeName)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("state.GetType() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("state.GetType() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func Test_state_GetTypeTrue(t *testing.T) {
	type args struct {
		typeName string
	}
	tests := []struct {
		name   string
		fields *state
		args   args
		want   pb.TypeUpdate
		want1  bool
	}{
		{
			name: "Test_state_GetTypeTrue",
			fields: &state{
				RWMutex:   sync.RWMutex{},
				predicate: map[string]*pb.SchemaUpdate{},
				types: map[string]*pb.TypeUpdate{
					"t1": &pb.TypeUpdate{
						TypeName: "t1",
						Fields:   []*pb.SchemaUpdate{},
					},
				},
				elog:      nil,
				mutSchema: map[string]*pb.SchemaUpdate{},
			},
			args: args{
				typeName: "t1",
			},
			want: pb.TypeUpdate{
				TypeName: "t1",
				Fields:   []*pb.SchemaUpdate{},
			},
			want1: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := tt.fields.GetType(tt.args.typeName)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("state.GetType() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("state.GetType() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

// End: GetType tests

// Begin: IsIndexed tests
func Test_state_IsIndexed(t *testing.T) {
	type args struct {
		ctx  context.Context
		pred string
	}
	tests := []struct {
		name   string
		fields *state
		args   args
		want   bool
	}{
		{
			name:   "Test_state_IsIndexed false",
			fields: &state{},
			args: args{
				ctx:  context.Background(),
				pred: "test",
			},
			want: false,
		},
		{
			name: "Test_state_IsIndexed isWrite",
			fields: &state{
				RWMutex:   sync.RWMutex{},
				predicate: map[string]*pb.SchemaUpdate{},
				types:     map[string]*pb.TypeUpdate{},
				elog:      nil,
				mutSchema: map[string]*pb.SchemaUpdate{
					"pred1": &pb.SchemaUpdate{
						Predicate:       "",
						ValueType:       0,
						Directive:       0,
						Tokenizer:       []string{"token1"},
						Count:           false,
						List:            false,
						Upsert:          false,
						Lang:            false,
						NonNullable:     false,
						NonNullableList: false,
						ObjectTypeName:  "",
						NoConflict:      false,
					},
				},
			},
			args: args{
				ctx:  context.WithValue(context.Background(), isWrite, true),
				pred: "pred1",
			},
			want: true,
		},
		{
			name: "Test_state_IsIndexed Token > 0",
			fields: &state{
				RWMutex: sync.RWMutex{},
				predicate: map[string]*pb.SchemaUpdate{
					"pred1": &pb.SchemaUpdate{},
				},
				types:     map[string]*pb.TypeUpdate{},
				elog:      nil,
				mutSchema: map[string]*pb.SchemaUpdate{},
			},
			args: args{
				ctx:  context.Background(),
				pred: "pred1",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fields.IsIndexed(tt.args.ctx, tt.args.pred); got != tt.want {
				t.Errorf("state.IsIndexed() = %v, want %v", got, tt.want)
			}
		})
	}
}

// End: IsIndexed tests

func Test_state_TokenizerNames(t *testing.T) {
	type args struct {
		ctx  context.Context
		pred string
	}
	tests := []struct {
		name   string
		fields *state
		args   args
		want   []string
	}{
		{
			name: "",
			fields: &state{
				RWMutex: sync.RWMutex{},
				predicate: map[string]*pb.SchemaUpdate{
					"t1": &pb.SchemaUpdate{
						Predicate:       "t1",
						ValueType:       0,
						Directive:       0,
						Tokenizer:       []string{"exact"},
						Count:           false,
						List:            false,
						Upsert:          false,
						Lang:            false,
						NonNullable:     false,
						NonNullableList: false,
						ObjectTypeName:  "",
						NoConflict:      false,
					},
				},
				types: map[string]*pb.TypeUpdate{},
				elog:  nil,
				mutSchema: map[string]*pb.SchemaUpdate{
					"t1": &pb.SchemaUpdate{},
				},
			},
			args: args{
				ctx:  context.Context(context.Background()),
				pred: "t1",
			},
			want: []string{"exact"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fields.TokenizerNames(tt.args.ctx, tt.args.pred); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("state.TokenizerNames() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_state_Namespaces(t *testing.T) {
	tests := []struct {
		name   string
		fields *state
		want   map[uint64]struct{}
	}{
		{
			name:   "Test_state_Namespaces 1",
			fields: nil,
			want:   nil,
		},
		{
			name: "Test_state_Namespaces 2",
			fields: &state{
				RWMutex:   sync.RWMutex{},
				predicate: map[string]*pb.SchemaUpdate{},
				types:     map[string]*pb.TypeUpdate{},
				elog:      nil,
				mutSchema: map[string]*pb.SchemaUpdate{},
			},
			want: map[uint64]struct{}{},
		},
		// {
		// 	name: "Test_state_Namespaces 3",
		// 	fields: &state{
		// 		RWMutex:   sync.RWMutex{},
		// 		predicate: map[string]*pb.SchemaUpdate{},
		// 		types: map[string]*pb.TypeUpdate{
		// 			"type1": &pb.TypeUpdate{
		// 				TypeName: "typeName1",
		// 				Fields:   []*pb.SchemaUpdate{},
		// 			},
		// 		},
		// 		elog:      nil,
		// 		mutSchema: map[string]*pb.SchemaUpdate{},
		// 	},
		// 	want: map[uint64]struct{}{},
		// },
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fields.Namespaces(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("state.Namespaces() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_state_DeletePredsForNs(t *testing.T) {
	type args struct {
		delNs uint64
	}
	tests := []struct {
		name   string
		fields *state
		args   args
	}{
		{
			name:   "Test_state_DeletePredsForNs Nil",
			fields: nil,
			args: args{
				delNs: 0,
			},
		},
		// {
		// 	name: "Test_state_DeletePredsForNs Predicate",
		// 	fields: &state{
		// 		RWMutex: sync.RWMutex{},
		// 		predicate: map[string]*pb.SchemaUpdate{
		// 			"string": &pb.SchemaUpdate{},
		// 		},
		// 		types: map[string]*pb.TypeUpdate{},
		// 		elog:  nil,
		// 		mutSchema: map[string]*pb.SchemaUpdate{
		// 			"string": &pb.SchemaUpdate{},
		// 		},
		// 	},
		// 	args: args{
		// 		delNs: 184469551615,
		// 	},
		// },
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.fields.DeletePredsForNs(tt.args.delNs)
		})
	}
}

func Test_state_HasTokenizer(t *testing.T) {
	type args struct {
		ctx  context.Context
		id   byte
		pred string
	}
	tests := []struct {
		name   string
		fields *state
		args   args
		want   bool
	}{
		{
			name:   "Test_state_HasTokenizer false",
			fields: &state{},
			args: args{
				ctx:  context.Context(context.Background()),
				id:   0,
				pred: "",
			},
			want: false,
		},
		// Todo: true case - find out how ro pass a schema
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fields.HasTokenizer(tt.args.ctx, tt.args.id, tt.args.pred); got != tt.want {
				t.Errorf("state.HasTokenizer() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_state_HasCount(t *testing.T) {
	type args struct {
		ctx  context.Context
		pred string
	}
	tests := []struct {
		name   string
		fields *state
		args   args
		want   bool
	}{
		{
			name:   "Test_state_HasCount false",
			fields: &state{},
			args: args{
				ctx:  context.Context(context.Background()),
				pred: "",
			},
			want: false,
		},
		{
			name: "Test_state_HasCount Count",
			fields: &state{
				RWMutex: sync.RWMutex{},
				predicate: map[string]*pb.SchemaUpdate{
					"pred1": &pb.SchemaUpdate{},
				},
				types: map[string]*pb.TypeUpdate{},
				elog:  nil,
				mutSchema: map[string]*pb.SchemaUpdate{
					"mut": &pb.SchemaUpdate{
						Predicate:       "pred1",
						ValueType:       0,
						Directive:       0,
						Tokenizer:       []string{},
						Count:           true,
						List:            false,
						Upsert:          false,
						Lang:            false,
						NonNullable:     false,
						NonNullableList: false,
						ObjectTypeName:  "",
						NoConflict:      false,
					},
				},
			},
			args: args{
				ctx:  context.WithValue(context.Background(), isWrite, true),
				pred: "pred1",
			},
			want: false,
		},
		{
			name: "Test_state_HasCount Count2",
			fields: &state{
				RWMutex: sync.RWMutex{},
				predicate: map[string]*pb.SchemaUpdate{
					"pred1": &pb.SchemaUpdate{},
				},
				types: map[string]*pb.TypeUpdate{},
				elog:  nil,
				mutSchema: map[string]*pb.SchemaUpdate{
					"pred1": &pb.SchemaUpdate{
						Predicate:       "pred1",
						ValueType:       0,
						Directive:       0,
						Tokenizer:       []string{},
						Count:           true,
						List:            false,
						Upsert:          false,
						Lang:            false,
						NonNullable:     false,
						NonNullableList: false,
						ObjectTypeName:  "",
						NoConflict:      false,
					},
				},
			},
			args: args{
				ctx:  context.WithValue(context.Background(), isWrite, true),
				pred: "pred1",
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fields.HasCount(tt.args.ctx, tt.args.pred); got != tt.want {
				t.Errorf("state.HasCount() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_state_Tokenizer(t *testing.T) {
	type args struct {
		ctx  context.Context
		pred string
	}
	tests := []struct {
		name   string
		fields *state
		args   args
		want   []tok.Tokenizer
	}{
		{
			name: "Test_state_Tokenizer ",
			fields: &state{
				RWMutex:   sync.RWMutex{},
				predicate: map[string]*pb.SchemaUpdate{},
				types:     map[string]*pb.TypeUpdate{},
				elog:      nil,
				mutSchema: map[string]*pb.SchemaUpdate{
					"pred1": &pb.SchemaUpdate{},
				},
			},
			args: args{
				ctx:  context.WithValue(context.Background(), isWrite, true),
				pred: "pred1",
			},
			want: []tok.Tokenizer{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fields.Tokenizer(tt.args.ctx, tt.args.pred); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("state.Tokenizer() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_state_IsReversed(t *testing.T) {
	type args struct {
		ctx  context.Context
		pred string
	}
	tests := []struct {
		name   string
		fields *state
		args   args
		want   bool
	}{
		{
			name: "Test_state_IsReversed isWrite",
			fields: &state{
				RWMutex:   sync.RWMutex{},
				predicate: map[string]*pb.SchemaUpdate{},
				types:     map[string]*pb.TypeUpdate{},
				elog:      nil,
				mutSchema: map[string]*pb.SchemaUpdate{
					"pred1": &pb.SchemaUpdate{
						Predicate:       "",
						ValueType:       0,
						Directive:       2,
						Tokenizer:       []string{},
						Count:           false,
						List:            false,
						Upsert:          false,
						Lang:            false,
						NonNullable:     false,
						NonNullableList: false,
						ObjectTypeName:  "",
						NoConflict:      false,
					},
				},
			},
			args: args{
				ctx:  context.WithValue(context.Background(), isWrite, true),
				pred: "pred1",
			},
			want: true,
		},
		{
			name:   "Test_state_IsReversed false",
			fields: &state{},
			args: args{
				ctx:  context.Background(),
				pred: "",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fields.IsReversed(tt.args.ctx, tt.args.pred); got != tt.want {
				t.Errorf("state.IsReversed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_state_IsList(t *testing.T) {
	type args struct {
		pred string
	}
	tests := []struct {
		name   string
		fields *state
		args   args
		want   bool
	}{
		{
			name: "Test_state_IsList",
			fields: &state{
				RWMutex: sync.RWMutex{},
				predicate: map[string]*pb.SchemaUpdate{
					"test": &pb.SchemaUpdate{
						Predicate:       "test",
						ValueType:       0,
						Directive:       0,
						Tokenizer:       []string{},
						Count:           false,
						List:            true,
						Upsert:          false,
						Lang:            false,
						NonNullable:     false,
						NonNullableList: false,
						ObjectTypeName:  "",
						NoConflict:      false,
					},
				},
				types:     map[string]*pb.TypeUpdate{},
				elog:      nil,
				mutSchema: map[string]*pb.SchemaUpdate{},
			},
			args: args{
				pred: "test",
			},
			want: true,
		},
		{
			name:   "Test_state_IsList false",
			fields: &state{},
			args:   args{},
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fields.IsList(tt.args.pred); got != tt.want {
				t.Errorf("state.IsList() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_state_HasUpsert(t *testing.T) {
	type args struct {
		pred string
	}
	tests := []struct {
		name   string
		fields *state
		args   args
		want   bool
	}{
		{
			name: "Test_state_HasUpsert",
			fields: &state{
				RWMutex: sync.RWMutex{},
				predicate: map[string]*pb.SchemaUpdate{
					"test": &pb.SchemaUpdate{
						Predicate:       "",
						ValueType:       0,
						Directive:       0,
						Tokenizer:       []string{},
						Count:           false,
						List:            false,
						Upsert:          true,
						Lang:            false,
						NonNullable:     false,
						NonNullableList: false,
						ObjectTypeName:  "",
						NoConflict:      false,
					},
				},
				types:     map[string]*pb.TypeUpdate{},
				elog:      nil,
				mutSchema: map[string]*pb.SchemaUpdate{},
			},
			args: args{
				pred: "test",
			},
			want: true,
		},
		{
			name: "Test_state_HasUpsert false",
			fields: &state{
				RWMutex:   sync.RWMutex{},
				predicate: map[string]*pb.SchemaUpdate{},
				types:     map[string]*pb.TypeUpdate{},
				elog:      nil,
				mutSchema: map[string]*pb.SchemaUpdate{},
			},
			args: args{
				pred: "test",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fields.HasUpsert(tt.args.pred); got != tt.want {
				t.Errorf("state.HasUpsert() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_state_HasLang(t *testing.T) {
	type args struct {
		pred string
	}
	tests := []struct {
		name   string
		fields *state
		args   args
		want   bool
	}{
		{
			name: "Test_state_HasLang",
			fields: &state{
				RWMutex: sync.RWMutex{},
				predicate: map[string]*pb.SchemaUpdate{
					"test": &pb.SchemaUpdate{
						Predicate:       "",
						ValueType:       0,
						Directive:       0,
						Tokenizer:       []string{},
						Count:           false,
						List:            false,
						Upsert:          false,
						Lang:            true,
						NonNullable:     false,
						NonNullableList: false,
						ObjectTypeName:  "",
						NoConflict:      false,
					},
				},
				types:     map[string]*pb.TypeUpdate{},
				elog:      nil,
				mutSchema: map[string]*pb.SchemaUpdate{},
			},
			args: args{
				pred: "test",
			},
			want: true,
		},
		{
			name:   "Test_state_HasLang false",
			fields: &state{},
			args:   args{},
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fields.HasLang(tt.args.pred); got != tt.want {
				t.Errorf("state.HasLang() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_state_HasNoConflict(t *testing.T) {
	type args struct {
		pred string
	}
	tests := []struct {
		name   string
		fields *state
		args   args
		want   bool
	}{
		{
			name: "Test_state_HasNoConflict",
			fields: &state{
				RWMutex: sync.RWMutex{},
				predicate: map[string]*pb.SchemaUpdate{
					"test": &pb.SchemaUpdate{},
				},
				types:     map[string]*pb.TypeUpdate{},
				elog:      nil,
				mutSchema: map[string]*pb.SchemaUpdate{},
			},
			args: args{
				pred: "test",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fields.HasNoConflict(tt.args.pred); got != tt.want {
				t.Errorf("state.HasNoConflict() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_state_IndexingInProgress(t *testing.T) {
	tests := []struct {
		name   string
		fields *state
		want   bool
	}{
		{
			name: "Test_state_IndexingInProgress > 0",
			fields: &state{
				RWMutex:   sync.RWMutex{},
				predicate: map[string]*pb.SchemaUpdate{},
				types:     map[string]*pb.TypeUpdate{},
				elog:      nil,
				mutSchema: map[string]*pb.SchemaUpdate{
					"test": &pb.SchemaUpdate{},
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fields.IndexingInProgress(); got != tt.want {
				t.Errorf("state.IndexingInProgress() = %v, want %v", got, tt.want)
			}
		})
	}
}
