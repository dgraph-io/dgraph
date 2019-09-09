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
	"fmt"
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
	SchemaQuery          QueryType    = "schema"
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
	Operation(r *Request, parsingTimer OffsetTimer, validationTimer OffsetTimer) (Operation, error)
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
	Type() Type
	SelectionSet() []Field
	Location() *Location
}

// TODO: Location will be swapped with the the one the x soon when errors
// are swapped from gqlparser errors to x.
type Location struct {
	Line   int
	Column int
}

// A Mutation is a field (from the schema's Mutation type) from an Operation
type Mutation interface {
	Field
	MutationType() MutationType
	MutatedType() Type
	QueryField() Field
}

// A Query is a field (from the schema's Query type) from an Operation
type Query interface {
	Field
	QueryType() QueryType
}

// A Type is a GraphQL type like: Float, T, T! and [T!]!.  If it's not a list, then
// ListType is nil.  If it's an object type then Field gets field definitions by
// name from the definition of the type; IDField gets the ID field of the type.
type Type interface {
	Field(name string) FieldDefinition
	IDField() FieldDefinition
	Name() string
	Nullable() bool
	ListType() Type
	fmt.Stringer
}

// A FieldDefinition is a field as defined in some Type in the schema.  As opposed
// to a Field, which is an instance of a query or mutation asking for a field
// (which in turn must have a FieldDefinition of the right type in the schema.)
type FieldDefinition interface {
	Name() string
	Type() Type
	IsID() bool
	Inverse() (Type, FieldDefinition)
}

type astType struct {
	typ      *ast.Type
	inSchema *ast.Schema
}

type schema struct {
	schema *ast.Schema
}

type operation struct {
	op   *ast.OperationDefinition
	vars map[string]interface{}

	// The fields below are used by schema introspection queries.
	query    string
	doc      *ast.QueryDocument
	inSchema *ast.Schema
}

type field struct {
	field *ast.Field
	op    *operation
	sel   ast.Selection
}

type fieldDefinition struct {
	fieldDef *ast.FieldDefinition
	inSchema *ast.Schema
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
			qs = append(qs, &query{field: f, op: o, sel: s})
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
			"ID argument (%s) of %s was not able to be parsed", id, f.Name())
	}

	return uid, err
}

func (f *field) Type() Type {
	return &astType{
		typ:      f.field.Definition.Type,
		inSchema: f.op.inSchema,
	}
}

func (f *field) SelectionSet() (flds []Field) {
	for _, s := range f.field.SelectionSet {
		if fld, ok := s.(*ast.Field); ok {
			flds = append(flds, &field{field: fld, op: f.op})
		}
	}

	return
}

func (f *field) Location() *Location {
	return &Location{
		Line:   f.field.Position.Line,
		Column: f.field.Position.Column}
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

func (q *query) Type() Type {
	return (*field)(q).Type()
}

func (q *query) SelectionSet() []Field {
	return (*field)(q).SelectionSet()
}

func (q *query) Location() *Location {
	return (*field)(q).Location()
}

func (q *query) ResponseName() string {
	return (*field)(q).ResponseName()
}

func (q *query) QueryType() QueryType {
	switch {
	case strings.HasPrefix(q.Name(), "get"):
		return GetQuery
	case q.Name() == "__schema" || q.Name() == "__type":
		return SchemaQuery
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

func (m *mutation) Type() Type {
	return (*field)(m).Type()
}

func (m *mutation) IDArgValue() (uint64, error) {
	return (*field)(m).IDArgValue()
}

func (m *mutation) SelectionSet() []Field {
	return (*field)(m).SelectionSet()
}

func (m *mutation) QueryField() Field {
	// TODO: All our mutations currently have exactly 1 field, but that will change
	// - need a better way to get the right field by convention.
	return m.SelectionSet()[0]
}

func (m *mutation) Location() *Location {
	return (*field)(m).Location()
}

func (m *mutation) ResponseName() string {
	return (*field)(m).ResponseName()
}

// MutatedType returns the underlying type that gets mutated by m.
//
// It's not the same as the response type of m because mutations don't directly
// return what they mutate.  Mutations return a payload package that includes
// the actual node mutated as a field.
func (m *mutation) MutatedType() Type {
	// ATM there's a single field in the mutation payload.
	// TODO: Need a better way to get this by convention??
	return m.SelectionSet()[0].Type()
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

func (t *astType) Field(name string) FieldDefinition {
	return &fieldDefinition{
		// this ForName lookup is a loop in the underlying schema :-(
		fieldDef: t.inSchema.Types[t.Name()].Fields.ForName(name),
		inSchema: t.inSchema,
	}
}

func (fd *fieldDefinition) Name() string {
	return fd.fieldDef.Name
}

func (fd *fieldDefinition) IsID() bool {
	return isID(fd.fieldDef)
}

func isID(fd *ast.FieldDefinition) bool {
	// this will change once we have more types of ID fields
	return fd.Type.Name() == "ID"
}

func (fd *fieldDefinition) Type() Type {
	return &astType{
		typ:      fd.fieldDef.Type,
		inSchema: fd.inSchema,
	}
}

func (fd *fieldDefinition) Inverse() (Type, FieldDefinition) {

	invDirective := fd.fieldDef.Directives.ForName(inverseDirective)
	if invDirective == nil {
		return nil, nil
	}

	invFieldArg := invDirective.Arguments.ForName(inverseArg)
	if invFieldArg == nil {
		return nil, nil // really not possible
	}

	// typ must exist if the schema passed GQL validation
	typ := fd.inSchema.Types[fd.Type().Name()]

	// fld must exist if the schema passed our validation
	fld := typ.Fields.ForName(invFieldArg.Value.Raw)

	return fd.Type(), &fieldDefinition{fieldDef: fld, inSchema: fd.inSchema}
}

func (t *astType) Name() string {
	if t.typ.NamedType == "" {
		return t.typ.Elem.NamedType
	}
	return t.typ.NamedType
}

func (t *astType) Nullable() bool {
	return !t.typ.NonNull
}

func (t *astType) ListType() Type {
	if t.typ.Elem == nil {
		return nil
	}
	return &astType{typ: t.typ.Elem}
}

func (t *astType) String() string {
	if t == nil {
		return ""
	}

	var sb strings.Builder
	// give it enough space in case it happens to be `[t.Name()!]!`
	sb.Grow(len(t.Name()) + 4)

	if t.ListType() == nil {
		sb.WriteString(t.Name())
	} else {
		// There's no lists of lists, so this needn't be recursive
		sb.WriteRune('[')
		sb.WriteString(t.Name())
		if !t.ListType().Nullable() {
			sb.WriteRune('!')
		}
		sb.WriteRune(']')
	}

	if !t.Nullable() {
		sb.WriteRune('!')
	}

	return sb.String()
}

func (t *astType) IDField() FieldDefinition {
	def := t.inSchema.Types[t.Name()]
	if def.Kind != ast.Object {
		return nil
	}

	for _, fd := range def.Fields {
		if isID(fd) {
			return &fieldDefinition{
				fieldDef: fd,
				inSchema: t.inSchema,
			}
		}
	}

	return nil
}
