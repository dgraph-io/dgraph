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
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"text/scanner"

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
	NotSupportedQuery    QueryType    = "notsupported"
	AddMutation          MutationType = "add"
	UpdateMutation       MutationType = "update"
	DeleteMutation       MutationType = "delete"
	HTTPMutation         MutationType = "http"
	NotSupportedMutation MutationType = "notsupported"
	IDType                            = "ID"
	IDArgName                         = "id"
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
	ResponseName() string
	ArgValue(name string) interface{}
	IsArgListType(name string) bool
	IDArgValue() (*string, uint64, error)
	XIDArg() string
	SetArgTo(arg string, val interface{})
	Skip() bool
	Include() bool
	Cascade() []string
	HasCustomDirective() (bool, map[string]bool)
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
	fmt.Stringer
}

// A FieldDefinition is a field as defined in some Type in the schema.  As opposed
// to a Field, which is an instance of a query or mutation asking for a field
// (which in turn must have a FieldDefinition of the right type in the schema.)
type FieldDefinition interface {
	Name() string
	Type() Type
	IsID() bool
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
	// customDirectives stores the mapping of typeName -> fieldName -> @custom definition.
	// It is read-only.
	// The outer map will contain typeName key only if one of the fields on that type has @custom.
	// The inner map will contain fieldName key only if that field has @custom.
	// It is pre-computed so that runtime queries and mutations can look it up quickly, and not do
	// something like field.Directives.ForName("custom"), which results in iterating over all the
	// directives of the field.
	customDirectives map[string]map[string]*ast.Directive
	// Map from typename to auth rules
	authRules map[string]*TypeAuth
}

type operation struct {
	op     *ast.OperationDefinition
	vars   map[string]interface{}
	header http.Header

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
		// of the schema hence we ignore BuiltIn, query and mutation types.
		if inputTyp.BuiltIn || isQueryOrMutationType(inputTyp) || inputTyp.Name == "Subscription" ||
			(inputTyp.Kind != ast.Object && inputTyp.Kind != ast.Interface) {
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
		// for UpdateTPayload and get the type from the first field. There is no direct way to get
		// the type from the definition of an object. We use Update and not Add here because
		// Interfaces only have Update.
		var def *ast.Definition
		if def = s.schema.Types["Update"+mutatedTypeName+"Payload"]; def == nil {
			def = s.schema.Types["Add"+mutatedTypeName+"Payload"]
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

func customMappings(s *ast.Schema) map[string]map[string]*ast.Directive {
	customDirectives := make(map[string]map[string]*ast.Directive)

	for _, typ := range s.Types {
		for _, field := range typ.Fields {
			for i, dir := range field.Directives {
				if dir.Name == customDirective {
					// remove custom directive from s
					lastIndex := len(field.Directives) - 1
					field.Directives[i] = field.Directives[lastIndex]
					field.Directives = field.Directives[:lastIndex]
					// now put it into mapping
					var fieldMap map[string]*ast.Directive
					if innerMap, ok := customDirectives[typ.Name]; !ok {
						fieldMap = make(map[string]*ast.Directive)
					} else {
						fieldMap = innerMap
					}
					fieldMap[field.Name] = dir
					customDirectives[typ.Name] = fieldMap
					// break, as there can only be one @custom
					break
				}
			}
		}
	}

	return customDirectives
}

// AsSchema wraps a github.com/vektah/gqlparser/ast.Schema.
func AsSchema(s *ast.Schema) (Schema, error) {
	// Auth rules can't be effectively validated as part of the normal rules -
	// because they need the fully generated schema to be checked against.
	authRules, err := authRules(s)
	if err != nil {
		return nil, err
	}

	dgraphPredicate := dgraphMapping(s)
	sch := &schema{
		schema:           s,
		dgraphPredicate:  dgraphPredicate,
		typeNameAst:      typeMappings(s),
		customDirectives: customMappings(s),
		authRules:        authRules,
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

func (f *field) ArgValue(name string) interface{} {
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
	return f.arguments[name]
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

	if f.field.Directives.ForName(cascadeDirective) == nil {
		return nil
	}
	return []string{"__all__"}
}

func (f *field) HasCustomDirective() (bool, map[string]bool) {
	custom := f.op.inSchema.customDirectives[f.GetObjectName()][f.Name()]
	if custom == nil {
		return false, nil
	}

	var rf map[string]bool
	httpArg := custom.Arguments.ForName("http")

	bodyArg := httpArg.Value.Children.ForName("body")
	if bodyArg != nil {
		bodyTemplate := bodyArg.Raw
		_, rf, _ = parseBodyTemplate(bodyTemplate)
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

	graphqlArg := httpArg.Value.Children.ForName("graphql")
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

func (f *field) XIDArg() string {
	xidArgName := ""
	passwordField := f.Type().PasswordField()
	for _, arg := range f.field.Arguments {
		if arg.Name != IDArgName && (passwordField == nil ||
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
		bt, rf, err := parseBodyTemplate(bodyTemplate)
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
		if fconf.Template != nil {
			if err = SubstituteVarsInBody(fconf.Template, bodyVars); err != nil {
				return fconf, errors.Wrapf(err, "while substituting vars in Body")
			}
		}
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
	// As ID maps to uid in dgraph, so it is not stored as an edge, hence does not appear in
	// f.op.inSchema.dgraphPredicate map. So, always include the queried field if it is of ID type.
	if f.Type().Name() == IDType {
		return true
	}
	// Given a list of dgraph types, we query the schema and find the one which is an ast.Object
	// and not an Interface object.
	for _, typ := range dgraphTypes {
		styp, ok := typ.(string)
		if !ok {
			continue
		}
		for _, origTyp := range f.op.inSchema.typeNameAst[styp] {
			if origTyp.Kind == ast.Object {
				// If the field doesn't exist in the map corresponding to the object type, then we
				// don't need to include it.
				_, ok := f.op.inSchema.dgraphPredicate[origTyp.Name][f.Name()]
				return ok || f.Name() == Typename
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

func (q *query) SetArgTo(arg string, val interface{}) {
	(*field)(q).SetArgTo(arg, val)
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

func queryType(name string, custom *ast.Directive) QueryType {
	switch {
	case custom != nil:
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

func (m *mutation) SetArgTo(arg string, val interface{}) {
	(*field)(m).SetArgTo(arg, val)
}

func (m *mutation) IsArgListType(name string) bool {
	return (*field)(m).IsArgListType(name)
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

func (t *astType) Field(name string) FieldDefinition {
	return &fieldDefinition{
		// this ForName lookup is a loop in the underlying schema :-(
		fieldDef:        t.inSchema.schema.Types[t.Name()].Fields.ForName(name),
		inSchema:        t.inSchema,
		dgraphPredicate: t.dgraphPredicate,
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
			})
	}

	return result
}

func (fd *fieldDefinition) Name() string {
	return fd.fieldDef.Name
}

func (fd *fieldDefinition) IsID() bool {
	return isID(fd.fieldDef)
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

func (fd *fieldDefinition) Inverse() FieldDefinition {

	invDirective := fd.fieldDef.Directives.ForName(inverseDirective)
	if invDirective == nil {
		return nil
	}

	invFieldArg := invDirective.Arguments.ForName(inverseArg)
	if invFieldArg == nil {
		return nil // really not possible
	}

	// typ must exist if the schema passed GQL validation
	typ := fd.inSchema.schema.Types[fd.Type().Name()]

	// fld must exist if the schema passed our validation
	fld := typ.Fields.ForName(invFieldArg.Value.Raw)

	return &fieldDefinition{
		fieldDef:        fld,
		inSchema:        fd.inSchema,
		dgraphPredicate: fd.dgraphPredicate}
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
	// typ must exist if the schema passed GQL validation
	typ := fd.inSchema.schema.Types[fd.Type().Name()]

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
		dgraphPredicate: fd.dgraphPredicate}
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
				fieldDef: fd,
				inSchema: t.inSchema,
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
		fieldDef: fd,
		inSchema: t.inSchema,
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
				fieldDef: fd,
				inSchema: t.inSchema,
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
		if fld.Type.NonNull && !isID(fld) && !(fld.Name == exclusion) {
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

func isName(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			continue
		case r >= 'A' && r <= 'Z':
			continue
		default:
			return false
		}
	}
	return true
}

// Given a template for a body with variables defined, this function parses the body
// and converts it into a JSON representation and returns that. It also returns a list of the field
// names that are required by this template.
// for e.g.
// { author: $id, post: { id: $postID }}
// would return
// { "author" : "$id", "post": { "id": "$postID" }} and { "id": true, "postID": true}
// If the final result is not a valid JSON, then an error is returned.
func parseBodyTemplate(body string) (*interface{}, map[string]bool, error) {
	var s scanner.Scanner
	s.Init(strings.NewReader(body))

	result := new(bytes.Buffer)
	parsingVariable := false
	depth := 0
	requiredFields := make(map[string]bool)
	for tok := s.Scan(); tok != scanner.EOF; tok = s.Scan() {
		text := s.TokenText()
		switch {
		case text == "{":
			result.WriteString(text)
			depth++
		case text == "}":
			depth--
			result.WriteString(text)
		case text == ":" || text == "," || text == "[" || text == "]":
			result.WriteString(text)
		case text == "$":
			parsingVariable = true
		case isName(text):
			// Name could either be a key or be part of a variable after dollar.
			if !parsingVariable {
				result.WriteString(fmt.Sprintf(`"%s"`, text))
				continue
			}
			requiredFields[text] = true
			variable := "$" + text
			fmt.Fprintf(result, `"%s"`, variable)
			parsingVariable = false

		default:
			return nil, nil, errors.Errorf("invalid character: %s while parsing body template",
				text)
		}
	}
	if depth != 0 {
		return nil, nil, errors.New("found unmatched curly braces while parsing body template")
	}

	if result.Len() == 0 {
		return nil, nil, nil
	}

	var m interface{}
	if err := json.Unmarshal(result.Bytes(), &m); err != nil {
		return nil, nil, errors.Errorf("couldn't unmarshal HTTP body: %s as JSON", result.Bytes())
	}
	return &m, requiredFields, nil
}

func getVar(key string, variables map[string]interface{}) (interface{}, error) {
	if !strings.HasPrefix(key, "$") {
		return nil, errors.Errorf("expected a variable to start with $. Found: %s", key)
	}
	val, ok := variables[key[1:]]
	if !ok {
		return nil, errors.Errorf("couldn't find variable: %s in variables map", key)
	}

	return val, nil
}

func substituteSingleVarInBody(key string, valPtr *interface{},
	variables map[string]interface{}) error {
	// Look it up in the map and replace.
	val, err := getVar(key, variables)
	if err != nil {
		return err
	}
	*valPtr = val
	return nil
}

func substituteVarInMapInBody(object, variables map[string]interface{}) error {
	for k, v := range object {
		switch val := v.(type) {
		case string:
			vval, err := getVar(val, variables)
			if err != nil {
				return err
			}
			object[k] = vval
		case map[string]interface{}:
			if err := substituteVarInMapInBody(val, variables); err != nil {
				return err
			}
		case []interface{}:
			if err := substituteVarInSliceInBody(val, variables); err != nil {
				return err
			}
		default:
			return errors.Errorf("got unexpected type value in map: %+v", v)
		}
	}
	return nil
}

func substituteVarInSliceInBody(slice []interface{}, variables map[string]interface{}) error {
	for k, v := range slice {
		switch val := v.(type) {
		case string:
			vval, err := getVar(val, variables)
			if err != nil {
				return err
			}
			slice[k] = vval
		case map[string]interface{}:
			if err := substituteVarInMapInBody(val, variables); err != nil {
				return err
			}
		case []interface{}:
			if err := substituteVarInSliceInBody(val, variables); err != nil {
				return err
			}
		default:
			return errors.Errorf("got unexpected type value in array: %+v", v)
		}
	}
	return nil
}

// Given a JSON representation for a body with variables defined, this function substitutes
// the variables and returns the final JSON.
// for e.g.
// { "author" : "$id", "post": { "id": "$postID" }} with variables {"id": "0x3", postID: "0x9"}
// should return { "author" : "0x3", "post": { "id": "0x9" }}
func SubstituteVarsInBody(jsonTemplate *interface{}, variables map[string]interface{}) error {
	if jsonTemplate == nil {
		return nil
	}

	switch val := (*jsonTemplate).(type) {
	case string:
		return substituteSingleVarInBody(val, jsonTemplate, variables)
	case map[string]interface{}:
		return substituteVarInMapInBody(val, variables)
	case []interface{}:
		return substituteVarInSliceInBody(val, variables)
	default:
		return errors.Errorf("got unexpected type value in jsonTemplate: %+v", val)
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
	_, rf, err := parseBodyTemplate("{" + args + "}")
	return rf, err
}
