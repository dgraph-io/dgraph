/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
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
	"strconv"
	"strings"

	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/gqlerror"
)

// Wrap the github.com/vektah/gqlparser/ast defintions so that the bulk of the GraphQL
// algorithm and interface is dependent on behaviours we expect from a GraphQL schema
// and validation, but not dependent the exact structure in the gqlparser.
//
// This also auto hooks up some bookkeeping that's otherwise no fun.  E.g. getting values for
// field arguments requires the variable map from the operation - so we'd need to carry vars
// through all the resolver functions.  Much nicer if they are resolved by magic here.
//
// TODO: *Note* not vendoring github.com/vektah/gqlparser at this stage.  You need to go get it.
// Will make decision on if it's exactly what we need as we move along.

// QueryType is currently supported queries
type QueryType string

// MutationType is currently supported mutations
type MutationType string

const (
	GetQuery             QueryType    = "get"
	FilterQuery          QueryType    = "query"
	NotSupportedQuery    QueryType    = "notsupported"
	AddMutation          MutationType = "add"
	UpdateMutation       MutationType = "update"
	DeleteMutation       MutationType = "delete"
	NotSupportedMutation MutationType = "notsupported"
	IDType                            = "ID"
	IDArgName                         = "id"
	InputArgName                      = "input"
)

// Schema represents a valid GraphQL schema
type Schema interface {
	Operation(r *Request) (Operation, error)
}

// An Operation is a single valid GraphQL operation.  It contains either
// Queries or Mutations, but not both.  Subscriptions are not yet supported.
type Operation interface {
	Queries() []Query
	Mutations() []Mutation
	IsQuery() bool
	IsMutation() bool
	IsSubscription() bool
}

// A Field is one field from an Operation.
type Field interface {
	Name() string
	Alias() string
	ResponseName() string
	ArgValue(name string) interface{}
	IDArgValue() (uint64, error)
	TypeName() string
	SelectionSet() []Field
}

// A Mutation is a field (from the schema's Mutation type) from an Operation
type Mutation interface {
	Field
	MutationType() MutationType
	MutatedTypeName() string
}

// A Query is a field (from the schema's Query type) from an Operation
type Query interface {
	Field
	QueryType() QueryType
}

type schema struct {
	schema *ast.Schema
}

type operation struct {
	op   *ast.OperationDefinition
	vars map[string]interface{}
}

type field struct {
	field *ast.Field
	op    *operation
}
type mutation field
type query field

func (o *operation) IsQuery() bool {
	return o.op.Operation == ast.Query
}

func (o *operation) IsMutation() bool {
	return o.op.Operation == ast.Mutation
}

func (o *operation) IsSubscription() bool {
	return o.op.Operation == ast.Subscription
}

func (o *operation) Queries() (qs []Query) {
	if !o.IsQuery() {
		return
	}

	for _, s := range o.op.SelectionSet {
		if f, ok := s.(*ast.Field); ok {
			qs = append(qs, &query{field: f, op: o})
		}
	}

	return
}

func (o *operation) Mutations() (ms []Mutation) {
	if !o.IsMutation() {
		return
	}

	for _, s := range o.op.SelectionSet {
		if f, ok := s.(*ast.Field); ok {
			ms = append(ms, &mutation{field: f, op: o})
		}
	}

	return
}

// AsSchema wraps a github.com/vektah/gqlparser/ast.Schema.
func AsSchema(s *ast.Schema) Schema {
	return &schema{schema: s}
}

func responseName(f *ast.Field) string {
	if f.Alias == "" {
		return f.Name
	}
	return f.Alias
}

func (f *field) Name() string {
	return f.field.Name
}

func (f *field) Alias() string {
	return f.field.Alias
}

func (f *field) ResponseName() string {
	return responseName(f.field)
}

func (f *field) ArgValue(name string) interface{} {
	// FIXME: cache ArgumentMap ?
	return f.field.ArgumentMap(f.op.vars)[name]
}

func (f *field) IDArgValue() (uint64, error) {
	idArg := f.ArgValue(IDArgName)
	if idArg == nil {
		return 0,
			gqlerror.ErrorPosf(f.field.GetPosition(),
				"ID argument not available on field %s", f.Name())
	}

	id, ok := idArg.(string)
	uid, err := strconv.ParseUint(id, 0, 64)

	if !ok || err != nil {
		err = gqlerror.ErrorPosf(f.field.GetPosition(),
			"ID argument of %s was not able to be parsed", f.Name())
	}

	return uid, err
}

func (f *field) TypeName() string {
	return f.field.Definition.Type.NamedType
}

func (f *field) SelectionSet() (flds []Field) {
	for _, s := range f.field.SelectionSet {
		if fld, ok := s.(*ast.Field); ok {
			flds = append(flds, &field{field: fld, op: f.op})
		}
	}

	return
}

func (q *query) Name() string {
	return (*field)(q).Name()
}

func (q *query) Alias() string {
	return (*field)(q).Alias()
}

func (q *query) ArgValue(name string) interface{} {
	return (*field)(q).ArgValue(name)
}

func (q *query) IDArgValue() (uint64, error) {
	return (*field)(q).IDArgValue()
}

func (q *query) TypeName() string {
	return (*field)(q).TypeName()
}

func (q *query) SelectionSet() []Field {
	return (*field)(q).SelectionSet()
}

func (q *query) ResponseName() string {
	return (*field)(q).ResponseName()
}

func (q *query) QueryType() QueryType {
	switch {
	case strings.HasPrefix(q.Name(), "get"):
		return GetQuery
	default:
		return NotSupportedQuery
	}
}

func (m *mutation) Name() string {
	return (*field)(m).Name()
}

func (m *mutation) Alias() string {
	return (*field)(m).Alias()
}

func (m *mutation) ArgValue(name string) interface{} {
	return (*field)(m).ArgValue(name)
}

func (m *mutation) TypeName() string {
	return (*field)(m).TypeName()
}

func (m *mutation) IDArgValue() (uint64, error) {
	return (*field)(m).IDArgValue()
}

func (m *mutation) SelectionSet() []Field {
	return (*field)(m).SelectionSet()
}

func (m *mutation) ResponseName() string {
	return (*field)(m).ResponseName()
}

func (m *mutation) MutatedTypeName() string {
	prefix := strings.TrimSuffix(m.TypeName(), "Payload")
	switch {
	case strings.HasPrefix(prefix, "Add"):
		return strings.TrimPrefix(prefix, "Add")
	case strings.HasPrefix(prefix, "Update"):
		return strings.TrimPrefix(prefix, "Update")
	case strings.HasPrefix(prefix, "Delete"):
		return strings.TrimPrefix(prefix, "Delete")
	}
	return m.TypeName()
}

func (m *mutation) MutationType() MutationType {
	switch {
	case strings.HasPrefix(m.Name(), "add"):
		return AddMutation
	case strings.HasPrefix(m.Name(), "update"):
		return UpdateMutation
	case strings.HasPrefix(m.Name(), "delete"):
		return DeleteMutation
	default:
		return NotSupportedMutation
	}
}
