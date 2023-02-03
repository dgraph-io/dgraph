/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
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
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"

	"github.com/dgraph-io/dgraph/graphql/authorization"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/gqlparser/v2/ast"
	"github.com/dgraph-io/gqlparser/v2/parser"
)

// Wrap the github.com/dgraph-io/gqlparser/ast defintions so that the bulk of the GraphQL
// algorithm and interface is dependent on behaviours we expect from a GraphQL schema
// and validation, but not dependent the exact structure in the gqlparser.
//
// This also auto hooks up some bookkeeping that's otherwise no fun.  E.g. getting values for
// field arguments requires the variable map from the operation - so we'd need to carry vars
// through all the resolver functions.  Much nicer if they are resolved by magic here.

var (
	trueVal  = true
	falseVal = false
)

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
	Template       interface{}
	Mode           string
	ForwardHeaders http.Header
	// would be empty for non-GraphQL requests
	RemoteGqlQueryName string
	RemoteGqlQuery     string

	// For the following request
	// graphql: "query($sinput: [SchoolInput]) { schoolNames(schools: $sinput) }"
	// the GraphqlBatchModeArgument would be sinput, we use it to know the GraphQL variable that
	// we should send the data in.
	GraphqlBatchModeArgument string
}

// EntityRepresentations is the parsed form of the `representations` argument in `_entities` query
type EntityRepresentations struct {
	TypeDefn Type            // the type corresponding to __typename in the representations argument
	KeyField FieldDefinition // the definition of the @key field
	KeyVals  []interface{}   // the list of values corresponding to the key field
	// a map of key field value to the input representation for that value. The keys in this map
	// are the string formatted version of the key field value.
	KeyValToRepresentation map[string]map[string]interface{}
}

// Query/Mutation types and arg names
const (
	GetQuery             QueryType    = "get"
	FilterQuery          QueryType    = "query"
	AggregateQuery       QueryType    = "aggregate"
	SchemaQuery          QueryType    = "schema"
	EntitiesQuery        QueryType    = "entities"
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
	UpsertArgName                     = "upsert"
	FilterArgName                     = "filter"
)

// Schema represents a valid GraphQL schema
type Schema interface {
	Operation(r *Request) (Operation, error)
	Queries(t QueryType) []string
	Mutations(t MutationType) []string
	IsFederated() bool
	SetMeta(meta *metaInfo)
	Meta() *metaInfo
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
	CacheControl() string
}

// A Field is one field from an Operation.
type Field interface {
	Name() string
	Alias() string
	// DgraphAlias is used as an alias in DQL while rewriting the GraphQL field.
	DgraphAlias() string
	ResponseName() string
	RemoteResponseName() string
	Arguments() map[string]interface{}
	ArgValue(name string) interface{}
	IsArgListType(name string) bool
	IDArgValue() (map[string]string, uint64, error)
	XIDArgs() map[string]string
	SetArgTo(arg string, val interface{})
	Skip() bool
	Include() bool
	// SkipField tells whether to skip this field during completion or not based on:
	//  * @skip
	//  * @include
	//  * __typename: used for skipping fields in abstract types
	//  * seenField: used for skipping when the field has already been seen at the current level
	SkipField(dgraphTypes []string, seenField map[string]bool) bool
	Cascade() []string
	// ApolloRequiredFields returns the fields names which were specified in @requires.
	ApolloRequiredFields() []string
	// CustomRequiredFields returns a map from DgraphAlias to the field definition of the fields
	// which are required to resolve this custom field.
	CustomRequiredFields() map[string]FieldDefinition
	// IsCustomHTTP tells whether this field has @custom(http: {...}) directive on it.
	IsCustomHTTP() bool
	// HasCustomHTTPChild tells whether any descendent of this field has @custom(http: {...}) on it.
	HasCustomHTTPChild() bool
	HasLambdaDirective() bool
	Type() Type
	IsExternal() bool
	SelectionSet() []Field
	Location() x.Location
	DgraphPredicate() string
	Operation() Operation
	// AbstractType tells us whether this field represents a GraphQL Interface/Union.
	AbstractType() bool
	IncludeAbstractField(types []string) bool
	TypeName(dgraphTypes []string) string
	GetObjectName() string
	IsAuthQuery() bool
	CustomHTTPConfig() (*FieldHTTPConfig, error)
	EnumValues() []string
	ConstructedFor() Type
	ConstructedForDgraphPredicate() string
	DgraphPredicateForAggregateField() string
	IsAggregateField() bool
	GqlErrorf(path []interface{}, message string, args ...interface{}) *x.GqlError
	// MaxPathLength finds the max length (including list indexes) of any path in the 'query' f.
	MaxPathLength() int
	// PreAllocatePathSlice is used to pre-allocate a path buffer of the correct size before running
	// CompleteObject on the top level query - means that we aren't reallocating slices multiple
	// times during the complete* functions.
	PreAllocatePathSlice() []interface{}
	// NullValue returns the appropriate null bytes to be written as value in the JSON response for
	// this field.
	//  * If this field is a list field then it returns []byte("[]").
	//  * If it is nullable, it returns []byte("null").
	//  * Otherwise, this field is non-nullable and so it will return a nil slice to indicate that.
	NullValue() []byte
	// NullResponse returns the bytes representing a JSON object to be used for setting the Data
	// field of a Resolved, when this field resolves to a NullValue.
	//  * If this field is a list field then it returns []byte(`{"fieldAlias":[]}`).
	//  * If it is nullable, it returns []byte(`{"fieldAlias":null}`).
	//  * Otherwise, this field is non-nullable and so it will return a nil slice to indicate that.
	// This is useful only for top-level fields like a query or mutation.
	NullResponse() []byte
	// CompleteAlias applies GraphQL alias completion for field to the input buffer buf.
	CompleteAlias(buf *bytes.Buffer)
	// GetAuthMeta returns the Dgraph.Authorization meta information stored in schema
	GetAuthMeta() *authorization.AuthMeta
}

// A Mutation is a field (from the schema's Mutation type) from an Operation
type Mutation interface {
	Field
	MutationType() MutationType
	MutatedType() Type
	QueryField() Field
	NumUidsField() Field
	HasLambdaOnMutate() bool
}

// A Query is a field (from the schema's Query type) from an Operation
type Query interface {
	Field
	QueryType() QueryType
	DQLQuery() string
	Rename(newName string)
	// RepresentationsArg returns a parsed version of the `representations` argument for `_entities`
	// query
	RepresentationsArg() (*EntityRepresentations, error)
	AuthFor(jwtVars map[string]interface{}) Query
}

// A Type is a GraphQL type like: Float, T, T! and [T!]!.  If it's not a list, then
// ListType is nil.  If it's an object type then Field gets field definitions by
// name from the definition of the type; IDField gets the ID field of the type.
type Type interface {
	Field(name string) FieldDefinition
	Fields() []FieldDefinition
	IDField() FieldDefinition
	XIDFields() []FieldDefinition
	InterfaceImplHasAuthRules() bool
	PasswordField() FieldDefinition
	Name() string
	DgraphName() string
	DgraphPredicate(fld string) string
	Nullable() bool
	// true if this is a union type
	IsUnion() bool
	IsInterface() bool
	// returns a list of member types for this union
	UnionMembers([]interface{}) []Type
	ListType() Type
	Interfaces() []string
	ImplementingTypes() []Type
	EnsureNonNulls(map[string]interface{}, string) error
	FieldOriginatedFrom(fieldName string) string
	AuthRules() *TypeAuth
	IsGeo() bool
	IsAggregateResult() bool
	IsInbuiltOrEnumType() bool
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
	IsExternal() bool
	HasIDDirective() bool
	Inverse() FieldDefinition
	WithMemberType(string) FieldDefinition
	// TODO - It might be possible to get rid of ForwardEdge and just use Inverse() always.
	ForwardEdge() FieldDefinition
	// GetAuthMeta returns the Dgraph.Authorization meta information stored in schema
	GetAuthMeta() *authorization.AuthMeta
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
	// lambdaOnMutate stores the mapping of mutationName -> true, if the config of @lambdaOnMutate
	// enables lambdas for that mutation.
	// It is read-only.
	lambdaOnMutate map[string]bool
	// requiresDirectives stores the mapping of typeName->fieldName->list of fields given in
	// @requires. It is read-only.
	requiresDirectives map[string]map[string][]string
	// remoteResponse stores the mapping of typeName->fieldName->responseName which will be used in result
	// completion step.
	remoteResponse map[string]map[string]string
	// Map from typename to auth rules
	authRules map[string]*TypeAuth
	// meta is the meta information extracted from input schema
	meta *metaInfo
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
	// hasCustomHTTPChild is used to cache whether any of the descendents of this field have a
	// @custom(http: {...}) on them. Its type is purposefully set to *bool to find out whether
	// this flag has already been calculated or not. If not calculated, it would be nil.
	// Otherwise, it would always contain a boolean value.
	hasCustomHTTPChild *bool
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

func (s *schema) IsFederated() bool {
	return s.schema.Types["_Entity"] != nil
}

func (s *schema) SetMeta(meta *metaInfo) {
	s.meta = meta
}

func (s *schema) Meta() *metaInfo {
	return s.meta
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

func (o *operation) CacheControl() string {
	if o.op.Directives.ForName(cacheControlDirective) == nil {
		return ""
	}
	return "public,max-age=" + o.op.Directives.ForName(cacheControlDirective).Arguments[0].Value.Raw
}

// parentInterface returns the name of an interface that a field belonging to a type definition
// typDef inherited from. If there is no such interface, then it returns an empty string.
//
// Given the following schema
//
//	interface A {
//	  name: String
//	}
//
//	type B implements A {
//		 name: String
//	  age: Int
//	}
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
		isInputTypeGeo := func(typName string) bool {
			return typName == "Point" || typName == "PointList" || typName == "Polygon" || typName == "MultiPolygon"
		}
		if inputTyp.BuiltIn || isQueryOrMutationType(inputTyp) || inputTyp.Name == "Subscription" ||
			(inputTyp.Kind != ast.Object && inputTyp.Kind != ast.Interface) || isInputTypeGeo(inputTyp.Name) {
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
			// If key field is of ID type but it is an external field,
			// then it is stored in Dgraph as string type with Hash index.
			// So we need the predicate mapping in this case.
			if isID(fld) && !hasExternal(fld) {
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

// customAndLambdaMappings does following things:
//   - If there is @custom on any field, it removes the directive from the list of directives on
//     that field. Instead, it puts it in a map of typeName->fieldName->custom directive definition.
//     This mapping is returned as the first return value, which is later used to determine if some
//     field has custom directive or not, and accordingly construct the HTTP request for the field.
//   - If there is @lambda on any field, it removes the directive from the list of directives on
//     that field. Instead, it puts it in a map of typeName->fieldName->bool. This mapping is returned
//     as the second return value, which is later used to determine if some field has lambda directive
//     or not. An appropriate @custom directive is also constructed for the field with @lambda and
//     put into the first mapping. Both of these mappings together are used to construct the HTTP
//     request for @lambda field. Internally, @lambda is just @custom(http: {
//     url: "<graphql_lambda_url: a-fixed-pre-defined-url>",
//     method: POST,
//     body: "<all-the-args-for-a-query-or-mutation>/<all-the-scalar-fields-from-parent-type-for-a
//     -@lambda-on-field>"
//     mode: BATCH (set only if @lambda was on a non query/mutation field)
//     })
//     So, by constructing an appropriate custom directive for @lambda fields,
//     we just reuse logic from @custom.
func customAndLambdaMappings(s *ast.Schema, ns uint64) (map[string]map[string]*ast.Directive,
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
							dir, ns, func(f *ast.FieldDefinition) bool {
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

func requiresMappings(s *ast.Schema) map[string]map[string][]string {
	requiresDirectives := make(map[string]map[string][]string)

	for _, typ := range s.Types {
		for _, f := range typ.Fields {
			for i, dir := range f.Directives {
				if dir.Name != apolloRequiresDirective {
					continue
				}
				lastIndex := len(f.Directives) - 1
				f.Directives[i] = f.Directives[lastIndex]
				f.Directives = f.Directives[:lastIndex]

				var fieldMap map[string][]string
				if existingFieldMap, ok := requiresDirectives[typ.Name]; ok {
					fieldMap = existingFieldMap
				} else {
					fieldMap = make(map[string][]string)
				}

				fieldMap[f.Name] = strings.Fields(dir.Arguments[0].Value.Raw)
				requiresDirectives[typ.Name] = fieldMap

				break
			}
		}
	}
	return requiresDirectives
}

func remoteResponseMapping(s *ast.Schema) map[string]map[string]string {
	remoteResponse := make(map[string]map[string]string)
	for _, typ := range s.Types {
		for _, field := range typ.Fields {
			for i, dir := range field.Directives {
				if dir.Name != remoteResponseDirective {
					continue
				}
				lastIndex := len(field.Directives) - 1
				field.Directives[i] = field.Directives[lastIndex]
				field.Directives = field.Directives[:lastIndex]

				var remoteFieldMap map[string]string
				if existingRemoteFieldMap, ok := remoteResponse[typ.Name]; ok {
					remoteFieldMap = existingRemoteFieldMap
				} else {
					remoteFieldMap = make(map[string]string)
				}

				remoteFieldMap[field.Name] = dir.Arguments[0].Value.Raw
				remoteResponse[typ.Name] = remoteFieldMap

				break
			}
		}
	}
	return remoteResponse
}

func hasExtends(def *ast.Definition) bool {
	return def.Directives.ForName(apolloExtendsDirective) != nil
}

func hasExternal(f *ast.FieldDefinition) bool {
	return f.Directives.ForName(apolloExternalDirective) != nil
}

func (f *field) IsExternal() bool {
	return hasExternal(f.field.Definition)
}

func (q *query) IsExternal() bool {
	return (*field)(q).IsExternal()
}

func (m *mutation) IsExternal() bool {
	return (*field)(m).IsExternal()
}

func (fd *fieldDefinition) IsExternal() bool {
	return hasExternal(fd.fieldDef)
}

func hasCustomOrLambda(f *ast.FieldDefinition) bool {
	for _, dir := range f.Directives {
		if dir.Name == customDirective || dir.Name == lambdaDirective {
			return true
		}
	}
	return false
}

func isKeyField(f *ast.FieldDefinition, typ *ast.Definition) bool {
	keyDirective := typ.Directives.ForName(apolloKeyDirective)
	if keyDirective == nil {
		return false
	}
	return f.Name == keyDirective.Arguments[0].Value.Raw
}

// Filter out those fields which have @external directive and are not @key fields
// in a definition.
func nonExternalAndKeyFields(defn *ast.Definition) ast.FieldList {
	fldList := make([]*ast.FieldDefinition, 0)
	for _, fld := range defn.Fields {
		if hasExternal(fld) && !isKeyField(fld, defn) {
			continue
		}
		fldList = append(fldList, fld)
	}
	return fldList
}

// externalAndNonKeyField returns true for those fields which have @external directive and
// are not @key fields and are not an argument to the @provides directive.
func externalAndNonKeyField(fld *ast.FieldDefinition, defn *ast.Definition, providesTypeMap map[string]bool) bool {
	return hasExternal(fld) && !isKeyField(fld, defn) && !providesTypeMap[fld.Name]
}

// buildCustomDirectiveForLambda returns custom directive for the given field to be used for @lambda
// The constructed @custom looks like this:
//
//	@custom(http: {
//	   url: "<graphql_lambda_url: a-fixed-pre-defined-url>",
//	   method: POST,
//	   body: "<all-the-args-for-a-query-or-mutation>/<all-the-scalar-fields-from-parent-type-for-a
//	   -@lambda-on-field>"
//	   mode: BATCH (set only if @lambda was on a non query/mutation field)
//	})
func buildCustomDirectiveForLambda(defn *ast.Definition, field *ast.FieldDefinition,
	lambdaDir *ast.Directive, ns uint64, skipInBodyTemplate func(f *ast.FieldDefinition) bool) *ast.
	Directive {
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
		getChildValue(httpUrl, x.LambdaUrl(ns), ast.StringValue, lambdaDir.Position),
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

func lambdaOnMutateMappings(s *ast.Schema) map[string]bool {
	result := make(map[string]bool)
	for _, typ := range s.Types {
		dir := typ.Directives.ForName(lambdaOnMutateDirective)
		if dir == nil {
			continue
		}

		for _, arg := range dir.Arguments {
			value, _ := arg.Value.Value(nil)
			if val, ok := value.(bool); ok && val {
				result[arg.Name+typ.Name] = true
			}
		}
	}
	return result
}

// AsSchema wraps a github.com/dgraph-io/gqlparser/ast.Schema.
func AsSchema(s *ast.Schema, ns uint64) (Schema, error) {
	customDirs, lambdaDirs := customAndLambdaMappings(s, ns)
	dgraphPredicate := dgraphMapping(s)
	sch := &schema{
		schema:             s,
		dgraphPredicate:    dgraphPredicate,
		typeNameAst:        typeMappings(s),
		customDirectives:   customDirs,
		lambdaDirectives:   lambdaDirs,
		lambdaOnMutate:     lambdaOnMutateMappings(s),
		requiresDirectives: requiresMappings(s),
		remoteResponse:     remoteResponseMapping(s),
		meta:               &metaInfo{}, // initialize with an empty metaInfo
	}
	sch.mutatedType = mutatedTypeMapping(sch, dgraphPredicate)
	// Auth rules can't be effectively validated as part of the normal rules -
	// because they need the fully generated schema to be checked against.
	var err error
	sch.authRules, err = authRules(sch)
	if err != nil {
		return nil, err
	}

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
	return f.field.ObjectDefinition.Name + "." + f.field.Alias
}

func (f *field) ResponseName() string {
	return responseName(f.field)
}

func (f *field) RemoteResponseName() string {
	remoteResponse := f.op.inSchema.remoteResponse[f.GetObjectName()][f.Name()]
	if remoteResponse == "" {
		return f.Name()
	}
	return remoteResponse
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

func (f *field) IsAggregateField() bool {
	return strings.HasSuffix(f.Name(), "Aggregate") && f.Type().IsAggregateResult()
}

func (f *field) GqlErrorf(path []interface{}, message string, args ...interface{}) *x.GqlError {
	pathCopy := make([]interface{}, len(path))
	copy(pathCopy, path)
	return &x.GqlError{
		Message:   fmt.Sprintf(message, args...),
		Locations: []x.Location{f.Location()},
		Path:      pathCopy,
	}
}

func (f *field) MaxPathLength() int {
	childMax := 0
	for _, child := range f.SelectionSet() {
		d := child.MaxPathLength()
		if d > childMax {
			childMax = d
		}
	}
	if f.Type().ListType() != nil {
		// It's f: [...], so add a space for field name and
		// a space for the index into the list
		return 2 + childMax
	}

	return 1 + childMax
}

func (f *field) PreAllocatePathSlice() []interface{} {
	return make([]interface{}, 0, f.MaxPathLength())
}

func (f *field) NullValue() []byte {
	typ := f.Type()
	if typ.ListType() != nil {
		// We could choose to set this to null.  This is our decision, not
		// anything required by the GraphQL spec.
		//
		// However, if we query, for example, for a person's friends with
		// some restrictions, and there aren't any, is that really a case to
		// set this at null and error if the list is required?  What
		// about if a person has just been added and doesn't have any friends?
		// Doesn't seem right to add null and cause error propagation.
		//
		// Seems best if we pick [], rather than null, as the list value if
		// there's nothing in the Dgraph result.
		return JsonEmptyList
	}

	if typ.Nullable() {
		return JsonNull
	}

	// this is a non-nullable field, so return a nil slice to indicate that.
	return nil
}

func (f *field) NullResponse() []byte {
	val := f.NullValue()
	if val == nil {
		// this is a non-nullable field, so return a nil slice to indicate that.
		return nil
	}

	key := []byte(f.ResponseName())

	buf := make([]byte, 0, 5+len(key)+len(val)) // 5 = 2 + 2 + 1
	buf = append(buf, '{', '"')
	buf = append(buf, key...)
	buf = append(buf, '"', ':')
	buf = append(buf, val...)
	buf = append(buf, '}')

	// finally return a JSON like: {"fieldAlias":null}
	return buf
}

func (f *field) CompleteAlias(buf *bytes.Buffer) {
	x.Check2(buf.WriteRune('"'))
	x.Check2(buf.WriteString(f.ResponseName()))
	x.Check2(buf.WriteString(`":`))
}

func (f *field) GetAuthMeta() *authorization.AuthMeta {
	return f.op.inSchema.meta.authMeta
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

func (f *field) SkipField(dgraphTypes []string, seenField map[string]bool) bool {
	if f.Skip() || !f.Include() {
		return true
	}
	// If typ is an abstract type, and typename is a concrete type, then we ignore fields which
	// aren't part of that concrete type. This would happen when multiple fragments (belonging
	// to different concrete types) are requested within a query for an abstract type.
	if !f.IncludeAbstractField(dgraphTypes) {
		return true
	}
	// if the field has already been seen at the current level, then we need to skip it.
	// Otherwise, mark it seen.
	if seenField[f.ResponseName()] {
		return true
	}
	seenField[f.ResponseName()] = true
	return false
}

func (f *field) Cascade() []string {
	dir := f.field.Directives.ForName(cascadeDirective)
	if dir == nil {
		return nil
	}
	fieldsVal, _ := dir.ArgumentMap(f.op.vars)[cascadeArg].([]interface{})
	if len(fieldsVal) == 0 {
		return []string{"__all__"}
	}

	fields := make([]string, 0)
	typ := f.Type()
	idField := typ.IDField()

	for _, value := range fieldsVal {
		if idField != nil && idField.Name() == value {
			fields = append(fields, "uid")
		} else {
			fields = append(fields, typ.DgraphPredicate(value.(string)))
		}

	}
	return fields
}

func toRequiredFieldDefs(requiredFieldNames map[string]bool, sibling *field) map[string]FieldDefinition {
	res := make(map[string]FieldDefinition, len(requiredFieldNames))
	parentType := &astType{
		typ:             &ast.Type{NamedType: sibling.field.ObjectDefinition.Name},
		inSchema:        sibling.op.inSchema,
		dgraphPredicate: sibling.op.inSchema.dgraphPredicate,
	}
	for rfName := range requiredFieldNames {
		fieldDef := parentType.Field(rfName)
		res[fieldDef.DgraphAlias()] = fieldDef
	}
	return res
}

func (f *field) ApolloRequiredFields() []string {
	return f.op.inSchema.requiresDirectives[f.GetObjectName()][f.Name()]
}

func (f *field) CustomRequiredFields() map[string]FieldDefinition {
	custom := f.op.inSchema.customDirectives[f.GetObjectName()][f.Name()]
	if custom == nil {
		return nil
	}

	httpArg := custom.Arguments.ForName(httpArg)
	if httpArg == nil {
		return nil
	}

	var rf map[string]bool
	bodyArg := httpArg.Value.Children.ForName(httpBody)
	graphqlArg := httpArg.Value.Children.ForName(httpGraphql)
	if bodyArg != nil {
		bodyTemplate := bodyArg.Raw
		_, rf, _ = parseBodyTemplate(bodyTemplate, graphqlArg == nil)
	}

	if rf == nil {
		rf = make(map[string]bool)
	}
	rawURL := httpArg.Value.Children.ForName(httpUrl).Raw
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
		return toRequiredFieldDefs(rf, f)
	}
	modeVal := SINGLE
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
			return nil
		}
	}
	return toRequiredFieldDefs(rf, f)
}

func (f *field) IsCustomHTTP() bool {
	custom := f.op.inSchema.customDirectives[f.GetObjectName()][f.Name()]
	if custom == nil {
		return false
	}

	return custom.Arguments.ForName(httpArg) != nil
}

func (f *field) HasCustomHTTPChild() bool {
	// let's see if we have already calculated whether this field has any custom http children
	if f.hasCustomHTTPChild != nil {
		return *(f.hasCustomHTTPChild)
	}
	// otherwise, we need to find out whether any descendents of this field have custom http
	selSet := f.SelectionSet()
	// this is a scalar field, so it can't even have a child => return false
	if len(selSet) == 0 {
		return false
	}
	// lets find if any direct child of this field has a @custom on it
	for _, fld := range selSet {
		if f.op.inSchema.customDirectives[fld.GetObjectName()][fld.Name()] != nil {
			f.hasCustomHTTPChild = &trueVal
			return true
		}
	}
	// if none of the direct child of this field have a @custom,
	// then lets see if any further descendents have @custom.
	for _, fld := range selSet {
		if fld.HasCustomHTTPChild() {
			f.hasCustomHTTPChild = &trueVal
			return true
		}
	}
	// if none of the descendents of this field have a @custom, return false
	f.hasCustomHTTPChild = &falseVal
	return false
}

func (f *field) HasLambdaDirective() bool {
	return f.op.inSchema.lambdaDirectives[f.GetObjectName()][f.Name()]
}

func (f *field) XIDArgs() map[string]string {
	xidToDgraphPredicate := make(map[string]string)
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
			xidToDgraphPredicate[arg.Name] = f.Type().DgraphPredicate(arg.Name)
		}
	}
	return xidToDgraphPredicate
}

func (f *field) IDArgValue() (xids map[string]string, uid uint64, err error) {
	idField := f.Type().IDField()
	passwordField := f.Type().PasswordField()
	xidArgName := ""
	xids = make(map[string]string)
	// This method is only called for Get queries and check. These queries can accept ID, XID
	// or Password. Therefore the non ID and Password field is an XID.
	// TODO maybe there is a better way to do this.
	for _, arg := range f.field.Arguments {
		if (idField == nil || arg.Name != idField.Name()) &&
			(passwordField == nil || arg.Name != passwordField.Name()) {
			xidArgName = arg.Name
		}

		if xidArgName != "" {
			var ok bool
			var xidArgVal string
			switch v := f.ArgValue(xidArgName).(type) {
			case int64:
				xidArgVal = strconv.FormatInt(v, 10)
			case string:
				xidArgVal = v
			default:
				pos := f.field.GetPosition()
				if !ok {
					err = x.GqlErrorf("Argument (%s) of %s was not able to be parsed as a string",
						xidArgName, f.Name()).WithLocations(x.Location{Line: pos.Line, Column: pos.Column})
					return
				}
			}
			xids[xidArgName] = xidArgVal
		}
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
		t = f.field.Definition.Type
	} else {
		// If f is a field that isn't defined in the schema, then it would have nil definition.
		// This can happen in case if the incoming request contains a query/mutation that isn't
		// defined in the schema being served. Resolving such a query would report that no
		// suitable resolver was found.
		// TODO: Ideally, this case shouldn't happen as the query isn't defined in the schema,
		// so it should be rejected by query validation itself. But, somehow that is not happening.

		// In this case we are returning a nullable type named "__Undefined__" from here, instead
		// of returning nil, so that the rest of the code can continue to work. The type is
		// nullable so that if the request contained other valid queries, they should still get a
		// data response.
		t = &ast.Type{NamedType: "__Undefined__", NonNull: false}
	}

	return &astType{
		typ:             t,
		inSchema:        f.op.inSchema,
		dgraphPredicate: f.op.inSchema.dgraphPredicate,
	}
}

func isAbstractKind(kind ast.DefinitionKind) bool {
	return kind == ast.Interface || kind == ast.Union
}

func (f *field) AbstractType() bool {
	return isAbstractKind(f.op.inSchema.schema.Types[f.field.Definition.Type.Name()].Kind)
}

func (f *field) GetObjectName() string {
	return f.field.ObjectDefinition.Name
}

func (t *astType) IsInbuiltOrEnumType() bool {
	_, ok := inbuiltTypeToDgraph[t.Name()]
	return ok || (t.inSchema.schema.Types[t.Name()].Kind == ast.Enum)
}

func getCustomHTTPConfig(f *field, isQueryOrMutation bool) (*FieldHTTPConfig, error) {
	custom := f.op.inSchema.customDirectives[f.GetObjectName()][f.Name()]
	httpArg := custom.Arguments.ForName(httpArg)
	fconf := &FieldHTTPConfig{
		URL:    httpArg.Value.Children.ForName(httpUrl).Raw,
		Method: httpArg.Value.Children.ForName(httpMethod).Raw,
	}

	fconf.Mode = SINGLE
	op := httpArg.Value.Children.ForName(mode)
	if op != nil {
		fconf.Mode = op.Raw
	}

	// both body and graphql can't be present together
	bodyArg := httpArg.Value.Children.ForName(httpBody)
	graphqlArg := httpArg.Value.Children.ForName(httpGraphql)
	var bodyTemplate string
	if bodyArg != nil {
		bodyTemplate = bodyArg.Raw
	} else if graphqlArg != nil {
		bodyTemplate = `{ query: $query, variables: $variables }`
	}
	// bodyTemplate will be empty if there was no body or graphql, like the case of a simple GET req
	if bodyTemplate != "" {
		bt, _, err := parseBodyTemplate(bodyTemplate, true)
		if err != nil {
			return nil, err
		}
		fconf.Template = bt
	}

	fconf.ForwardHeaders = http.Header{}
	// set application/json as the default Content-Type
	fconf.ForwardHeaders.Set("Content-Type", "application/json")
	secretHeaders := httpArg.Value.Children.ForName("secretHeaders")
	if secretHeaders != nil {
		for _, h := range secretHeaders.Children {
			key := strings.Split(h.Value.Raw, ":")
			if len(key) == 1 {
				key = []string{h.Value.Raw, h.Value.Raw}
			}
			val := string(f.op.inSchema.meta.secrets[key[1]])
			fconf.ForwardHeaders.Set(key[0], val)
		}
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
			return nil, gqlErr
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
				return nil, errors.Wrapf(err, "while substituting vars in URL")
			}
			bodyVars = argMap
		} else {
			bodyVars = make(map[string]interface{})
			bodyVars["query"] = fconf.RemoteGqlQuery
			bodyVars["variables"] = argMap
		}
		fconf.Template = SubstituteVarsInBody(fconf.Template, bodyVars)
	}
	return fconf, nil
}

func (f *field) CustomHTTPConfig() (*FieldHTTPConfig, error) {
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

func (f *field) TypeName(dgraphTypes []string) string {
	for _, typ := range dgraphTypes {
		for _, origTyp := range f.op.inSchema.typeNameAst[typ] {
			if origTyp.Kind != ast.Object {
				continue
			}
			return origTyp.Name
		}

	}
	return f.GetObjectName()
}

func (f *field) IncludeAbstractField(dgraphTypes []string) bool {
	if len(dgraphTypes) == 0 {
		// dgraph.type is returned only for fields on abstract types, so if there is no dgraph.type
		// information, then it means this ia a field on a concrete object type
		return true
	}
	// Given a list of dgraph types, we query the schema and find the one which is an ast.Object
	// and not an Interface object.
	for _, typ := range dgraphTypes {
		for _, origTyp := range f.op.inSchema.typeNameAst[typ] {
			if origTyp.Kind == ast.Object {
				// For fields coming from fragments inside an abstract type, there are two cases:
				// * If the field is from an interface implemented by this object, and was fetched
				//	 not because of a fragment on this object, but because of a fragment on some
				//	 other object, then we don't need to include it.
				// * If the field was fetched because of a fragment on an interface, and that
				//	 interface is not one of the interfaces implemented by this object, then we
				//	 don't need to include it.
				fragType, ok := f.op.interfaceImplFragFields[f.field]
				if ok && fragType != origTyp.Name && !x.HasString(origTyp.Interfaces, fragType) {
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

func (q *query) IsAggregateField() bool {
	return (*field)(q).IsAggregateField()
}

func (q *query) GqlErrorf(path []interface{}, message string, args ...interface{}) *x.GqlError {
	return (*field)(q).GqlErrorf(path, message, args...)
}

func (q *query) MaxPathLength() int {
	return (*field)(q).MaxPathLength()
}

func (q *query) PreAllocatePathSlice() []interface{} {
	return (*field)(q).PreAllocatePathSlice()
}

func (q *query) NullValue() []byte {
	return (*field)(q).NullValue()
}

func (q *query) NullResponse() []byte {
	return (*field)(q).NullResponse()
}

func (q *query) CompleteAlias(buf *bytes.Buffer) {
	(*field)(q).CompleteAlias(buf)
}

func (q *query) GetAuthMeta() *authorization.AuthMeta {
	return (*field)(q).GetAuthMeta()
}

func (q *query) RepresentationsArg() (*EntityRepresentations, error) {
	representations, ok := q.ArgValue("representations").([]interface{})
	if !ok {
		return nil, fmt.Errorf("error parsing `representations` argument")
	}
	if len(representations) == 0 {
		return nil, fmt.Errorf("expecting at least one item in `representations` argument")
	}
	representation, ok := representations[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("error parsing %dth item in the `_representations` argument", 0)
	}
	typename, ok := representation[Typename].(string)
	if !ok {
		return nil, fmt.Errorf("unable to extract __typename from %dth item in the"+
			" `_representations` argument", 0)
	}
	typ := q.op.inSchema.schema.Types[typename]
	if typ == nil {
		return nil, fmt.Errorf("type %s not found in the schema", typename)
	}
	keyDir := typ.Directives.ForName(apolloKeyDirective)
	if keyDir == nil {
		return nil, fmt.Errorf("type %s doesn't have a key Directive", typename)
	}
	keyFldName := keyDir.Arguments[0].Value.Raw

	// initialize the struct to return
	entityReprs := &EntityRepresentations{
		TypeDefn: &astType{
			typ:             &ast.Type{NamedType: typename},
			inSchema:        q.op.inSchema,
			dgraphPredicate: q.op.inSchema.dgraphPredicate,
		},
		KeyVals:                make([]interface{}, 0, len(representations)),
		KeyValToRepresentation: make(map[string]map[string]interface{}),
	}
	entityReprs.KeyField = entityReprs.TypeDefn.Field(keyFldName)

	// iterate over all the representations and parse
	for i, rep := range representations {
		representation, ok = rep.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("error parsing %dth item in the `_representations` argument", i)
		}

		typename, ok = representation[Typename].(string)
		if !ok {
			return nil, fmt.Errorf("unable to extract __typename from %dth item in the"+
				" `_representations` argument", i)
		}
		if typename != entityReprs.TypeDefn.Name() {
			return nil, fmt.Errorf("expected only one unique typename in `_representations`"+
				" argument, got: [%s, %s]", entityReprs.TypeDefn.Name(), typename)
		}

		keyVal, ok := representation[keyFldName]
		if !ok {
			return nil, fmt.Errorf("unable to extract value for key field `%s` from %dth item in"+
				" the `_representations` argument", keyFldName, i)
		}
		entityReprs.KeyVals = append(entityReprs.KeyVals, keyVal)
		entityReprs.KeyValToRepresentation[fmt.Sprint(keyVal)] = representation
	}

	return entityReprs, nil
}

func (q *query) AuthFor(jwtVars map[string]interface{}) Query {
	// copy the template, so that multiple queries can run rewriting for the rule.
	return &query{
		field: (*field)(q).field,
		op: &operation{op: q.op.op,
			query:    q.op.query,
			doc:      q.op.doc,
			inSchema: q.op.inSchema,
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

func (q *query) RemoteResponseName() string {
	return (*field)(q).RemoteResponseName()
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

func (q *query) SkipField(dgraphTypes []string, seenField map[string]bool) bool {
	return (*field)(q).SkipField(dgraphTypes, seenField)
}

func (q *query) Cascade() []string {
	return (*field)(q).Cascade()
}

func (q *query) ApolloRequiredFields() []string {
	return (*field)(q).ApolloRequiredFields()
}

func (q *query) CustomRequiredFields() map[string]FieldDefinition {
	return (*field)(q).CustomRequiredFields()
}

func (q *query) IsCustomHTTP() bool {
	return (*field)(q).IsCustomHTTP()
}

func (q *query) HasCustomHTTPChild() bool {
	return (*field)(q).HasCustomHTTPChild()
}

func (q *query) HasLambdaDirective() bool {
	return (*field)(q).HasLambdaDirective()
}

func (q *query) IDArgValue() (map[string]string, uint64, error) {
	return (*field)(q).IDArgValue()
}

func (q *query) XIDArgs() map[string]string {
	return (*field)(q).XIDArgs()
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

func (q *query) CustomHTTPConfig() (*FieldHTTPConfig, error) {
	return getCustomHTTPConfig((*field)(q), true)
}

func (q *query) EnumValues() []string {
	return nil
}

func (m *mutation) ConstructedFor() Type {
	return (*field)(m).ConstructedFor()
}

// In case the field f is of type <Type>Aggregate, the Type is retunred.
// In all other case the function returns the type of field f.
func (f *field) ConstructedFor() Type {
	if !f.IsAggregateField() {
		return f.Type()
	}

	// f has type of the form <SomeTypeName>AggregateResult
	fieldName := f.Type().Name()
	typeName := fieldName[:len(fieldName)-15]
	return &astType{
		typ: &ast.Type{
			NamedType: typeName,
		},
		inSchema:        f.op.inSchema,
		dgraphPredicate: f.op.inSchema.dgraphPredicate,
	}
}

func (q *query) ConstructedFor() Type {
	if q.QueryType() != AggregateQuery {
		return q.Type()
	}
	// Its of type AggregateQuery
	fieldName := q.Type().Name()
	typeName := fieldName[:len(fieldName)-15]
	return &astType{
		typ: &ast.Type{
			NamedType: typeName,
		},
		inSchema:        q.op.inSchema,
		dgraphPredicate: q.op.inSchema.dgraphPredicate,
	}
}

func (m *mutation) ConstructedForDgraphPredicate() string {
	return (*field)(m).ConstructedForDgraphPredicate()
}

func (q *query) ConstructedForDgraphPredicate() string {
	return (*field)(q).ConstructedForDgraphPredicate()
}

// In case, the field f is of type <Type>Aggregate it returns dgraph predicate of the Type.
// In all other cases it returns dgraph predicate of the field.
func (f *field) ConstructedForDgraphPredicate() string {
	if !f.IsAggregateField() {
		return f.DgraphPredicate()
	}
	// Remove last 9 characters of the field name.
	// Eg. to get "FieldName" from "FieldNameAggregate"
	fldName := f.Name()
	return f.op.inSchema.dgraphPredicate[f.field.ObjectDefinition.Name][fldName[:len(fldName)-9]]
}

func (m *mutation) DgraphPredicateForAggregateField() string {
	return (*field)(m).DgraphPredicateForAggregateField()
}

func (q *query) DgraphPredicateForAggregateField() string {
	return (*field)(q).DgraphPredicateForAggregateField()
}

// In case, the field f is of name <Name>Max / <Name>Min / <Name>Sum / <Name>Avg ,
// it returns corresponding dgraph predicate name.
// In all other cases it returns dgraph predicate of the field.
func (f *field) DgraphPredicateForAggregateField() string {
	aggregateFunctions := []string{"Max", "Min", "Sum", "Avg"}

	fldName := f.Name()
	var isAggregateFunction bool = false
	for _, function := range aggregateFunctions {
		if strings.HasSuffix(fldName, function) {
			isAggregateFunction = true
		}
	}
	if !isAggregateFunction {
		return f.DgraphPredicate()
	}
	// aggregateResultTypeName contains name of the type in which the aggregate field is defined,
	// it will be of the form <Type>AggregateResult
	// we need to obtain the type name from <Type> from <Type>AggregateResult
	aggregateResultTypeName := f.field.ObjectDefinition.Name

	// If aggregateResultTypeName is found to not end with AggregateResult, just return DgraphPredicate()
	if !strings.HasSuffix(aggregateResultTypeName, "AggregateResult") {
		// This is an extra precaution and ideally, the code should not reach over here.
		return f.DgraphPredicate()
	}
	mainTypeName := aggregateResultTypeName[:len(aggregateResultTypeName)-15]
	// Remove last 3 characters of the field name.
	// Eg. to get "FieldName" from "FieldNameMax"
	// As all Aggregate functions are of length 3, removing last 3 characters from fldName
	return f.op.inSchema.dgraphPredicate[mainTypeName][fldName[:len(fldName)-3]]
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
	case name == "_entities":
		return EntitiesQuery
	case strings.HasPrefix(name, "get"):
		return GetQuery
	case name == "__schema" || name == "__type" || name == "__typename":
		return SchemaQuery
	case strings.HasPrefix(name, "query"):
		return FilterQuery
	case strings.HasPrefix(name, "check"):
		return PasswordQuery
	case strings.HasPrefix(name, "aggregate"):
		return AggregateQuery
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

func (q *query) AbstractType() bool {
	return (*field)(q).AbstractType()
}

func (q *query) TypeName(dgraphTypes []string) string {
	return (*field)(q).TypeName(dgraphTypes)
}

func (q *query) IncludeAbstractField(dgraphTypes []string) bool {
	return (*field)(q).IncludeAbstractField(dgraphTypes)
}

func (m *mutation) Name() string {
	return (*field)(m).Name()
}

func (m *mutation) RemoteResponseName() string {
	return (*field)(m).RemoteResponseName()
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

func (m *mutation) SkipField(dgraphTypes []string, seenField map[string]bool) bool {
	return (*field)(m).SkipField(dgraphTypes, seenField)
}

func (m *mutation) Cascade() []string {
	return (*field)(m).Cascade()
}

func (m *mutation) ApolloRequiredFields() []string {
	return (*field)(m).ApolloRequiredFields()
}

func (m *mutation) CustomRequiredFields() map[string]FieldDefinition {
	return (*field)(m).CustomRequiredFields()
}

func (m *mutation) IsCustomHTTP() bool {
	return (*field)(m).IsCustomHTTP()
}

func (m *mutation) HasCustomHTTPChild() bool {
	return (*field)(m).HasCustomHTTPChild()
}

func (m *mutation) HasLambdaDirective() bool {
	return (*field)(m).HasLambdaDirective()
}

func (m *mutation) Type() Type {
	return (*field)(m).Type()
}

func (m *mutation) AbstractType() bool {
	return (*field)(m).AbstractType()
}

func (m *mutation) XIDArgs() map[string]string {
	return (*field)(m).XIDArgs()
}

func (m *mutation) IDArgValue() (map[string]string, uint64, error) {
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
			field.Directives = append(
				field.Directives,
				&ast.Directive{Name: cascadeDirective, Definition: m.op.inSchema.schema.Directives[cascadeDirective]},
			)
		}
		return f
	}
	return nil
}

func (m *mutation) NumUidsField() Field {
	for _, f := range m.SelectionSet() {
		if f.Name() == NumUid {
			return f
		}
	}
	return nil
}

func (m *mutation) HasLambdaOnMutate() bool {
	return m.op.inSchema.lambdaOnMutate[m.Name()]
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

func (m *mutation) CustomHTTPConfig() (*FieldHTTPConfig, error) {
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

func (m *mutation) TypeName(dgraphTypes []string) string {
	return (*field)(m).TypeName(dgraphTypes)
}

func (m *mutation) IncludeAbstractField(dgraphTypes []string) bool {
	return (*field)(m).IncludeAbstractField(dgraphTypes)
}

func (m *mutation) IsAuthQuery() bool {
	return (*field)(m).field.Arguments.ForName("dgraph.uid") != nil
}

func (m *mutation) IsAggregateField() bool {
	return (*field)(m).IsAggregateField()
}

func (m *mutation) GqlErrorf(path []interface{}, message string, args ...interface{}) *x.GqlError {
	return (*field)(m).GqlErrorf(path, message, args...)
}

func (m *mutation) MaxPathLength() int {
	return (*field)(m).MaxPathLength()
}

func (m *mutation) PreAllocatePathSlice() []interface{} {
	return (*field)(m).PreAllocatePathSlice()
}

func (m *mutation) NullValue() []byte {
	return (*field)(m).NullValue()
}

func (m *mutation) NullResponse() []byte {
	return (*field)(m).NullResponse()
}

func (m *mutation) CompleteAlias(buf *bytes.Buffer) {
	(*field)(m).CompleteAlias(buf)
}

func (m *mutation) GetAuthMeta() *authorization.AuthMeta {
	return (*field)(m).GetAuthMeta()
}

func (t *astType) AuthRules() *TypeAuth {
	return t.inSchema.authRules[t.DgraphName()]
}

func (t *astType) IsGeo() bool {
	return t.Name() == "Point" || t.Name() == "Polygon" || t.Name() == "MultiPolygon"
}

func (t *astType) IsAggregateResult() bool {
	return strings.HasSuffix(t.Name(), "AggregateResult")
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
	return fd.parentType.Name() + "." + fd.fieldDef.Name
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

func (fd *fieldDefinition) WithMemberType(memberType string) FieldDefinition {
	// just need to return a copy of this fieldDefinition with type set to memberType
	return &fieldDefinition{
		fieldDef: &ast.FieldDefinition{
			Name:         fd.fieldDef.Name,
			Arguments:    fd.fieldDef.Arguments,
			DefaultValue: fd.fieldDef.DefaultValue,
			Type:         &ast.Type{NamedType: memberType},
			Directives:   fd.fieldDef.Directives,
			Position:     fd.fieldDef.Position,
		},
		inSchema:        fd.inSchema,
		dgraphPredicate: fd.dgraphPredicate,
		parentType:      fd.parentType,
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

func (fd *fieldDefinition) GetAuthMeta() *authorization.AuthMeta {
	return fd.inSchema.meta.authMeta
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

func (t *astType) IsInterface() bool {
	return t.inSchema.schema.Types[t.typ.Name()].Kind == ast.Interface
}

func (t *astType) IsUnion() bool {
	return t.inSchema.schema.Types[t.typ.Name()].Kind == ast.Union
}

func (t *astType) UnionMembers(memberTypesList []interface{}) []Type {
	var memberTypes []Type
	if (memberTypesList) == nil {
		// if no specific members were requested, find out all the members of this union
		for _, typName := range t.inSchema.schema.Types[t.typ.Name()].Types {
			memberTypes = append(memberTypes, &astType{
				typ:             &ast.Type{NamedType: typName},
				inSchema:        t.inSchema,
				dgraphPredicate: t.dgraphPredicate,
			})
		}
	} else {
		// else return wrapper for only the members which were requested
		for _, typName := range memberTypesList {
			memberTypes = append(memberTypes, &astType{
				typ:             &ast.Type{NamedType: typName.(string)},
				inSchema:        t.inSchema,
				dgraphPredicate: t.dgraphPredicate,
			})
		}
	}
	return memberTypes
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
	// If the field is of ID type but it is an external field,
	// then it is stored in Dgraph as string type with Hash index.
	// So the this field is actually not stored as ID type.
	if (def.Kind != ast.Object && def.Kind != ast.Interface) || hasExtends(def) {
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

func (t *astType) XIDFields() []FieldDefinition {
	def := t.inSchema.schema.Types[t.Name()]
	if def.Kind != ast.Object && def.Kind != ast.Interface {
		return nil
	}

	// If field is of ID type but it is an external field,
	// then it is stored in Dgraph as string type with Hash index.
	// So it should be returned as an XID Field.
	var xids []FieldDefinition
	for _, fd := range def.Fields {
		if hasIDDirective(fd) || (hasExternal(fd) && isID(fd)) {
			xids = append(xids, &fieldDefinition{
				fieldDef:   fd,
				inSchema:   t.inSchema,
				parentType: t,
			})
		}
	}
	// XIDs are sorted by name to ensure consistency.
	sort.Slice(xids, func(i, j int) bool { return xids[i].Name() < xids[j].Name() })
	return xids
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

func (t *astType) ImplementingTypes() []Type {
	objects := t.inSchema.schema.PossibleTypes[t.typ.Name()]
	if len(objects) == 0 {
		return nil
	}
	types := make([]Type, 0, len(objects))
	for _, typ := range objects {
		types = append(types, &astType{
			typ:             &ast.Type{NamedType: typ.Name},
			inSchema:        t.inSchema,
			dgraphPredicate: t.dgraphPredicate,
		})
	}
	return types
}

// CheckNonNulls checks that any non nullables in t are present in obj.
// Fields of type ID are not checked, nor is any exclusion.
//
// For our reference types for adding/linking objects, we'd like to have something like
//
//	input PostRef {
//		id: ID!
//	}
//
//	input PostNew {
//		title: String!
//		text: String
//		author: AuthorRef!
//	}
//
// and then have something like this
//
// input PostNewOrReference = PostRef | PostNew
//
//	input AuthorNew {
//	  ...
//	  posts: [PostNewOrReference]
//	}
//
// but GraphQL doesn't allow union types in input, so best we can do is
//
//	input PostRef {
//		id: ID
//		title: String
//		text: String
//		author: AuthorRef
//	}
//
// and then check ourselves that either there's an ID, or there's all the bits to
// satisfy a valid post.
func (t *astType) EnsureNonNulls(obj map[string]interface{}, exclusion string) error {
	for _, fld := range t.inSchema.schema.Types[t.Name()].Fields {
		if fld.Type.NonNull && !isID(fld) && fld.Name != exclusion &&
			t.inSchema.customDirectives[t.Name()][fld.Name] == nil {
			if val, ok := obj[fld.Name]; !ok || val == nil {
				return errors.Errorf(
					"type %s requires a value for field %s, but no value present",
					t.Name(), fld.Name)
			}
		}
	}
	return nil
}

func getAsPathParamValue(val interface{}) string {
	switch v := val.(type) {
	case json.RawMessage:
		var temp interface{}
		_ = Unmarshal(v, &temp) // this can't error, as it was marshalled earlier
		return getAsPathParamValue(temp)
	case string:
		return v
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
	case json.RawMessage:
		var temp interface{}
		_ = Unmarshal(v, &temp) // this can't error, as it was marshalled earlier
		setQueryParamValue(queryParams, key, temp)
	case string:
		queryParams.Add(key, v)
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
func parseBodyTemplate(body string, strictJSON bool) (interface{}, map[string]bool, error) {
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

	return jsonTemplate, requiredVariables, nil
}

func isVar(key string) bool {
	return strings.HasPrefix(key, "$")
}

func substituteVarInMapInBody(object, variables map[string]interface{}) map[string]interface{} {
	objCopy := make(map[string]interface{}, len(object))
	for k, v := range object {
		switch val := v.(type) {
		case string:
			if isVar(val) {
				if vval, ok := variables[val[1:]]; ok {
					objCopy[k] = vval
				}
			} else {
				objCopy[k] = val
			}
		case map[string]interface{}:
			objCopy[k] = substituteVarInMapInBody(val, variables)
		case []interface{}:
			objCopy[k] = substituteVarInSliceInBody(val, variables)
		default:
			objCopy[k] = val
		}
	}
	return objCopy
}

func substituteVarInSliceInBody(slice []interface{}, variables map[string]interface{}) []interface{} {
	sliceCopy := make([]interface{}, len(slice))
	for k, v := range slice {
		switch val := v.(type) {
		case string:
			if isVar(val) {
				sliceCopy[k] = variables[val[1:]]
			} else {
				sliceCopy[k] = val
			}
		case map[string]interface{}:
			sliceCopy[k] = substituteVarInMapInBody(val, variables)
		case []interface{}:
			sliceCopy[k] = substituteVarInSliceInBody(val, variables)
		default:
			sliceCopy[k] = val
		}
	}
	return sliceCopy
}

// Given a JSON representation for a body with variables defined, this function substitutes
// the variables and returns the final JSON.
// for e.g.
//
//	{
//			"author" : "$id",
//			"name" : "Jerry",
//			"age" : 23,
//			"post": {
//				"id": "$postID"
//			}
//	}
//
// with variables {"id": "0x3",	postID: "0x9"}
// should return
//
//	{
//			"author" : "0x3",
//			"name" : "Jerry",
//			"age" : 23,
//			"post": {
//				"id": "0x9"
//			}
//	}
func SubstituteVarsInBody(jsonTemplate interface{}, variables map[string]interface{}) interface{} {
	if jsonTemplate == nil {
		return nil
	}

	switch val := jsonTemplate.(type) {
	case string:
		if isVar(val) {
			return variables[val[1:]]
		}
	case map[string]interface{}:
		return substituteVarInMapInBody(val, variables)
	case []interface{}:
		return substituteVarInSliceInBody(val, variables)
	}

	// this must be a hard-coded scalar, so just return as it is
	return jsonTemplate
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
//
//	Hello{
//		name {
//			age
//		}
//		friend
//	}
//
// will return
//
//	{
//		name {
//			age
//		}
//		friend
//	}
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
