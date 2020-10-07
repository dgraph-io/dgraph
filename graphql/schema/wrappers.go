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
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/vektah/gqlparser/v2/parser"

	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
	"github.com/vektah/gqlparser/v2/ast"
)

// Wrap the github.com/vektah/gqlparser/ast defintions so that the bulk of the GraphQL
// algorithm and interface is dependent on behaviours we expect from a GraphQL schema
// and validation, but not dependent the exact structure in the gqlparser.
//
// This also auto hooks up some bookkeeping that's otherwise no fun.  E.g. getting values for
// field arguments requires the variable map from the operation - so we'd need to carry vars
// through all the resolver functions.  Much nicer if they are resolved by magic here.

// QueryType is currently supported queries
type QueryType string

// MutationType is currently supported mutations
type MutationType string

// FieldHTTPConfig contains the config needed to resolve a field using a remote HTTP endpoint
// which could a GraphQL or a REST endpoint.
type FieldHTTPConfig struct {
	URL    string
	Method string
	// would be nil if there is no body
	Template       *interface{}
	Mode           string
	ForwardHeaders http.Header
	// would be empty for non-GraphQL requests
	RemoteGqlQueryName string
	RemoteGqlQuery     string

	// args required by the HTTP/GraphQL request. These should be present in the parent type
	// in the case of resolving a field or in the parent field in case of a query/mutation
	RequiredArgs map[string]bool

	// For the following request
	// graphql: "query($sinput: [SchoolInput]) { schoolNames(schools: $sinput) }"
	// the GraphqlBatchModeArgument would be sinput, we use it to know the GraphQL variable that
	// we should send the data in.
	GraphqlBatchModeArgument string
}

// Query/Mutation types and arg names
const (
	GetQuery             QueryType    = "get"
	FilterQuery          QueryType    = "query"
	SchemaQuery          QueryType    = "schema"
	PasswordQuery        QueryType    = "checkPassword"
	HTTPQuery            QueryType    = "http"
	DQLQuery             QueryType    = "dql"
	NotSupportedQuery    QueryType    = "notsupported"
	AddMutation          MutationType = "add"
	UpdateMutation       MutationType = "update"
	DeleteMutation       MutationType = "delete"
	HTTPMutation         MutationType = "http"
	NotSupportedMutation MutationType = "notsupported"
	IDType                            = "ID"
	InputArgName                      = "input"
	FilterArgName                     = "filter"
)

// Schema represents a valid GraphQL schema
type Schema interface {
	Operation(r *Request) (Operation, error)
	Queries(t QueryType) []string
	Mutations(t MutationType) []string
}

// An Operation is a single valid GraphQL operation.  It contains either
// Queries or Mutations, but not both.  Subscriptions are not yet supported.
type Operation interface {
	Queries() []Query
	Mutations() []Mutation
	Schema() Schema
	IsQuery() bool
	IsMutation() bool
	IsSubscription() bool
}

// A Field is one field from an Operation.
type Field interface {
	Name() string
	Alias() string
	// DgraphAlias is used as an alias in DQL while rewriting the GraphQL field
	DgraphAlias() string
	ResponseName() string
	Arguments() map[string]interface{}
	ArgValue(name string) interface{}
	IsArgListType(name string) bool
	IDArgValue() (*string, uint64, error)
	XIDArg() string
	SetArgTo(arg string, val interface{})
	Skip() bool
	Include() bool
	Cascade() []string
	HasCustomDirective() (bool, map[string]bool)
	HasLambdaDirective() bool
	Type() Type
	SelectionSet() []Field
	Location() x.Location
	DgraphPredicate() string
	Operation() Operation
	// InterfaceType tells us whether this field represents a GraphQL Interface.
	InterfaceType() bool
	IncludeInterfaceField(types []interface{}) bool
	TypeName(dgraphTypes []interface{}) string
	GetObjectName() string
	IsAuthQuery() bool
	CustomHTTPConfig() (FieldHTTPConfig, error)
	EnumValues() []string
}

// A Mutation is a field (from the schema's Mutation type) from an Operation
type Mutation interface {
	Field
	MutationType() MutationType
	MutatedType() Type
	QueryField() Field
	NumUidsField() Field
}

// A Query is a field (from the schema's Query type) from an Operation
type Query interface {
	Field
	QueryType() QueryType
	DQLQuery() string
	Rename(newName string)
	AuthFor(typ Type, jwtVars map[string]interface{}) Query
}

// A Type is a GraphQL type like: Float, T, T! and [T!]!.  If it's not a list, then
// ListType is nil.  If it's an object type then Field gets field definitions by
// name from the definition of the type; IDField gets the ID field of the type.
type Type interface {
	Field(name string) FieldDefinition
	Fields() []FieldDefinition
	IDField() FieldDefinition
	XIDField() FieldDefinition
	InterfaceImplHasAuthRules() bool
	PasswordField() FieldDefinition
	Name() string
	DgraphName() string
	DgraphPredicate(fld string) string
	Nullable() bool
	ListType() Type
	Interfaces() []string
	EnsureNonNulls(map[string]interface{}, string) error
	FieldOriginatedFrom(fieldName string) string
	AuthRules() *TypeAuth
	IsPoint() bool
	fmt.Stringer
}

// A FieldDefinition is a field as defined in some Type in the schema.  As opposed
// to a Field, which is an instance of a query or mutation asking for a field
// (which in turn must have a FieldDefinition of the right type in the schema.)
type FieldDefinition interface {
	Name() string
	DgraphAlias() string
	DgraphPredicate() string
	Type() Type
	ParentType() Type
	IsID() bool
	HasIDDirective() bool
	Inverse() FieldDefinition
	// TODO - It might be possible to get rid of ForwardEdge and just use Inverse() always.
	ForwardEdge() FieldDefinition
}

type astType struct {
	typ             *ast.Type
	inSchema        *schema
	dgraphPredicate map[string]map[string]string
}

type schema struct {
	schema *ast.Schema
	// dgraphPredicate gives us the dgraph predicate corresponding to a typeName + fieldName.
	// It is pre-computed so that runtime queries and mutations can look it
	// up quickly.
	// The key for the first map are the type names. The second map has a mapping of the
	// fieldName => dgraphPredicate.
	dgraphPredicate map[string]map[string]string
	// Map of mutation field name to mutated type.
	mutatedType map[string]*astType
	// Map from typename to ast.Definition
	typeNameAst map[string][]*ast.Definition
	// map from field name to bool, indicating if a field name was repeated across different types
	// implementing the same interface
	repeatedFieldNames map[string]bool
	// customDirectives stores the mapping of typeName -> fieldName -> @custom definition.
	// It is read-only.
	// The outer map will contain typeName key only if one of the fields on that type has @custom.
	// The inner map will contain fieldName key only if that field has @custom.
	// It is pre-computed so that runtime queries and mutations can look it up quickly, and not do
	// something like field.Directives.ForName("custom"), which results in iterating over all the
	// directives of the field.
	customDirectives map[string]map[string]*ast.Directive
	// lambdaDirectives stores the mapping of typeName->fieldName->true, if the field has @lambda.
	// It is read-only.
	lambdaDirectives map[string]map[string]bool
	// Map from typename to auth rules
	authRules map[string]*TypeAuth
}

type operation struct {
	op     *ast.OperationDefinition
	vars   map[string]interface{}
	header http.Header
	// interfaceImplFragFields stores a mapping from a field collected from a fragment inside an
	// interface to its typeCondition. It is used during completion to find out if a field should
	// be included in GraphQL response or not.
	interfaceImplFragFields map[*ast.Field]string

	// The fields below are used by schema introspection queries.
	query    string
	doc      *ast.QueryDocument
	inSchema *schema
}

type field struct {
	field *ast.Field
	op    *operation
	sel   ast.Selection
	// arguments contains the computed values for arguments taking into account the values
	// for the GraphQL variables supplied in the query.
	arguments map[string]interface{}
}

type fieldDefinition struct {
	fieldDef        *ast.FieldDefinition
	parentType      Type
	inSchema        *schema
	dgraphPredicate map[string]map[string]string
}

type mutation field
type query field

func (s *schema) Queries(t QueryType) []string {
	if s.schema.Query == nil {
		return nil
	}
	var result []string
	for _, q := range s.schema.Query.Fields {
		if queryType(q.Name, s.customDirectives["Query"][q.Name]) == t {
			result = append(result, q.Name)
		}
	}
	return result
}

func (s *schema) Mutations(t MutationType) []string {
	if s.schema.Mutation == nil {
		return nil
	}
	var result []string
	for _, m := range s.schema.Mutation.Fields {
		if mutationType(m.Name, s.customDirectives["Mutation"][m.Name]) == t {
			result = append(result, m.Name)
		}
	}
	return result
}

func (o *operation) IsQuery() bool {
	return o.op.Operation == ast.Query
}

func (o *operation) IsMutation() bool {
	return o.op.Operation == ast.Mutation
}

func (o *operation) IsSubscription() bool {
	return o.op.Operation == ast.Subscription
}

func (o *operation) Schema() Schema {
	return o.inSchema
}

func (o *operation) Queries() (qs []Query) {
	if o.IsMutation() {
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

// parentInterface returns the name of an interface that a field belonging to a type definition
// typDef inherited from. If there is no such interface, then it returns an empty string.
//
// Given the following schema
// interface A {
//   name: String
// }
//
// type B implements A {
//	 name: String
//   age: Int
// }
//
// calling parentInterface on the fieldName name with type definition for B, would return A.
func parentInterface(sch *ast.Schema, typDef *ast.Definition, fieldName string) *ast.Definition {
	if len(typDef.Interfaces) == 0 {
		return nil
	}

	for _, iface := range typDef.Interfaces {
		interfaceDef := sch.Types[iface]
		for _, interfaceField := range interfaceDef.Fields {
			if fieldName == interfaceField.Name {
				return interfaceDef
			}
		}
	}
	return nil
}

func parentInterfaceForPwdField(sch *ast.Schema, typDef *ast.Definition,
	fieldName string) *ast.Definition {
	if len(typDef.Interfaces) == 0 {
		return nil
	}

	for _, iface := range typDef.Interfaces {
		interfaceDef := sch.Types[iface]
		pwdField := getPasswordField(interfaceDef)
		if pwdField != nil && fieldName == pwdField.Name {
			return interfaceDef
		}
	}
	return nil
}

func convertPasswordDirective(dir *ast.Directive) *ast.FieldDefinition {
	if dir.Name != "secret" {
		return nil
	}

	name := dir.Arguments.ForName("field").Value.Raw
	pred := dir.Arguments.ForName("pred")
	dirs := ast.DirectiveList{}

	if pred != nil {
		dirs = ast.DirectiveList{{
			Name: "dgraph",
			Arguments: ast.ArgumentList{{
				Name: "pred",
				Value: &ast.Value{
					Raw:  pred.Value.Raw,
					Kind: ast.StringValue,
				},
			}},
			Position: dir.Position,
		}}
	}

	fd := &ast.FieldDefinition{
		Name: name,
		Type: &ast.Type{
			NamedType: "String",
			NonNull:   true,
			Position:  dir.Position,
		},
		Directives: dirs,
		Position:   dir.Position,
	}

	return fd
}

func dgraphMapping(sch *ast.Schema) map[string]map[string]string {
	const (
		add     = "Add"
		update  = "Update"
		del     = "Delete"
		payload = "Payload"
	)

	dgraphPredicate := make(map[string]map[string]string)
	for _, inputTyp := range sch.Types {
		// We only want to consider input types (object and interface) defined by the user as part
		// of the schema hence we ignore BuiltIn, query and mutation types and Geo types.
		if inputTyp.BuiltIn || isQueryOrMutationType(inputTyp) || inputTyp.Name == "Subscription" ||
			(inputTyp.Kind != ast.Object && inputTyp.Kind != ast.Interface) || inputTyp.Name == "Point" {
			continue
		}

		originalTyp := inputTyp
		inputTypeName := inputTyp.Name
		if strings.HasPrefix(inputTypeName, add) && strings.HasSuffix(inputTypeName, payload) {
			continue
		}

		dgraphPredicate[originalTyp.Name] = make(map[string]string)

		if (strings.HasPrefix(inputTypeName, update) || strings.HasPrefix(inputTypeName, del)) &&
			strings.HasSuffix(inputTypeName, payload) {
			// For UpdateTypePayload and DeleteTypePayload, inputTyp should be Type.
			if strings.HasPrefix(inputTypeName, update) {
				inputTypeName = strings.TrimSuffix(strings.TrimPrefix(inputTypeName, update),
					payload)
			} else if strings.HasPrefix(inputTypeName, del) {
				inputTypeName = strings.TrimSuffix(strings.TrimPrefix(inputTypeName, del), payload)
			}
			inputTyp = sch.Types[inputTypeName]
		}

		// We add password field to the cached type information to be used while opening
		// resolving and rewriting queries to be sent to dgraph. Otherwise, rewriter won't
		// know what the password field in AddInputType/ TypePatch/ TypeRef is.
		var fields ast.FieldList
		fields = append(fields, inputTyp.Fields...)
		for _, directive := range inputTyp.Directives {
			fd := convertPasswordDirective(directive)
			if fd == nil {
				continue
			}
			fields = append(fields, fd)
		}

		for _, fld := range fields {
			if isID(fld) {
				// We don't need a mapping for the field, as we the dgraph predicate for them is
				// fixed i.e. uid.
				continue
			}
			typName := typeName(inputTyp)
			parentInt := parentInterface(sch, inputTyp, fld.Name)
			if parentInt != nil {
				typName = typeName(parentInt)
			}
			// 1. For fields that have @dgraph(pred: xxxName) directive, field name would be
			//    xxxName.
			// 2. For fields where the type (or underlying interface) has a @dgraph(type: xxxName)
			//    directive, field name would be xxxName.fldName.
			//
			// The cases below cover the cases where neither the type or field have @dgraph
			// directive.
			// 3. For types which don't inherit from an interface the keys, value would be.
			//    typName,fldName => typName.fldName
			// 4. For types which inherit fields from an interface
			//    typName,fldName => interfaceName.fldName
			// 5. For DeleteTypePayload type
			//    DeleteTypePayload,fldName => typName.fldName

			fname := fieldName(fld, typName)
			dgraphPredicate[originalTyp.Name][fld.Name] = fname
		}
	}
	return dgraphPredicate
}

func mutatedTypeMapping(s *schema,
	dgraphPredicate map[string]map[string]string) map[string]*astType {
	if s.schema.Mutation == nil {
		return nil
	}

	m := make(map[string]*astType, len(s.schema.Mutation.Fields))
	for _, field := range s.schema.Mutation.Fields {
		mutatedTypeName := ""
		switch {
		case strings.HasPrefix(field.Name, "add"):
			mutatedTypeName = strings.TrimPrefix(field.Name, "add")
		case strings.HasPrefix(field.Name, "update"):
			mutatedTypeName = strings.TrimPrefix(field.Name, "update")
		case strings.HasPrefix(field.Name, "delete"):
			mutatedTypeName = strings.TrimPrefix(field.Name, "delete")
		default:
		}
		// This is a convoluted way of getting the type for mutatedTypeName. We get the definition
		// for AddTPayload and get the type from the first field. There is no direct way to get
		// the type from the definition of an object. Interfaces can't have Add and if there is no non Id
		// field then Update also will not be there, so we use Delete if there is no AddTPayload.
		var def *ast.Definition
		if def = s.schema.Types["Add"+mutatedTypeName+"Payload"]; def == nil {
			def = s.schema.Types["Delete"+mutatedTypeName+"Payload"]
		}

		if def == nil {
			continue
		}

		// Accessing 0th element should be safe to do as according to the spec an object must define
		// one or more fields.
		typ := def.Fields[0].Type
		// This would contain mapping of mutation field name to the Type()
		// for e.g. addPost => astType for Post
		m[field.Name] = &astType{typ, s, dgraphPredicate}
	}
	return m
}

func typeMappings(s *ast.Schema) map[string][]*ast.Definition {
	typeNameAst := make(map[string][]*ast.Definition)

	for _, typ := range s.Types {
		name := typeName(typ)
		typeNameAst[name] = append(typeNameAst[name], typ)
	}

	return typeNameAst
}

func repeatedFieldMappings(s *ast.Schema, dgPreds map[string]map[string]string) map[string]bool {
	repeatedFieldNames := make(map[string]bool)

	for _, typ := range s.Types {
		if typ.Kind != ast.Interface {
			continue
		}

		interfaceFields := make(map[string]bool)
		for _, field := range typ.Fields {
			interfaceFields[field.Name] = true
		}

		type fieldInfo struct {
			dgPred   string
			repeated bool
		}

		repeatedFieldsInTypesWithCommonAncestor := make(map[string]*fieldInfo)
		for _, typ := range s.PossibleTypes[typ.Name] {
			typPreds := dgPreds[typ.Name]
			for _, field := range typ.Fields {
				// ignore this field if it was inherited from the common interface or is of ID type.
				// We ignore ID type fields too, because they map only to uid in dgraph and can't
				// map to two different predicates.
				if interfaceFields[field.Name] || field.Type.Name() == IDType {
					continue
				}
				// if we find a field with same name from types implementing a common interface
				// and its DgraphPredicate is different than what was previously encountered, then
				// we mark it as repeated field, so that queries will rewrite it with correct alias
				dgPred := typPreds[field.Name]
				if fInfo, ok := repeatedFieldsInTypesWithCommonAncestor[field.Name]; ok && fInfo.
					dgPred != dgPred {
					repeatedFieldsInTypesWithCommonAncestor[field.Name].repeated = true
				} else {
					repeatedFieldsInTypesWithCommonAncestor[field.Name] = &fieldInfo{dgPred: dgPred}
				}
			}
		}

		for fName, info := range repeatedFieldsInTypesWithCommonAncestor {
			if info.repeated {
				repeatedFieldNames[fName] = true
			}
		}
	}

	return repeatedFieldNames
}

// customAndLambdaMappings does following things:
// * If there is @custom on any field, it removes the directive from the list of directives on
//	 that field. Instead, it puts it in a map of typeName->fieldName->custom directive definition.
//	 This mapping is returned as the first return value, which is later used to determine if some
//	 field has custom directive or not, and accordingly construct the HTTP request for the field.
// * If there is @lambda on any field, it removes the directive from the list of directives on
//	 that field. Instead, it puts it in a map of typeName->fieldName->bool. This mapping is returned
//	 as the second return value, which is later used to determine if some field has lambda directive
//	 or not. An appropriate @custom directive is also constructed for the field with @lambda and
//	 put into the first mapping. Both of these mappings together are used to construct the HTTP
//	 request for @lambda field. Internally, @lambda is just @custom(http: {
//	   url: "<graphql_lambda_url: a-fixed-pre-defined-url>",
//	   method: POST,
//	   body: "<all-the-args-for-a-query-or-mutation>/<all-the-scalar-fields-from-parent-type-for-a
//	   -@lambda-on-field>"
//	   mode: BATCH (set only if @lambda was on a non query/mutation field)
//	 })
//	 So, by constructing an appropriate custom directive for @lambda fields,
//	 we just reuse logic from @custom.
func customAndLambdaMappings(s *ast.Schema) (map[string]map[string]*ast.Directive,
	map[string]map[string]bool) {
	customDirectives := make(map[string]map[string]*ast.Directive)
	lambdaDirectives := make(map[string]map[string]bool)

	for _, typ := range s.Types {
		for _, field := range typ.Fields {
			for i, dir := range field.Directives {
				if dir.Name == customDirective || dir.Name == lambdaDirective {
					// remove @custom/@lambda directive from s
					lastIndex := len(field.Directives) - 1
					field.Directives[i] = field.Directives[lastIndex]
					field.Directives = field.Directives[:lastIndex]
					// get the @custom mapping for this type
					var customFieldMap map[string]*ast.Directive
					if existingCustomFieldMap, ok := customDirectives[typ.Name]; ok {
						customFieldMap = existingCustomFieldMap
					} else {
						customFieldMap = make(map[string]*ast.Directive)
					}

					if dir.Name == customDirective {
						// if it was @custom, put the directive at the @custom mapping for the field
						customFieldMap[field.Name] = dir
					} else {
						// for lambda, first update the lambda directives map
						var lambdaFieldMap map[string]bool
						if existingLambdaFieldMap, ok := lambdaDirectives[typ.Name]; ok {
							lambdaFieldMap = existingLambdaFieldMap
						} else {
							lambdaFieldMap = make(map[string]bool)
						}
						lambdaFieldMap[field.Name] = true
						lambdaDirectives[typ.Name] = lambdaFieldMap
						// then, build a custom directive with correct semantics to be put
						// into custom directives map at this field
						customFieldMap[field.Name] = buildCustomDirectiveForLambda(typ, field,
							dir, func(f *ast.FieldDefinition) bool {
								// Need to skip the fields which have a @custom/@lambda from
								// going in body template. The field itself may not have the
								// directive anymore because the directive may have been removed by
								// this function already. So, using these maps to find the same.
								return lambdaFieldMap[f.Name] || customFieldMap[f.Name] != nil
							})
					}
					// finally, update the custom directives map for this type
					customDirectives[typ.Name] = customFieldMap
					// break, as there can only be one @custom/@lambda
					break
				}
			}
		}
	}

	return customDirectives, lambdaDirectives
}

func hasCustomOrLambda(f *ast.FieldDefinition) bool {
	for _, dir := range f.Directives {
		if dir.Name == customDirective || dir.Name == lambdaDirective {
			return true
		}
	}
	return false
}

// buildCustomDirectiveForLambda returns custom directive for the given field to be used for @lambda
// The constructed @custom looks like this:
//	@custom(http: {
//	   url: "<graphql_lambda_url: a-fixed-pre-defined-url>",
//	   method: POST,
//	   body: "<all-the-args-for-a-query-or-mutation>/<all-the-scalar-fields-from-parent-type-for-a
//	   -@lambda-on-field>"
//	   mode: BATCH (set only if @lambda was on a non query/mutation field)
//	})
func buildCustomDirectiveForLambda(defn *ast.Definition, field *ast.FieldDefinition,
	lambdaDir *ast.Directive, skipInBodyTemplate func(f *ast.FieldDefinition) bool) *ast.Directive {
	comma := ""
	var bodyTemplate strings.Builder

	// this function appends a variable to the body template for @custom
	appendToBodyTemplate := func(varName string) {
		bodyTemplate.WriteString(comma)
		bodyTemplate.WriteString(varName)
		bodyTemplate.WriteString(": $")
		bodyTemplate.WriteString(varName)
		comma = ", "
	}

	// first let's construct the body template for the custom directive
	bodyTemplate.WriteString("{")
	if isQueryOrMutationType(defn) {
		// for queries and mutations we need to put their arguments in the body template
		for _, arg := range field.Arguments {
			appendToBodyTemplate(arg.Name)
		}
	} else {
		// For fields in other types, skip the ones in body template which have a @lambda or @custom
		// or are not scalar. The skipInBodyTemplate function is also used to check these
		// conditions, in case the field can't tell by itself.
		for _, f := range defn.Fields {
			if hasCustomOrLambda(f) || !isScalar(f.Type.Name()) || skipInBodyTemplate(f) {
				continue
			}
			appendToBodyTemplate(f.Name)
		}
	}
	bodyTemplate.WriteString("}")

	// build the children for http argument
	httpArgChildrens := []*ast.ChildValue{
		getChildValue(httpUrl, x.Config.GraphqlLambdaUrl, ast.StringValue, lambdaDir.Position),
		getChildValue(httpMethod, http.MethodPost, ast.EnumValue, lambdaDir.Position),
		getChildValue(httpBody, bodyTemplate.String(), ast.StringValue, lambdaDir.Position),
	}
	if !isQueryOrMutationType(defn) {
		httpArgChildrens = append(httpArgChildrens,
			getChildValue(mode, BATCH, ast.EnumValue, lambdaDir.Position))
	}

	// build the custom directive
	return &ast.Directive{
		Name: customDirective,
		Arguments: []*ast.Argument{{
			Name: httpArg,
			Value: &ast.Value{
				Kind:     ast.ObjectValue,
				Children: httpArgChildrens,
				Position: lambdaDir.Position,
			},
			Position: lambdaDir.Position,
		}},
		Position: lambdaDir.Position,
	}
}

func getChildValue(name, raw string, kind ast.ValueKind, position *ast.Position) *ast.ChildValue {
	return &ast.ChildValue{
		Name:     name,
		Value:    &ast.Value{Raw: raw, Kind: kind, Position: position},
		Position: position,
	}
}

// AsSchema wraps a github.com/vektah/gqlparser/ast.Schema.
func AsSchema(s *ast.Schema) (Schema, error) {
	// Auth rules can't be effectively validated as part of the normal rules -
	// because they need the fully generated schema to be checked against.
	authRules, err := authRules(s)
	if err != nil {
		return nil, err
	}

	customDirs, lambdaDirs := customAndLambdaMappings(s)
	dgraphPredicate := dgraphMapping(s)
	sch := &schema{
		schema:             s,
		dgraphPredicate:    dgraphPredicate,
		typeNameAst:        typeMappings(s),
		repeatedFieldNames: repeatedFieldMappings(s, dgraphPredicate),
		customDirectives:   customDirs,
		lambdaDirectives:   lambdaDirs,
		authRules:          authRules,
	}
	sch.mutatedType = mutatedTypeMapping(sch, dgraphPredicate)

	return sch, nil
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

func (f *field) DgraphAlias() string {
	// if this field is repeated, then it should be aliased using its dgraph predicate which will be
	// unique across repeated fields
	if f.op.inSchema.repeatedFieldNames[f.Name()] {
		dgraphPredicate := f.DgraphPredicate()
		// There won't be any dgraph predicate for fields in introspection queries, as they are not
		// stored in dgraph. So we identify those fields using this condition, and just let the
		// field name get returned for introspection query fields, because the raw data response is
		// prepared for them using only the field name, so that is what should be used to pick them
		// back up from that raw data response before completion is performed.
		// Now, the reason to not combine this if check with the outer one is because this
		// function is performance critical. If there are a million fields in the output,
		// it would be called a million times. So, better to perform this check and allocate memory
		// for the variable only when necessary to do so.
		if dgraphPredicate != "" {
			return dgraphPredicate
		}
	}
	// if not repeated, alias it using its name
	return f.Name()
}

func (f *field) ResponseName() string {
	return responseName(f.field)
}

func (f *field) SetArgTo(arg string, val interface{}) {
	if f.arguments == nil {
		f.arguments = make(map[string]interface{})
	}
	f.arguments[arg] = val

	// If the argument doesn't exist, add it to the list. It is used later on to get
	// parameters. Value isn't required because it's fetched using the arguments map.
	argument := f.field.Arguments.ForName(arg)
	if argument == nil {
		f.field.Arguments = append(f.field.Arguments, &ast.Argument{Name: arg})
	}
}

func (f *field) IsAuthQuery() bool {
	return f.field.Arguments.ForName("dgraph.uid") != nil
}

func (f *field) Arguments() map[string]interface{} {
	if f.arguments == nil {
		// Compute and cache the map first time this function is called for a field.
		f.arguments = f.field.ArgumentMap(f.op.vars)
		// use a deep-copy only if the request uses variables, as a variable could be shared by
		// multiple queries in a single request and internally in our code we may overwrite field
		// arguments which may result in the shared value being overwritten for all queries in a
		// request.
		if f.op.vars != nil {
			f.arguments = x.DeepCopyJsonMap(f.arguments)
		}
	}
	return f.arguments
}

func (f *field) ArgValue(name string) interface{} {
	return f.Arguments()[name]
}

func (f *field) IsArgListType(name string) bool {
	arg := f.field.Arguments.ForName(name)
	if arg == nil {
		return false
	}

	return arg.Value.ExpectedType.Elem != nil
}

func (f *field) Skip() bool {
	dir := f.field.Directives.ForName("skip")
	if dir == nil {
		return false
	}
	return dir.ArgumentMap(f.op.vars)["if"].(bool)
}

func (f *field) Include() bool {
	dir := f.field.Directives.ForName("include")
	if dir == nil {
		return true
	}
	return dir.ArgumentMap(f.op.vars)["if"].(bool)
}

func (f *field) Cascade() []string {
	dir := f.field.Directives.ForName(cascadeDirective)
	if dir == nil {
		return nil
	}
	arg := dir.Arguments.ForName(cascadeArg)
	if arg == nil || arg.Value == nil || len(arg.Value.Children) == 0 {
		return []string{"__all__"}
	}
	fields := make([]string, 0, len(arg.Value.Children))
	typ := f.Type()
	idField := typ.IDField()

	for _, child := range arg.Value.Children {
		if idField != nil && idField.Name() == child.Value.Raw {
			fields = append(fields, "uid")
		} else {
			fields = append(fields, typ.DgraphPredicate(child.Value.Raw))
		}

	}
	return fields
}

func (f *field) HasCustomDirective() (bool, map[string]bool) {
	custom := f.op.inSchema.customDirectives[f.GetObjectName()][f.Name()]
	if custom == nil {
		return false, nil
	}

	var rf map[string]bool
	httpArg := custom.Arguments.ForName("http")

	bodyArg := httpArg.Value.Children.ForName("body")
	graphqlArg := httpArg.Value.Children.ForName("graphql")
	if bodyArg != nil {
		bodyTemplate := bodyArg.Raw
		_, rf, _ = parseBodyTemplate(bodyTemplate, graphqlArg == nil)
	}

	if rf == nil {
		rf = make(map[string]bool)
	}
	rawURL := httpArg.Value.Children.ForName("url").Raw
	// Error here should be nil as we should have parsed and validated the URL
	// already.
	u, _ := url.Parse(rawURL)
	// Parse variables from the path and query params.
	elems := strings.Split(u.Path, "/")
	for _, elem := range elems {
		if strings.HasPrefix(elem, "$") {
			rf[elem[1:]] = true
		}
	}
	for k := range u.Query() {
		val := u.Query().Get(k)
		if strings.HasPrefix(val, "$") {
			rf[val[1:]] = true
		}
	}

	if graphqlArg == nil {
		return true, rf
	}
	modeVal := ""
	modeArg := httpArg.Value.Children.ForName(mode)
	if modeArg != nil {
		modeVal = modeArg.Raw
	}

	if modeVal == SINGLE {
		// For BATCH mode, required args would have been parsed from the body above.
		var err error
		rf, err = parseRequiredArgsFromGQLRequest(graphqlArg.Raw)
		// This should not be returning an error since we should have validated this during schema
		// update.
		if err != nil {
			return true, nil
		}
	}
	return true, rf
}

func (f *field) HasLambdaDirective() bool {
	return f.op.inSchema.lambdaDirectives[f.GetObjectName()][f.Name()]
}

func (f *field) XIDArg() string {
	xidArgName := ""
	passwordField := f.Type().PasswordField()

	args := f.field.Definition.Arguments
	if len(f.field.Definition.Arguments) == 0 {
		// For acl endpoints like getCurrentUser which redirects to getUser resolver, the field
		// definition doesn't change and hence we can't find the arguments for getUser. As a
		// fallback, we get the args from the query field arguments in that case.
		args = f.op.inSchema.schema.Query.Fields.ForName(f.Name()).Arguments
	}

	for _, arg := range args {
		if arg.Type.Name() != IDType && (passwordField == nil ||
			arg.Name != passwordField.Name()) {
			xidArgName = arg.Name
		}
	}
	return f.Type().DgraphPredicate(xidArgName)
}

func (f *field) IDArgValue() (xid *string, uid uint64, err error) {
	idField := f.Type().IDField()
	passwordField := f.Type().PasswordField()
	xidArgName := ""
	// This method is only called for Get queries and check. These queries can accept ID, XID
	// or Password. Therefore the non ID and Password field is an XID.
	// TODO maybe there is a better way to do this.
	for _, arg := range f.field.Arguments {
		if (idField == nil || arg.Name != idField.Name()) &&
			(passwordField == nil || arg.Name != passwordField.Name()) {
			xidArgName = arg.Name
		}
	}
	if xidArgName != "" {
		xidArgVal, ok := f.ArgValue(xidArgName).(string)
		pos := f.field.GetPosition()
		if !ok {
			err = x.GqlErrorf("Argument (%s) of %s was not able to be parsed as a string",
				xidArgName, f.Name()).WithLocations(x.Location{Line: pos.Line, Column: pos.Column})
			return
		}
		xid = &xidArgVal
	}

	if idField == nil {
		return
	}

	idArg := f.ArgValue(idField.Name())
	if idArg != nil {
		id, ok := idArg.(string)
		var ierr error
		uid, ierr = strconv.ParseUint(id, 0, 64)

		if !ok || ierr != nil {
			pos := f.field.GetPosition()
			err = x.GqlErrorf("ID argument (%s) of %s was not able to be parsed", id, f.Name()).
				WithLocations(x.Location{Line: pos.Line, Column: pos.Column})
			return
		}
	}

	return
}

func (f *field) Type() Type {
	var t *ast.Type
	if f.field != nil && f.field.Definition != nil {
		// This is strange.  There was a case with a parsed schema and query where the field
		// had a nil Definition ... how ???
		t = f.field.Definition.Type
	}

	return &astType{
		typ:             t,
		inSchema:        f.op.inSchema,
		dgraphPredicate: f.op.inSchema.dgraphPredicate,
	}
}

func (f *field) InterfaceType() bool {
	return f.op.inSchema.schema.Types[f.field.Definition.Type.Name()].Kind == ast.Interface
}

func (f *field) GetObjectName() string {
	return f.field.ObjectDefinition.Name
}

func getCustomHTTPConfig(f *field, isQueryOrMutation bool) (FieldHTTPConfig, error) {
	custom := f.op.inSchema.customDirectives[f.GetObjectName()][f.Name()]
	httpArg := custom.Arguments.ForName("http")
	fconf := FieldHTTPConfig{
		URL:    httpArg.Value.Children.ForName("url").Raw,
		Method: httpArg.Value.Children.ForName("method").Raw,
	}

	fconf.Mode = SINGLE
	op := httpArg.Value.Children.ForName(mode)
	if op != nil {
		fconf.Mode = op.Raw
	}

	// both body and graphql can't be present together
	bodyArg := httpArg.Value.Children.ForName("body")
	graphqlArg := httpArg.Value.Children.ForName("graphql")
	var bodyTemplate string
	if bodyArg != nil {
		bodyTemplate = bodyArg.Raw
	} else if graphqlArg != nil {
		bodyTemplate = `{ query: $query, variables: $variables }`
	}
	// bodyTemplate will be empty if there was no body or graphql, like the case of a simple GET req
	if bodyTemplate != "" {
		bt, rf, err := parseBodyTemplate(bodyTemplate, true)
		if err != nil {
			return fconf, err
		}
		fconf.Template = bt
		fconf.RequiredArgs = rf
	}

	if !isQueryOrMutation && graphqlArg != nil && fconf.Mode == SINGLE {
		// For BATCH mode, required args would have been parsed from the body above.
		// Safe to ignore the error here since we should already have validated that we can parse
		// the required args from the GraphQL request during schema update.
		fconf.RequiredArgs, _ = parseRequiredArgsFromGQLRequest(graphqlArg.Raw)
	}

	fconf.ForwardHeaders = http.Header{}
	// set application/json as the default Content-Type
	fconf.ForwardHeaders.Set("Content-Type", "application/json")
	secretHeaders := httpArg.Value.Children.ForName("secretHeaders")
	if secretHeaders != nil {
		hc.RLock()
		for _, h := range secretHeaders.Children {
			key := strings.Split(h.Value.Raw, ":")
			if len(key) == 1 {
				key = []string{h.Value.Raw, h.Value.Raw}
			}
			val := string(hc.secrets[key[1]])
			fconf.ForwardHeaders.Set(key[0], val)
		}
		hc.RUnlock()
	}

	forwardHeaders := httpArg.Value.Children.ForName("forwardHeaders")
	if forwardHeaders != nil {
		for _, h := range forwardHeaders.Children {
			key := strings.Split(h.Value.Raw, ":")
			if len(key) == 1 {
				key = []string{h.Value.Raw, h.Value.Raw}
			}
			reqHeaderVal := f.op.header.Get(key[1])
			fconf.ForwardHeaders.Set(key[0], reqHeaderVal)
		}
	}

	if graphqlArg != nil {
		queryDoc, gqlErr := parser.ParseQuery(&ast.Source{Input: graphqlArg.Raw})
		if gqlErr != nil {
			return fconf, gqlErr
		}
		// queryDoc will always have only one operation with only one field
		qfield := queryDoc.Operations[0].SelectionSet[0].(*ast.Field)
		if fconf.Mode == BATCH {
			fconf.GraphqlBatchModeArgument = queryDoc.Operations[0].VariableDefinitions[0].Variable
		}
		fconf.RemoteGqlQueryName = qfield.Name
		buf := &bytes.Buffer{}
		buildGraphqlRequestFields(buf, f.field)
		remoteQuery := graphqlArg.Raw
		remoteQuery = remoteQuery[:strings.LastIndex(remoteQuery, "}")]
		remoteQuery = fmt.Sprintf("%s%s}", remoteQuery, buf.String())
		fconf.RemoteGqlQuery = remoteQuery
	}

	// if it is a query or mutation, substitute the vars in URL and Body here itself
	if isQueryOrMutation {
		var err error
		argMap := f.field.ArgumentMap(f.op.vars)
		var bodyVars map[string]interface{}
		// url params can exist only with body, and not with graphql
		if graphqlArg == nil {
			fconf.URL, err = SubstituteVarsInURL(fconf.URL, argMap)
			if err != nil {
				return fconf, errors.Wrapf(err, "while substituting vars in URL")
			}
			bodyVars = argMap
		} else {
			bodyVars = make(map[string]interface{})
			bodyVars["query"] = fconf.RemoteGqlQuery
			bodyVars["variables"] = argMap
		}
		SubstituteVarsInBody(fconf.Template, bodyVars)
	}
	return fconf, nil
}

func (f *field) CustomHTTPConfig() (FieldHTTPConfig, error) {
	return getCustomHTTPConfig(f, false)
}

func (f *field) EnumValues() []string {
	typ := f.Type()
	def := f.op.inSchema.schema.Types[typ.Name()]
	res := make([]string, 0, len(def.EnumValues))
	for _, e := range def.EnumValues {
		res = append(res, e.Name)
	}
	return res
}

func (f *field) SelectionSet() (flds []Field) {
	for _, s := range f.field.SelectionSet {
		if fld, ok := s.(*ast.Field); ok {
			flds = append(flds, &field{
				field: fld,
				op:    f.op,
			})
		}
	}

	return
}

func (f *field) Location() x.Location {
	return x.Location{
		Line:   f.field.Position.Line,
		Column: f.field.Position.Column}
}

func (f *field) Operation() Operation {
	return f.op
}

func (f *field) DgraphPredicate() string {
	return f.op.inSchema.dgraphPredicate[f.field.ObjectDefinition.Name][f.Name()]
}

func (f *field) TypeName(dgraphTypes []interface{}) string {
	for _, typ := range dgraphTypes {
		styp, ok := typ.(string)
		if !ok {
			continue
		}

		for _, origTyp := range f.op.inSchema.typeNameAst[styp] {
			if origTyp.Kind != ast.Object {
				continue
			}
			return origTyp.Name
		}

	}
	return ""
}

func (f *field) IncludeInterfaceField(dgraphTypes []interface{}) bool {
	// Given a list of dgraph types, we query the schema and find the one which is an ast.Object
	// and not an Interface object.
	for _, typ := range dgraphTypes {
		styp, ok := typ.(string)
		if !ok {
			continue
		}
		for _, origTyp := range f.op.inSchema.typeNameAst[styp] {
			if origTyp.Kind == ast.Object {
				// If the field is from an interface implemented by this object,
				// and was fetched not because of a fragment on this object,
				// but because of a fragment on some other object, then we don't need to include it.
				fragType, ok := f.op.interfaceImplFragFields[f.field]
				if ok && fragType != origTyp.Name {
					return false
				}

				// We include the field in response only if any of the following conditions hold:
				// * Field is __typename
				// * The field is of ID type: As ID maps to uid in dgraph, so it is not stored as an
				//	 edge, hence does not appear in f.op.inSchema.dgraphPredicate map. So, always
				//	 include the queried field if it is of ID type.
				// * If the field exists in the map corresponding to the object type
				_, ok = f.op.inSchema.dgraphPredicate[origTyp.Name][f.Name()]
				return ok || f.Type().Name() == IDType || f.Name() == Typename
			}
		}

	}
	return false
}

func (q *query) IsAuthQuery() bool {
	return (*field)(q).field.Arguments.ForName("dgraph.uid") != nil
}

func (q *query) AuthFor(typ Type, jwtVars map[string]interface{}) Query {
	// copy the template, so that multiple queries can run rewriting for the rule.
	return &query{
		field: (*field)(q).field,
		op: &operation{op: q.op.op,
			query:    q.op.query,
			doc:      q.op.doc,
			inSchema: typ.(*astType).inSchema,
			vars:     jwtVars,
		},
		sel: q.sel}
}

func (q *query) Rename(newName string) {
	q.field.Name = newName
}

func (q *query) Name() string {
	return (*field)(q).Name()
}

func (q *query) Alias() string {
	return (*field)(q).Alias()
}

func (q *query) DgraphAlias() string {
	return q.Name()
}

func (q *query) SetArgTo(arg string, val interface{}) {
	(*field)(q).SetArgTo(arg, val)
}

func (q *query) Arguments() map[string]interface{} {
	return (*field)(q).Arguments()
}

func (q *query) ArgValue(name string) interface{} {
	return (*field)(q).ArgValue(name)
}

func (q *query) IsArgListType(name string) bool {
	return (*field)(q).IsArgListType(name)
}

func (q *query) Skip() bool {
	return false
}

func (q *query) Include() bool {
	return true
}

func (q *query) Cascade() []string {
	return (*field)(q).Cascade()
}

func (q *query) HasCustomDirective() (bool, map[string]bool) {
	return (*field)(q).HasCustomDirective()
}

func (q *query) HasLambdaDirective() bool {
	return (*field)(q).HasLambdaDirective()
}

func (q *query) IDArgValue() (*string, uint64, error) {
	return (*field)(q).IDArgValue()
}

func (q *query) XIDArg() string {
	return (*field)(q).XIDArg()
}

func (q *query) Type() Type {
	return (*field)(q).Type()
}

func (q *query) SelectionSet() []Field {
	return (*field)(q).SelectionSet()
}

func (q *query) Location() x.Location {
	return (*field)(q).Location()
}

func (q *query) ResponseName() string {
	return (*field)(q).ResponseName()
}

func (q *query) GetObjectName() string {
	return q.field.ObjectDefinition.Name
}

func (q *query) CustomHTTPConfig() (FieldHTTPConfig, error) {
	return getCustomHTTPConfig((*field)(q), true)
}

func (q *query) EnumValues() []string {
	return nil
}

func (q *query) QueryType() QueryType {
	return queryType(q.Name(), q.op.inSchema.customDirectives["Query"][q.Name()])
}

func (q *query) DQLQuery() string {
	if customDir := q.op.inSchema.customDirectives["Query"][q.Name()]; customDir != nil {
		if dqlArgument := customDir.Arguments.ForName(dqlArg); dqlArgument != nil {
			return dqlArgument.Value.Raw
		}
	}
	return ""
}

func queryType(name string, custom *ast.Directive) QueryType {
	switch {
	case custom != nil:
		if custom.Arguments.ForName(dqlArg) != nil {
			return DQLQuery
		}
		return HTTPQuery
	case strings.HasPrefix(name, "get"):
		return GetQuery
	case name == "__schema" || name == "__type" || name == "__typename":
		return SchemaQuery
	case strings.HasPrefix(name, "query"):
		return FilterQuery
	case strings.HasPrefix(name, "check"):
		return PasswordQuery
	default:
		return NotSupportedQuery
	}
}

func (q *query) Operation() Operation {
	return (*field)(q).Operation()
}

func (q *query) DgraphPredicate() string {
	return (*field)(q).DgraphPredicate()
}

func (q *query) InterfaceType() bool {
	return (*field)(q).InterfaceType()
}

func (q *query) TypeName(dgraphTypes []interface{}) string {
	return (*field)(q).TypeName(dgraphTypes)
}

func (q *query) IncludeInterfaceField(dgraphTypes []interface{}) bool {
	return (*field)(q).IncludeInterfaceField(dgraphTypes)
}

func (m *mutation) Name() string {
	return (*field)(m).Name()
}

func (m *mutation) Alias() string {
	return (*field)(m).Alias()
}

func (m *mutation) DgraphAlias() string {
	return m.Name()
}

func (m *mutation) SetArgTo(arg string, val interface{}) {
	(*field)(m).SetArgTo(arg, val)
}

func (m *mutation) IsArgListType(name string) bool {
	return (*field)(m).IsArgListType(name)
}

func (m *mutation) Arguments() map[string]interface{} {
	return (*field)(m).Arguments()
}

func (m *mutation) ArgValue(name string) interface{} {
	return (*field)(m).ArgValue(name)
}

func (m *mutation) Skip() bool {
	return false
}

func (m *mutation) Include() bool {
	return true
}

func (m *mutation) Cascade() []string {
	return (*field)(m).Cascade()
}

func (m *mutation) HasCustomDirective() (bool, map[string]bool) {
	return (*field)(m).HasCustomDirective()
}

func (m *mutation) HasLambdaDirective() bool {
	return (*field)(m).HasLambdaDirective()
}

func (m *mutation) Type() Type {
	return (*field)(m).Type()
}

func (m *mutation) InterfaceType() bool {
	return (*field)(m).InterfaceType()
}

func (m *mutation) XIDArg() string {
	return (*field)(m).XIDArg()
}

func (m *mutation) IDArgValue() (*string, uint64, error) {
	return (*field)(m).IDArgValue()
}

func (m *mutation) SelectionSet() []Field {
	return (*field)(m).SelectionSet()
}

func (m *mutation) QueryField() Field {
	for _, f := range m.SelectionSet() {
		if f.Name() == NumUid || f.Name() == Typename || f.Name() == Msg {
			continue
		}
		// if @cascade was given on mutation itself, then it should get applied for the query which
		// gets executed to fetch the results of that mutation, so propagating it to the QueryField.
		if len(m.Cascade()) != 0 && len(f.Cascade()) == 0 {
			field := f.(*field).field
			field.Directives = append(field.Directives, &ast.Directive{Name: cascadeDirective})
		}
		return f
	}
	return m.SelectionSet()[0]
}

func (m *mutation) NumUidsField() Field {
	for _, f := range m.SelectionSet() {
		if f.Name() == NumUid {
			return f
		}
	}
	return nil
}

func (m *mutation) Location() x.Location {
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
	return m.op.inSchema.mutatedType[m.Name()]
}

func (m *mutation) CustomHTTPConfig() (FieldHTTPConfig, error) {
	return getCustomHTTPConfig((*field)(m), true)
}

func (m *mutation) EnumValues() []string {
	return nil
}

func (m *mutation) GetObjectName() string {
	return m.field.ObjectDefinition.Name
}

func (m *mutation) MutationType() MutationType {
	return mutationType(m.Name(), m.op.inSchema.customDirectives["Mutation"][m.Name()])
}

func mutationType(name string, custom *ast.Directive) MutationType {
	switch {
	case custom != nil:
		return HTTPMutation
	case strings.HasPrefix(name, "add"):
		return AddMutation
	case strings.HasPrefix(name, "update"):
		return UpdateMutation
	case strings.HasPrefix(name, "delete"):
		return DeleteMutation
	default:
		return NotSupportedMutation
	}
}

func (m *mutation) Operation() Operation {
	return (*field)(m).Operation()
}

func (m *mutation) DgraphPredicate() string {
	return (*field)(m).DgraphPredicate()
}

func (m *mutation) TypeName(dgraphTypes []interface{}) string {
	return (*field)(m).TypeName(dgraphTypes)
}

func (m *mutation) IncludeInterfaceField(dgraphTypes []interface{}) bool {
	return (*field)(m).IncludeInterfaceField(dgraphTypes)
}

func (m *mutation) IsAuthQuery() bool {
	return (*field)(m).field.Arguments.ForName("dgraph.uid") != nil
}

func (t *astType) AuthRules() *TypeAuth {
	return t.inSchema.authRules[t.DgraphName()]
}

func (t *astType) IsPoint() bool {
	return t.Name() == "Point"
}

func (t *astType) Field(name string) FieldDefinition {
	return &fieldDefinition{
		// this ForName lookup is a loop in the underlying schema :-(
		fieldDef:        t.inSchema.schema.Types[t.Name()].Fields.ForName(name),
		inSchema:        t.inSchema,
		dgraphPredicate: t.dgraphPredicate,
		parentType:      t,
	}
}

func (t *astType) Fields() []FieldDefinition {
	var result []FieldDefinition

	for _, fld := range t.inSchema.schema.Types[t.Name()].Fields {
		result = append(result,
			&fieldDefinition{
				fieldDef:        fld,
				inSchema:        t.inSchema,
				dgraphPredicate: t.dgraphPredicate,
				parentType:      t,
			})
	}

	return result
}

func (fd *fieldDefinition) Name() string {
	return fd.fieldDef.Name
}

func (fd *fieldDefinition) DgraphAlias() string {
	if fd.inSchema.repeatedFieldNames[fd.Name()] {
		return fd.DgraphPredicate()
	}
	return fd.Name()
}

func (fd *fieldDefinition) DgraphPredicate() string {
	return fd.dgraphPredicate[fd.parentType.Name()][fd.Name()]
}

func (fd *fieldDefinition) IsID() bool {
	return isID(fd.fieldDef)
}

func (fd *fieldDefinition) HasIDDirective() bool {
	if fd.fieldDef == nil {
		return false
	}
	return hasIDDirective(fd.fieldDef)
}

func hasIDDirective(fd *ast.FieldDefinition) bool {
	id := fd.Directives.ForName("id")
	return id != nil
}

func isID(fd *ast.FieldDefinition) bool {
	return fd.Type.Name() == "ID"
}

func (fd *fieldDefinition) Type() Type {
	return &astType{
		typ:             fd.fieldDef.Type,
		inSchema:        fd.inSchema,
		dgraphPredicate: fd.dgraphPredicate,
	}
}

func (fd *fieldDefinition) ParentType() Type {
	return fd.parentType
}

func (fd *fieldDefinition) Inverse() FieldDefinition {

	invDirective := fd.fieldDef.Directives.ForName(inverseDirective)
	if invDirective == nil {
		return nil
	}

	invFieldArg := invDirective.Arguments.ForName(inverseArg)
	if invFieldArg == nil {
		return nil // really not possible
	}

	typeWrapper := fd.Type()
	// typ must exist if the schema passed GQL validation
	typ := fd.inSchema.schema.Types[typeWrapper.Name()]

	// fld must exist if the schema passed our validation
	fld := typ.Fields.ForName(invFieldArg.Value.Raw)

	return &fieldDefinition{
		fieldDef:        fld,
		inSchema:        fd.inSchema,
		dgraphPredicate: fd.dgraphPredicate,
		parentType:      typeWrapper,
	}
}

// ForwardEdge gets the field definition for a forward edge if this field is a reverse edge
// i.e. if it has a dgraph directive like
// @dgraph(name: "~movies")
func (fd *fieldDefinition) ForwardEdge() FieldDefinition {
	dd := fd.fieldDef.Directives.ForName(dgraphDirective)
	if dd == nil {
		return nil
	}

	arg := dd.Arguments.ForName(dgraphPredArg)
	if arg == nil {
		return nil // really not possible
	}
	name := arg.Value.Raw

	if !strings.HasPrefix(name, "~") && !strings.HasPrefix(name, "<~") {
		return nil
	}

	fedge := strings.Trim(name, "<~>")
	typeWrapper := fd.Type()
	// typ must exist if the schema passed GQL validation
	typ := fd.inSchema.schema.Types[typeWrapper.Name()]

	var fld *ast.FieldDefinition
	// Have to range through all the fields and find the correct forward edge. This would be
	// expensive and should ideally be cached on schema update.
	for _, field := range typ.Fields {
		dir := field.Directives.ForName(dgraphDirective)
		if dir == nil {
			continue
		}
		predArg := dir.Arguments.ForName(dgraphPredArg)
		if predArg == nil || predArg.Value.Raw == "" {
			continue
		}
		if predArg.Value.Raw == fedge {
			fld = field
			break
		}
	}

	return &fieldDefinition{
		fieldDef:        fld,
		inSchema:        fd.inSchema,
		dgraphPredicate: fd.dgraphPredicate,
		parentType:      typeWrapper,
	}
}

func (t *astType) Name() string {
	if t.typ.NamedType == "" {
		return t.typ.Elem.NamedType
	}
	return t.typ.NamedType
}

func (t *astType) DgraphName() string {
	typeDef := t.inSchema.schema.Types[t.typ.Name()]
	name := typeName(typeDef)
	if name != "" {
		return name
	}
	return t.Name()
}

func (t *astType) Nullable() bool {
	return !t.typ.NonNull
}

func (t *astType) ListType() Type {
	if t.typ == nil || t.typ.Elem == nil {
		return nil
	}
	return &astType{
		typ:             t.typ.Elem,
		inSchema:        t.inSchema,
		dgraphPredicate: t.dgraphPredicate}
}

// DgraphPredicate returns the name of the predicate in Dgraph that represents this
// type's field fld.  Mostly this will be type_name.field_name,.
func (t *astType) DgraphPredicate(fld string) string {
	return t.dgraphPredicate[t.Name()][fld]
}

func (t *astType) String() string {
	if t == nil {
		return ""
	}

	var sb strings.Builder
	// give it enough space in case it happens to be `[t.Name()!]!`
	sb.Grow(len(t.Name()) + 4)

	if t.ListType() == nil {
		x.Check2(sb.WriteString(t.Name()))
	} else {
		// There's no lists of lists, so this needn't be recursive
		x.Check2(sb.WriteRune('['))
		x.Check2(sb.WriteString(t.Name()))
		if !t.ListType().Nullable() {
			x.Check2(sb.WriteRune('!'))
		}
		x.Check2(sb.WriteRune(']'))
	}

	if !t.Nullable() {
		x.Check2(sb.WriteRune('!'))
	}

	return sb.String()
}

func (t *astType) IDField() FieldDefinition {
	def := t.inSchema.schema.Types[t.Name()]
	if def.Kind != ast.Object && def.Kind != ast.Interface {
		return nil
	}

	for _, fd := range def.Fields {
		if isID(fd) {
			return &fieldDefinition{
				fieldDef:   fd,
				inSchema:   t.inSchema,
				parentType: t,
			}
		}
	}

	return nil
}

func (t *astType) PasswordField() FieldDefinition {
	def := t.inSchema.schema.Types[t.Name()]
	if def.Kind != ast.Object && def.Kind != ast.Interface {
		return nil
	}

	fd := getPasswordField(def)
	if fd == nil {
		return nil
	}

	return &fieldDefinition{
		fieldDef:   fd,
		inSchema:   t.inSchema,
		parentType: t,
	}
}

func (t *astType) XIDField() FieldDefinition {
	def := t.inSchema.schema.Types[t.Name()]
	if def.Kind != ast.Object && def.Kind != ast.Interface {
		return nil
	}

	for _, fd := range def.Fields {
		if hasIDDirective(fd) {
			return &fieldDefinition{
				fieldDef:   fd,
				inSchema:   t.inSchema,
				parentType: t,
			}
		}
	}

	return nil
}

// InterfaceImplHasAuthRules checks if an interface's implementation has auth rules.
func (t *astType) InterfaceImplHasAuthRules() bool {
	schema := t.inSchema.schema
	types := schema.Types
	if typ, ok := types[t.Name()]; !ok || typ.Kind != ast.Interface {
		return false
	}

	for implName, implements := range schema.Implements {
		for _, intrface := range implements {
			if intrface.Name != t.Name() {
				continue
			}
			if val, ok := t.inSchema.authRules[implName]; ok && val.Rules != nil {
				return true
			}
		}
	}
	return false
}

func (t *astType) Interfaces() []string {
	interfaces := t.inSchema.schema.Types[t.typ.Name()].Interfaces
	if len(interfaces) == 0 {
		return nil
	}

	// Look up the interface types in the schema and find their typeName which could have been
	// overwritten using @dgraph(type: ...)
	names := make([]string, 0, len(interfaces))
	for _, intr := range interfaces {
		i := t.inSchema.schema.Types[intr]
		name := intr
		if n := typeName(i); n != "" {
			name = n
		}
		names = append(names, name)
	}
	return names
}

// CheckNonNulls checks that any non nullables in t are present in obj.
// Fields of type ID are not checked, nor is any exclusion.
//
// For our reference types for adding/linking objects, we'd like to have something like
//
// input PostRef {
// 	id: ID!
// }
//
// input PostNew {
// 	title: String!
// 	text: String
// 	author: AuthorRef!
// }
//
// and then have something like this
//
// input PostNewOrReference = PostRef | PostNew
//
// input AuthorNew {
//   ...
//   posts: [PostNewOrReference]
// }
//
// but GraphQL doesn't allow union types in input, so best we can do is
//
// input PostRef {
// 	id: ID
// 	title: String
// 	text: String
// 	author: AuthorRef
// }
//
// and then check ourselves that either there's an ID, or there's all the bits to
// satisfy a valid post.
func (t *astType) EnsureNonNulls(obj map[string]interface{}, exclusion string) error {
	for _, fld := range t.inSchema.schema.Types[t.Name()].Fields {
		if fld.Type.NonNull && !isID(fld) && fld.Name != exclusion {
			if val, ok := obj[fld.Name]; !ok || val == nil {
				return errors.Errorf(
					"type %s requires a value for field %s, but no value present",
					t.Name(), fld.Name)
			}
		}
	}
	return nil
}

// convertSliceToStringSlice converts any slice passed as argument to a slice of string
// Ensure that the argument is actually a slice, otherwise it will result in panic.
func convertSliceToStringSlice(slice interface{}) []string {
	val := reflect.ValueOf(slice)
	size := val.Len()
	list := make([]string, size)
	for i := 0; i < size; i++ {
		list[i] = fmt.Sprintf("%v", val.Index(i))
	}
	return list
}

func getAsPathParamValue(val interface{}) string {
	switch v := val.(type) {
	case string:
		return v
	case []string:
		return strings.Join(v, ",")
	case []bool, []int, []int8, []int16, []int32, []int64, []uint, []uint8, []uint16,
		[]uint32, []uint64, []float32, []float64:
		return strings.Join(convertSliceToStringSlice(v), ",")
	case []interface{}:
		return getAsInterfaceSliceInPath(v)
	case map[string]interface{}:
		return getAsMapInPath(v)
	default:
		return fmt.Sprintf("%v", val)
	}
}

func getAsInterfaceSliceInPath(slice []interface{}) string {
	var b strings.Builder
	size := len(slice)
	for i := 0; i < size; i++ {
		b.WriteString(getAsPathParamValue(slice[i]))
		if i != size-1 {
			b.WriteString(",")
		}
	}
	return b.String()
}

func getAsMapInPath(object map[string]interface{}) string {
	var b strings.Builder
	size := len(object)
	i := 1

	keys := make([]string, 0, size)
	for k := range object {
		keys = append(keys, k)
	}
	// ensure fixed order in output
	sort.Strings(keys)

	for _, k := range keys {
		b.WriteString(k)
		b.WriteString(",")
		b.WriteString(getAsPathParamValue(object[k]))
		if i != size {
			b.WriteString(",")
		}
		i++
	}
	return b.String()
}

func setQueryParamValue(queryParams url.Values, key string, val interface{}) {
	switch v := val.(type) {
	case []string:
		queryParams[key] = v
	case []bool, []int, []int8, []int16, []int32, []int64, []uint, []uint8, []uint16,
		[]uint32, []uint64, []float32, []float64:
		queryParams[key] = convertSliceToStringSlice(v)
	case []interface{}:
		setInterfaceSliceInQuery(queryParams, key, v)
	case map[string]interface{}:
		setMapInQuery(queryParams, key, v)
	default:
		queryParams.Add(key, fmt.Sprintf("%v", val))
	}
}

func setInterfaceSliceInQuery(queryParams url.Values, key string, slice []interface{}) {
	for _, val := range slice {
		setQueryParamValue(queryParams, key, val)
	}
}

func setMapInQuery(queryParams url.Values, key string, object map[string]interface{}) {
	for k, v := range object {
		k = fmt.Sprintf("%s[%s]", key, k)
		setQueryParamValue(queryParams, k, v)
	}
}

func SubstituteVarsInURL(rawURL string, vars map[string]interface{}) (string,
	error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}

	// Parse variables from path params.
	elems := strings.Split(u.Path, "/")
	rawPathSegments := make([]string, len(elems))
	for idx, elem := range elems {
		if strings.HasPrefix(elem, "$") {
			// see https://swagger.io/docs/specification/serialization/ to refer how different
			// kinds of parameters get serialized when they appear in path
			elems[idx] = getAsPathParamValue(vars[elem[1:]])
			rawPathSegments[idx] = url.PathEscape(elems[idx])
		} else {
			rawPathSegments[idx] = elem
		}
	}
	// we need both of them to make sure u.String() works correctly
	u.Path = strings.Join(elems, "/")
	u.RawPath = strings.Join(rawPathSegments, "/")

	// Parse variables from query params.
	q := u.Query()
	for k := range q {
		val := q.Get(k)
		if strings.HasPrefix(val, "$") {
			qv, ok := vars[val[1:]]
			if !ok {
				q.Del(k)
				continue
			}
			if qv == nil {
				qv = ""
			}
			// this ensures that any values added for this key by us are preserved,
			// while the value with $ is removed, as that will be the first value in list
			q[k] = q[k][1:]
			// see https://swagger.io/docs/specification/serialization/ to refer how different
			// kinds of parameters get serialized when they appear in query
			setQueryParamValue(q, k, qv)
		}
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func parseAsGraphQLArg(bodyTemplate string) (*ast.Value, error) {
	// bodyTemplate is always formed like { k1: 3.4, k2: $var, k3: "string", ...},
	// so the `input` arg here will have an object value after parsing
	doc, err := parser.ParseQuery(&ast.Source{Input: `query { dummy(input:` + bodyTemplate + `) }`})
	if err != nil {
		return nil, err
	}
	return doc.Operations[0].SelectionSet[0].(*ast.Field).Arguments[0].Value, nil
}

func parseAsJSONTemplate(value *ast.Value, vars map[string]bool, strictJSON bool) (interface{}, error) {
	switch value.Kind {
	case ast.ObjectValue:
		m := make(map[string]interface{})
		for _, elem := range value.Children {
			elemVal, err := parseAsJSONTemplate(elem.Value, vars, strictJSON)
			if err != nil {
				return nil, err
			}
			m[elem.Name] = elemVal
		}
		return m, nil
	case ast.ListValue:
		var l []interface{}
		for _, elem := range value.Children {
			elemVal, err := parseAsJSONTemplate(elem.Value, vars, strictJSON)
			if err != nil {
				return nil, err
			}
			l = append(l, elemVal)
		}
		return l, nil
	case ast.Variable:
		vars[value.Raw] = true
		return value.String(), nil
	case ast.IntValue:
		return strconv.ParseInt(value.Raw, 10, 64)
	case ast.FloatValue:
		return strconv.ParseFloat(value.Raw, 64)
	case ast.BooleanValue:
		return strconv.ParseBool(value.Raw)
	case ast.StringValue:
		return value.Raw, nil
	case ast.BlockValue, ast.EnumValue:
		if strictJSON {
			return nil, fmt.Errorf("found unexpected value: %s", value.String())
		}
		return value.Raw, nil
	case ast.NullValue:
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown value kind: %d, for value: %s", value.Kind, value.String())
	}
}

// Given a template for a body with variables defined, this function parses the body
// and converts it into a JSON representation and returns that. It also returns a list of the
// variable names that are required by this template.
// for e.g.
// { author: $id, post: { id: $postID }}
// would return
// { "author" : "$id", "post": { "id": "$postID" }} and { "id": true, "postID": true}
// If the final result is not a valid JSON, then an error is returned.
//
// In strictJSON mode block strings and enums are invalid and throw an error.
// strictJSON should be false when the body template is being used for custom graphql arg parsing,
// otherwise it should be true.
func parseBodyTemplate(body string, strictJSON bool) (*interface{}, map[string]bool, error) {
	if strings.TrimSpace(body) == "" {
		return nil, nil, nil
	}

	parsedBodyTemplate, err := parseAsGraphQLArg(body)
	if err != nil {
		return nil, nil, err
	}

	requiredVariables := make(map[string]bool)
	jsonTemplate, err := parseAsJSONTemplate(parsedBodyTemplate, requiredVariables, strictJSON)
	if err != nil {
		return nil, nil, err
	}

	return &jsonTemplate, requiredVariables, nil
}

func isVar(key string) bool {
	return strings.HasPrefix(key, "$")
}

func substituteVarInMapInBody(object, variables map[string]interface{}) {
	for k, v := range object {
		switch val := v.(type) {
		case string:
			if isVar(val) {
				vval, ok := variables[val[1:]]
				if ok {
					object[k] = vval
				} else {
					delete(object, k)
				}
			}
		case map[string]interface{}:
			substituteVarInMapInBody(val, variables)
		case []interface{}:
			substituteVarInSliceInBody(val, variables)
		}
	}
}

func substituteVarInSliceInBody(slice []interface{}, variables map[string]interface{}) {
	for k, v := range slice {
		switch val := v.(type) {
		case string:
			if isVar(val) {
				slice[k] = variables[val[1:]]
			}
		case map[string]interface{}:
			substituteVarInMapInBody(val, variables)
		case []interface{}:
			substituteVarInSliceInBody(val, variables)
		}
	}
}

// Given a JSON representation for a body with variables defined, this function substitutes
// the variables and returns the final JSON.
// for e.g.
// {
//		"author" : "$id",
//		"name" : "Jerry",
//		"age" : 23,
//		"post": {
//			"id": "$postID"
//		}
// }
// with variables {"id": "0x3",	postID: "0x9"}
// should return
// {
//		"author" : "0x3",
//		"name" : "Jerry",
//		"age" : 23,
//		"post": {
//			"id": "0x9"
//		}
// }
func SubstituteVarsInBody(jsonTemplate *interface{}, variables map[string]interface{}) {
	if jsonTemplate == nil {
		return
	}

	switch val := (*jsonTemplate).(type) {
	case string:
		if isVar(val) {
			*jsonTemplate = variables[val[1:]]
		}
	case map[string]interface{}:
		substituteVarInMapInBody(val, variables)
	case []interface{}:
		substituteVarInSliceInBody(val, variables)
	}
}

// FieldOriginatedFrom returns the name of the interface from which given field was inherited.
// If the field wasn't inherited, but belonged to this type, this type's name is returned.
// Otherwise, empty string is returned.
func (t *astType) FieldOriginatedFrom(fieldName string) string {
	for _, implements := range t.inSchema.schema.Implements[t.Name()] {
		if implements.Fields.ForName(fieldName) != nil {
			return implements.Name
		}
	}

	if t.inSchema.schema.Types[t.Name()].Fields.ForName(fieldName) != nil {
		return t.Name()
	}

	return ""
}

// buildGraphqlRequestFields will build graphql request body from ast.
// for eg:
// Hello{
// 	name {
// 		age
// 	}
// 	friend
// }
// will return
// {
// 	name {
// 		age
// 	}
// 	friend
// }
func buildGraphqlRequestFields(writer *bytes.Buffer, field *ast.Field) {
	// Add beginning curly braces
	if len(field.SelectionSet) == 0 {
		return
	}
	writer.WriteString("{\n")
	for i := 0; i < len(field.SelectionSet); i++ {
		castedField := field.SelectionSet[i].(*ast.Field)
		writer.WriteString(castedField.Name)

		if len(castedField.Arguments) > 0 {
			writer.WriteString("(")
			for idx, arg := range castedField.Arguments {
				if idx != 0 {
					writer.WriteString(", ")
				}
				writer.WriteString(arg.Name)
				writer.WriteString(": ")
				writer.WriteString(arg.Value.String())
			}
			writer.WriteString(")")
		}

		if len(castedField.SelectionSet) > 0 {
			// recursively add fields.
			buildGraphqlRequestFields(writer, castedField)
		}
		writer.WriteString("\n")
	}
	// Add ending curly braces
	writer.WriteString("}")
}

// parseRequiredArgsFromGQLRequest parses a GraphQL request and gets the arguments required by it.
func parseRequiredArgsFromGQLRequest(req string) (map[string]bool, error) {
	// Single request would be of the form query($id: ID!) { userName(id: $id)}
	// There are two ( here, one for defining the variables and other for the query arguments.
	// We need to fetch the query arguments here.

	// The request can contain nested arguments/variables as well, so we get the args here and
	// then wrap them with { } to pass to parseBodyTemplate to get the required fields.

	bracket := strings.Index(req, "{")
	req = req[bracket:]
	args := req[strings.Index(req, "(")+1 : strings.LastIndex(req, ")")]
	_, rf, err := parseBodyTemplate("{"+args+"}", false)
	return rf, err
}
