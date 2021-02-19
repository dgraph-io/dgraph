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
	"sort"
	"strings"

	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/gqlparser/v2/ast"
	"github.com/dgraph-io/gqlparser/v2/gqlerror"
	"github.com/dgraph-io/gqlparser/v2/parser"
)

const (
	inverseDirective = "hasInverse"
	inverseArg       = "field"

	searchDirective = "search"
	searchArgs      = "by"

	dgraphDirective = "dgraph"
	dgraphTypeArg   = "type"
	dgraphPredArg   = "pred"

	idDirective           = "id"
	subscriptionDirective = "withSubscription"
	secretDirective       = "secret"
	authDirective         = "auth"
	customDirective       = "custom"
	remoteDirective       = "remote" // types with this directive are not stored in Dgraph.
	lambdaDirective       = "lambda"

	generateDirective       = "generate"
	generateQueryArg        = "query"
	generateGetField        = "get"
	generateQueryField      = "query"
	generatePasswordField   = "password"
	generateAggregateField  = "aggregate"
	generateMutationArg     = "mutation"
	generateAddField        = "add"
	generateUpdateField     = "update"
	generateDeleteField     = "delete"
	generateSubscriptionArg = "subscription"

	cascadeDirective = "cascade"
	cascadeArg       = "fields"

	cacheControlDirective = "cacheControl"
	CacheControlHeader    = "Cache-Control"

	// Directives to support Apollo Federation
	apolloKeyDirective      = "key"
	apolloKeyArg            = "fields"
	apolloExternalDirective = "external"
	apolloExtendsDirective  = "extends"

	// custom directive args and fields
	dqlArg      = "dql"
	httpArg     = "http"
	httpUrl     = "url"
	httpMethod  = "method"
	httpBody    = "body"
	httpGraphql = "graphql"
	mode        = "mode"
	BATCH       = "BATCH"
	SINGLE      = "SINGLE"

	// geo type names and fields
	Point        = "Point"
	Polygon      = "Polygon"
	MultiPolygon = "MultiPolygon"
	Latitude     = "latitude"
	Longitude    = "longitude"
	Points       = "points"
	Coordinates  = "coordinates"
	Polygons     = "polygons"

	deprecatedDirective = "deprecated"
	NumUid              = "numUids"
	Msg                 = "msg"

	Typename = "__typename"

	// schemaExtras is everything that gets added to an input schema to make it
	// GraphQL valid and for the completion algorithm to use to build in search
	// capability into the schema.

	// Just remove directive definitions and not the input types
	schemaInputs = `
"""
The Int64 scalar type represents a signed 64‐bit numeric non‐fractional value.
Int64 can represent values in range [-(2^63),(2^63 - 1)].
"""
scalar Int64

"""
The DateTime scalar type represents date and time as a string in RFC3339 format.
For example: "1985-04-12T23:20:50.52Z" represents 20 minutes and 50.52 seconds after the 23rd hour of April 12th, 1985 in UTC.
"""
scalar DateTime

input IntRange{
	min: Int!
	max: Int!
}

input FloatRange{
	min: Float!
	max: Float!
}

input Int64Range{
	min: Int64!
	max: Int64!
}

input DateTimeRange{
	min: DateTime!
	max: DateTime!
}

input StringRange{
	min: String!
	max: String!
}

enum DgraphIndex {
	int
	int64
	float
	bool
	hash
	exact
	term
	fulltext
	trigram
	regexp
	year
	month
	day
	hour
	geo
}

input AuthRule {
	and: [AuthRule]
	or: [AuthRule]
	not: AuthRule
	rule: String
}

enum HTTPMethod {
	GET
	POST
	PUT
	PATCH
	DELETE
}

enum Mode {
	BATCH
	SINGLE
}

input CustomHTTP {
	url: String!
	method: HTTPMethod!
	body: String
	graphql: String
	mode: Mode
	forwardHeaders: [String!]
	secretHeaders: [String!]
	introspectionHeaders: [String!]
	skipIntrospection: Boolean
}

type Point {
	longitude: Float!
	latitude: Float!
}

input PointRef {
	longitude: Float!
	latitude: Float!
}

input NearFilter {
	distance: Float!
	coordinate: PointRef!
}

input PointGeoFilter {
	near: NearFilter
	within: WithinFilter
}

type PointList {
	points: [Point!]!
}

input PointListRef {
	points: [PointRef!]!
}

type Polygon {
	coordinates: [PointList!]!
}

input PolygonRef {
	coordinates: [PointListRef!]!
}

type MultiPolygon {
	polygons: [Polygon!]!
}

input MultiPolygonRef {
	polygons: [PolygonRef!]!
}

input WithinFilter {
	polygon: PolygonRef!
}

input ContainsFilter {
	point: PointRef
	polygon: PolygonRef
}

input IntersectsFilter {
	polygon: PolygonRef
	multiPolygon: MultiPolygonRef
}

input PolygonGeoFilter {
	near: NearFilter
	within: WithinFilter
	contains: ContainsFilter
	intersects: IntersectsFilter
}

input GenerateQueryParams {
	get: Boolean
	query: Boolean
	password: Boolean
	aggregate: Boolean
}

input GenerateMutationParams {
	add: Boolean
	update: Boolean
	delete: Boolean
}
`
	directiveDefs = `
directive @hasInverse(field: String!) on FIELD_DEFINITION
directive @search(by: [DgraphIndex!]) on FIELD_DEFINITION
directive @dgraph(type: String, pred: String) on OBJECT | INTERFACE | FIELD_DEFINITION
directive @id on FIELD_DEFINITION
directive @withSubscription on OBJECT | INTERFACE | FIELD_DEFINITION
directive @secret(field: String!, pred: String) on OBJECT | INTERFACE
directive @auth(
	password: AuthRule
	query: AuthRule,
	add: AuthRule,
	update: AuthRule,
	delete: AuthRule) on OBJECT | INTERFACE
directive @custom(http: CustomHTTP, dql: String) on FIELD_DEFINITION
directive @remote on OBJECT | INTERFACE | UNION | INPUT_OBJECT | ENUM
directive @cascade(fields: [String]) on FIELD
directive @lambda on FIELD_DEFINITION
directive @cacheControl(maxAge: Int!) on QUERY
directive @generate(
	query: GenerateQueryParams,
	mutation: GenerateMutationParams,
	subscription: Boolean) on OBJECT | INTERFACE
`

	apolloSupportedDirectiveDefs = `
directive @hasInverse(field: String!) on FIELD_DEFINITION
directive @search(by: [DgraphIndex!]) on FIELD_DEFINITION
directive @dgraph(type: String, pred: String) on OBJECT | INTERFACE | FIELD_DEFINITION
directive @id on FIELD_DEFINITION
directive @withSubscription on OBJECT | INTERFACE | FIELD_DEFINITION
directive @secret(field: String!, pred: String) on OBJECT | INTERFACE
directive @remote on OBJECT | INTERFACE | UNION | INPUT_OBJECT | ENUM
directive @cascade(fields: [String]) on FIELD
directive @lambda on FIELD_DEFINITION
directive @cacheControl(maxAge: Int!) on QUERY
`
	filterInputs = `
input IntFilter {
	eq: Int
	in: [Int]
	le: Int
	lt: Int
	ge: Int
	gt: Int
	between: IntRange
}

input Int64Filter {
	eq: Int64
	in: [Int64]
	le: Int64
	lt: Int64
	ge: Int64
	gt: Int64
	between: Int64Range
}

input FloatFilter {
	eq: Float
	in: [Float]
	le: Float
	lt: Float
	ge: Float
	gt: Float
	between: FloatRange
}

input DateTimeFilter {
	eq: DateTime
	in: [DateTime]
	le: DateTime
	lt: DateTime
	ge: DateTime
	gt: DateTime
	between: DateTimeRange
}

input StringTermFilter {
	allofterms: String
	anyofterms: String
}

input StringRegExpFilter {
	regexp: String
}

input StringFullTextFilter {
	alloftext: String
	anyoftext: String
}

input StringExactFilter {
	eq: String
	in: [String]
	le: String
	lt: String
	ge: String
	gt: String
	between: StringRange
}

input StringHashFilter {
	eq: String
	in: [String]
}
`

	apolloSchemaExtras = `
scalar _Any
scalar _FieldSet

type _Service {
	sdl: String
}

directive @external on FIELD_DEFINITION
directive @key(fields: _FieldSet!) on OBJECT | INTERFACE
directive @extends on OBJECT | INTERFACE
`
	apolloSchemaQueries = `
type Query {
	_entities(representations: [_Any!]!): [_Entity]!
	_service: _Service!
}
`
)

// Filters for Boolean and enum aren't needed in here schemaExtras because they are
// generated directly for the bool field / enum.  E.g. if
// `type T { b: Boolean @search }`,
// then the filter allows `filter: {b: true}`.  That's better than having
// `input BooleanFilter { eq: Boolean }`, which would require writing
// `filter: {b: {eq: true}}`.
//
// It'd be nice to be able to just write `filter: isPublished` for say a Post
// with a Boolean isPublished field, but there's no way to get that in GraphQL
// because input union types aren't yet sorted out in GraphQL.  So it's gotta
// be `filter: {isPublished: true}`

type directiveValidator func(
	sch *ast.Schema,
	typ *ast.Definition,
	field *ast.FieldDefinition,
	dir *ast.Directive,
	secrets map[string]x.SensitiveByteSlice) gqlerror.List

type searchTypeIndex struct {
	gqlType string
	dgIndex string
}

var numUids = &ast.FieldDefinition{
	Name: NumUid,
	Type: &ast.Type{NamedType: "Int"},
}

// search arg -> supported GraphQL type
// == supported Dgraph index -> GraphQL type it applies to
var supportedSearches = map[string]searchTypeIndex{
	"int":          {"Int", "int"},
	"int64":        {"Int64", "int"},
	"float":        {"Float", "float"},
	"bool":         {"Boolean", "bool"},
	"hash":         {"String", "hash"},
	"exact":        {"String", "exact"},
	"term":         {"String", "term"},
	"fulltext":     {"String", "fulltext"},
	"trigram":      {"String", "trigram"},
	"regexp":       {"String", "trigram"},
	"year":         {"DateTime", "year"},
	"month":        {"DateTime", "month"},
	"day":          {"DateTime", "day"},
	"hour":         {"DateTime", "hour"},
	"point":        {"Point", "geo"},
	"polygon":      {"Polygon", "geo"},
	"multiPolygon": {"MultiPolygon", "geo"},
}

// GraphQL scalar/object type -> default search arg
// used if the schema specifies @search without an arg
var defaultSearches = map[string]string{
	"Boolean":      "bool",
	"Int":          "int",
	"Int64":        "int64",
	"Float":        "float",
	"String":       "term",
	"DateTime":     "year",
	"Point":        "point",
	"Polygon":      "polygon",
	"MultiPolygon": "multiPolygon",
}

// graphqlSpecScalars holds all the scalar types supported by the graphql spec.
var graphqlSpecScalars = map[string]bool{
	"Int":     true,
	"Float":   true,
	"String":  true,
	"Boolean": true,
	"ID":      true,
}

// Dgraph index filters that have contains intersecting filter
// directive.
var filtersCollisions = map[string][]string{
	"StringHashFilter":  {"StringExactFilter"},
	"StringExactFilter": {"StringHashFilter"},
}

// GraphQL types that can be used for ordering in orderasc and orderdesc.
var orderable = map[string]bool{
	"Int":      true,
	"Int64":    true,
	"Float":    true,
	"String":   true,
	"DateTime": true,
}

// GraphQL types that can be summed. Types that have a well defined addition function.
var summable = map[string]bool{
	"Int":   true,
	"Int64": true,
	"Float": true,
}

var enumDirectives = map[string]bool{
	"trigram": true,
	"hash":    true,
	"exact":   true,
	"regexp":  true,
}

// index name -> GraphQL input filter for that index
var builtInFilters = map[string]string{
	"bool":         "Boolean",
	"int":          "IntFilter",
	"int64":        "Int64Filter",
	"float":        "FloatFilter",
	"year":         "DateTimeFilter",
	"month":        "DateTimeFilter",
	"day":          "DateTimeFilter",
	"hour":         "DateTimeFilter",
	"term":         "StringTermFilter",
	"trigram":      "StringRegExpFilter",
	"regexp":       "StringRegExpFilter",
	"fulltext":     "StringFullTextFilter",
	"exact":        "StringExactFilter",
	"hash":         "StringHashFilter",
	"point":        "PointGeoFilter",
	"polygon":      "PolygonGeoFilter",
	"multiPolygon": "PolygonGeoFilter",
}

// GraphQL in-built type -> Dgraph scalar
var inbuiltTypeToDgraph = map[string]string{
	"ID":           "uid",
	"Boolean":      "bool",
	"Int":          "int",
	"Int64":        "int",
	"Float":        "float",
	"String":       "string",
	"DateTime":     "dateTime",
	"Password":     "password",
	"Point":        "geo",
	"Polygon":      "geo",
	"MultiPolygon": "geo",
}

func ValidatorNoOp(
	sch *ast.Schema,
	typ *ast.Definition,
	field *ast.FieldDefinition,
	dir *ast.Directive,
	secrets map[string]x.SensitiveByteSlice) gqlerror.List {
	return nil
}

var directiveValidators = map[string]directiveValidator{
	inverseDirective:        hasInverseValidation,
	searchDirective:         searchValidation,
	dgraphDirective:         dgraphDirectiveValidation,
	idDirective:             idValidation,
	subscriptionDirective:   ValidatorNoOp,
	secretDirective:         passwordValidation,
	authDirective:           ValidatorNoOp, // Just to get it printed into generated schema
	customDirective:         customDirectiveValidation,
	remoteDirective:         ValidatorNoOp,
	deprecatedDirective:     ValidatorNoOp,
	lambdaDirective:         lambdaDirectiveValidation,
	generateDirective:       ValidatorNoOp,
	apolloKeyDirective:      ValidatorNoOp,
	apolloExtendsDirective:  ValidatorNoOp,
	apolloExternalDirective: apolloExternalValidation,
}

// directiveLocationMap stores the directives and their locations for the ones which can be
// applied at type level in the user supplied schema. It is used during validation.
var directiveLocationMap = map[string]map[ast.DefinitionKind]bool{
	inverseDirective:      nil,
	searchDirective:       nil,
	dgraphDirective:       {ast.Object: true, ast.Interface: true},
	idDirective:           nil,
	subscriptionDirective: {ast.Object: true, ast.Interface: true},
	secretDirective:       {ast.Object: true, ast.Interface: true},
	authDirective:         {ast.Object: true, ast.Interface: true},
	customDirective:       nil,
	remoteDirective: {ast.Object: true, ast.Interface: true, ast.Union: true,
		ast.InputObject: true, ast.Enum: true},
	cascadeDirective:  nil,
	generateDirective: {ast.Object: true, ast.Interface: true},
}

// Struct to store parameters of @generate directive
type GenerateDirectiveParams struct {
	generateGetQuery       bool
	generateFilterQuery    bool
	generatePasswordQuery  bool
	generateAggregateQuery bool
	generateAddMutation    bool
	generateUpdateMutation bool
	generateDeleteMutation bool
	generateSubscription   bool
}

func parseGenerateDirectiveParams(defn *ast.Definition) *GenerateDirectiveParams {
	ret := &GenerateDirectiveParams{
		generateGetQuery:       true,
		generateFilterQuery:    true,
		generatePasswordQuery:  true,
		generateAggregateQuery: true,
		generateAddMutation:    true,
		generateUpdateMutation: true,
		generateDeleteMutation: true,
		generateSubscription:   false,
	}

	if dir := defn.Directives.ForName(generateDirective); dir != nil {

		if queryArg := dir.Arguments.ForName(generateQueryArg); queryArg != nil {
			if getField := queryArg.Value.Children.ForName(generateGetField); getField != nil {
				if getFieldVal, err := getField.Value(nil); err == nil {
					ret.generateGetQuery = getFieldVal.(bool)
				}
			}
			if queryField := queryArg.Value.Children.ForName(generateQueryField); queryField != nil {
				if queryFieldVal, err := queryField.Value(nil); err == nil {
					ret.generateFilterQuery = queryFieldVal.(bool)
				}
			}
			if passwordField := queryArg.Value.Children.ForName(generatePasswordField); passwordField != nil {
				if passwordFieldVal, err := passwordField.Value(nil); err == nil {
					ret.generatePasswordQuery = passwordFieldVal.(bool)
				}
			}
			if aggregateField := queryArg.Value.Children.ForName(generateAggregateField); aggregateField != nil {
				if aggregateFieldVal, err := aggregateField.Value(nil); err == nil {
					ret.generateAggregateQuery = aggregateFieldVal.(bool)
				}
			}
		}

		if mutationArg := dir.Arguments.ForName(generateMutationArg); mutationArg != nil {
			if addField := mutationArg.Value.Children.ForName(generateAddField); addField != nil {
				if addFieldVal, err := addField.Value(nil); err == nil {
					ret.generateAddMutation = addFieldVal.(bool)
				}
			}
			if updateField := mutationArg.Value.Children.ForName(generateUpdateField); updateField != nil {
				if updateFieldVal, err := updateField.Value(nil); err == nil {
					ret.generateUpdateMutation = updateFieldVal.(bool)
				}
			}
			if deleteField := mutationArg.Value.Children.ForName(generateDeleteField); deleteField != nil {
				if deleteFieldVal, err := deleteField.Value(nil); err == nil {
					ret.generateDeleteMutation = deleteFieldVal.(bool)
				}
			}
		}

		if subscriptionArg := dir.Arguments.ForName(generateSubscriptionArg); subscriptionArg != nil {
			if subscriptionVal, err := subscriptionArg.Value.Value(nil); err == nil {
				ret.generateSubscription = subscriptionVal.(bool)
			}
		}
	}

	return ret
}

var schemaDocValidations []func(schema *ast.SchemaDocument) gqlerror.List
var schemaValidations []func(schema *ast.Schema, definitions []string) gqlerror.List
var defnValidations, typeValidations []func(schema *ast.Schema, defn *ast.Definition) gqlerror.List
var fieldValidations []func(typ *ast.Definition, field *ast.FieldDefinition) gqlerror.List

func copyAstFieldDef(src *ast.FieldDefinition) *ast.FieldDefinition {
	var dirs ast.DirectiveList
	dirs = append(dirs, src.Directives...)

	// We add arguments for filters and order statements later.
	dst := &ast.FieldDefinition{
		Name:         src.Name,
		DefaultValue: src.DefaultValue,
		Type:         src.Type,
		Directives:   dirs,
		Arguments:    src.Arguments,
		Position:     src.Position,
	}
	return dst
}

// expandSchema adds schemaExtras to the doc and adds any fields inherited from interfaces into
// implementing types
func expandSchema(doc *ast.SchemaDocument) *gqlerror.Error {
	schemaExtras := schemaInputs + directiveDefs + filterInputs
	docExtras, gqlErr := parser.ParseSchema(&ast.Source{Input: schemaExtras})
	if gqlErr != nil {
		x.Panic(gqlErr)
	}

	// Cache the interface definitions in a map. They could also be defined after types which
	// implement them.
	interfaces := make(map[string]*ast.Definition)
	for _, defn := range doc.Definitions {
		if defn.Kind == ast.Interface {
			interfaces[defn.Name] = defn
		}
	}

	// Walk through type definitions which implement an interface and fill in the fields from the
	// interface.
	for _, defn := range doc.Definitions {
		if defn.Kind == ast.Object && len(defn.Interfaces) > 0 {
			fieldSeen := make(map[string]string)
			// fieldSeen a map from field name to interface name in which the field was seen.
			defFields := make(map[string]int64)
			// defFields is used to keep track of fields in the defn before any inherited fields are added to it.
			for _, d := range defn.Fields {
				defFields[d.Name]++
			}
			initialDefFields := defn.Fields
			// initialDefFields store initial field definitions of the type.
			for _, implements := range defn.Interfaces {
				i, ok := interfaces[implements]
				if !ok {
					// This would fail schema validation later.
					continue
				}
				fields := make([]*ast.FieldDefinition, 0, len(i.Fields))
				for _, field := range i.Fields {
					// If field name is repeated multiple times in type then it will result in validation error later.
					if defFields[field.Name] == 1 {
						if field.Type.String() != initialDefFields.ForName(field.Name).Type.String() {
							return gqlerror.ErrorPosf(defn.Position, "For type %s to implement interface"+
								" %s the field %s must have type %s", defn.Name, i.Name, field.Name, field.Type.String())
						}
						if fieldSeen[field.Name] == "" {
							// Overwrite the existing field definition in type with the field definition of interface
							*defn.Fields.ForName(field.Name) = *field
						} else if field.Type.NamedType != IDType {
							// If field definition is already written,just add interface definition in type
							// It will later results in validation error because of repeated fields
							fields = append(fields, copyAstFieldDef(field))
						}
					} else if field.Type.NamedType == IDType && fieldSeen[field.Name] != "" {
						// If ID type is already seen in any other interface then we don't copy it again
						// And validator won't throw error for id types later
						if field.Type.String() != defn.Fields.ForName(field.Name).Type.String() {
							return gqlerror.ErrorPosf(defn.Position, "field %s is of type %s in interface %s"+
								" and is of type %s in interface %s",
								field.Name, field.Type.String(), i.Name, defn.Fields.ForName(field.Name).Type.String(), fieldSeen[field.Name])
						}
					} else {
						// Creating a copy here is important, otherwise arguments like filter, order
						// etc. are added multiple times if the pointer is shared.
						fields = append(fields, copyAstFieldDef(field))
					}
					fieldSeen[field.Name] = i.Name
				}
				defn.Fields = append(fields, defn.Fields...)
				passwordDirective := i.Directives.ForName("secret")
				if passwordDirective != nil {
					defn.Directives = append(defn.Directives, passwordDirective)
				}
			}
		}
	}

	doc.Definitions = append(doc.Definitions, docExtras.Definitions...)
	doc.Directives = append(doc.Directives, docExtras.Directives...)
	expandSchemaWithApolloExtras(doc)
	return nil
}

func expandSchemaWithApolloExtras(doc *ast.SchemaDocument) {
	var apolloKeyTypes []string
	for _, defn := range doc.Definitions {
		if defn.Directives.ForName(apolloKeyDirective) != nil {
			apolloKeyTypes = append(apolloKeyTypes, defn.Name)
		}
	}

	// No need to Expand with Apollo federation Extras
	if len(apolloKeyTypes) == 0 {
		return
	}

	// Form _Entity union with all the entities
	// for e.g : union _Entity = A | B
	// where A and B are object with @key directives
	entityUnionDefinition := &ast.Definition{Kind: ast.Union, Name: "_Entity", Types: apolloKeyTypes}
	doc.Definitions = append(doc.Definitions, entityUnionDefinition)

	// Parse Apollo Queries and append to the Parsed Schema
	docApolloQueries, gqlErr := parser.ParseSchema(&ast.Source{Input: apolloSchemaQueries})
	if gqlErr != nil {
		x.Panic(gqlErr)
	}

	queryDefinition := doc.Definitions.ForName("Query")
	if queryDefinition == nil {
		doc.Definitions = append(doc.Definitions, docApolloQueries.Definitions[0])
	} else {
		queryDefinition.Fields = append(queryDefinition.Fields, docApolloQueries.Definitions[0].Fields...)
	}

	docExtras, gqlErr := parser.ParseSchema(&ast.Source{Input: apolloSchemaExtras})
	if gqlErr != nil {
		x.Panic(gqlErr)
	}
	doc.Definitions = append(doc.Definitions, docExtras.Definitions...)
	doc.Directives = append(doc.Directives, docExtras.Directives...)

}

// preGQLValidation validates schema before GraphQL validation.  Validation
// before GraphQL validation means the schema only has allowed structures, and
// means we can give better errors than GrqphQL validation would give if their
// schema contains something that will fail because of the extras we inject into
// the schema.
func preGQLValidation(schema *ast.SchemaDocument) gqlerror.List {
	var errs []*gqlerror.Error

	for _, defn := range schema.Definitions {
		if defn.BuiltIn {
			// prelude definitions are built in and we don't want to validate them.
			continue
		}
		errs = append(errs, applyDefnValidations(defn, nil, defnValidations)...)
	}

	errs = append(errs, applySchemaDocValidations(schema)...)

	return errs
}

// postGQLValidation validates schema after gql validation.  Some validations
// are easier to run once we know that the schema is GraphQL valid and that validation
// has fleshed out the schema structure; we just need to check if it also satisfies
// the extra rules.
func postGQLValidation(schema *ast.Schema, definitions []string,
	secrets map[string]x.SensitiveByteSlice) gqlerror.List {
	var errs []*gqlerror.Error

	for _, defn := range definitions {
		typ := schema.Types[defn]

		errs = append(errs, applyDefnValidations(typ, schema, typeValidations)...)

		for _, field := range typ.Fields {
			errs = append(errs, applyFieldValidations(typ, field)...)

			for _, dir := range field.Directives {
				if directiveValidators[dir.Name] == nil {
					continue
				}
				errs = append(errs, directiveValidators[dir.Name](schema, typ, field, dir, secrets)...)
			}
		}
	}

	errs = append(errs, applySchemaValidations(schema, definitions)...)

	return errs
}

func applySchemaDocValidations(schema *ast.SchemaDocument) gqlerror.List {
	var errs []*gqlerror.Error

	for _, rule := range schemaDocValidations {
		newErrs := rule(schema)
		for _, err := range newErrs {
			errs = appendIfNotNull(errs, err)
		}
	}

	return errs
}

func applySchemaValidations(schema *ast.Schema, definitions []string) gqlerror.List {
	var errs []*gqlerror.Error

	for _, rule := range schemaValidations {
		newErrs := rule(schema, definitions)
		for _, err := range newErrs {
			errs = appendIfNotNull(errs, err)
		}
	}

	return errs
}

func applyDefnValidations(defn *ast.Definition, schema *ast.Schema,
	rules []func(schema *ast.Schema, defn *ast.Definition) gqlerror.List) gqlerror.List {
	var errs []*gqlerror.Error
	for _, rule := range rules {
		errs = append(errs, rule(schema, defn)...)
	}
	return errs
}

func applyFieldValidations(typ *ast.Definition, field *ast.FieldDefinition) gqlerror.List {
	var errs []*gqlerror.Error

	for _, rule := range fieldValidations {
		errs = append(errs, rule(typ, field)...)
	}

	return errs
}

// completeSchema generates all the required types and fields for
// query/mutation/update for all the types mentioned in the schema.
// In case of Apollo service Query, input types from queries and mutations
// are excluded due to the limited support currently.
func completeSchema(sch *ast.Schema, definitions []string, apolloServiceQuery bool) {
	query := sch.Types["Query"]
	if query != nil {
		query.Kind = ast.Object
		sch.Query = query
	} else {
		sch.Query = &ast.Definition{
			Kind:   ast.Object,
			Name:   "Query",
			Fields: make([]*ast.FieldDefinition, 0),
		}
	}

	mutation := sch.Types["Mutation"]
	if mutation != nil {
		mutation.Kind = ast.Object
		sch.Mutation = mutation
	} else {
		sch.Mutation = &ast.Definition{
			Kind:   ast.Object,
			Name:   "Mutation",
			Fields: make([]*ast.FieldDefinition, 0),
		}
	}

	sch.Subscription = &ast.Definition{
		Kind:   ast.Object,
		Name:   "Subscription",
		Fields: make([]*ast.FieldDefinition, 0),
	}

	for _, key := range definitions {
		defn := sch.Types[key]
		if key == "Query" {
			for _, q := range defn.Fields {
				subsDir := q.Directives.ForName(subscriptionDirective)
				customDir := q.Directives.ForName(customDirective)
				if subsDir != nil && customDir != nil {
					sch.Subscription.Fields = append(sch.Subscription.Fields, q)
				}
			}
			continue
		}
		if isQueryOrMutation(key) {
			continue
		}

		if defn.Kind == ast.Union {
			// TODO: properly check the case of reverse predicates (~) with union members and clean
			// them from unionRef or unionFilter as required.
			addUnionReferenceType(sch, defn)
			addUnionFilterType(sch, defn)
			addUnionMemberTypeEnum(sch, defn)
			continue
		}

		if defn.Kind != ast.Interface && defn.Kind != ast.Object {
			continue
		}

		params := parseGenerateDirectiveParams(defn)

		// Common types to both Interface and Object.
		addReferenceType(sch, defn)

		if params.generateUpdateMutation {
			addPatchType(sch, defn)
			addUpdateType(sch, defn)
			addUpdatePayloadType(sch, defn)
		}

		if params.generateDeleteMutation {
			addDeletePayloadType(sch, defn)
		}

		switch defn.Kind {
		case ast.Interface:
			// addInputType doesn't make sense as interface is like an abstract class and we can't
			// create objects of its type.
			if params.generateUpdateMutation {
				addUpdateMutation(sch, defn)
			}
			if params.generateDeleteMutation {
				addDeleteMutation(sch, defn)
			}

		case ast.Object:
			// types and inputs needed for mutations
			if params.generateAddMutation {
				addInputType(sch, defn)
				addAddPayloadType(sch, defn)
			}
			addMutations(sch, defn, params)
		}

		// types and inputs needed for query and search
		addFilterType(sch, defn)
		addTypeOrderable(sch, defn)
		addFieldFilters(sch, defn, apolloServiceQuery)
		addAggregationResultType(sch, defn)
		// Don't expose queries for the @extends type to the gateway
		// as it is resolved through `_entities` resolver.
		if !(apolloServiceQuery && hasExtends(defn)) {
			addQueries(sch, defn, params)
		}
		addTypeHasFilter(sch, defn)
		// We need to call this at last as aggregateFields
		// should not be part of HasFilter or UpdatePayloadType etc.
		addAggregateFields(sch, defn, apolloServiceQuery)
	}
}

func cleanupInput(sch *ast.Schema, def *ast.Definition, seen map[string]bool) {
	// seen helps us avoid cycles
	if def == nil || seen[def.Name] {
		return
	}

	i := 0
	// recursively walk over fields for an input type and delete those which are themselves empty.
	for _, f := range def.Fields {
		nt := f.Type.Name()
		enum := sch.Types[nt] != nil && sch.Types[nt].Kind == "ENUM"
		// Lets skip scalar types and enums.
		if _, ok := inbuiltTypeToDgraph[nt]; ok || enum {
			def.Fields[i] = f
			i++
			continue
		}

		seen[def.Name] = true
		cleanupInput(sch, sch.Types[nt], seen)

		// If after calling cleanup on an input type, it got deleted then it doesn't need to be
		// in the fields for this type anymore.
		if sch.Types[nt] == nil {
			continue
		}
		def.Fields[i] = f
		i++
	}
	def.Fields = def.Fields[:i]

	// In case of UpdateTypeInput, if TypePatch gets cleaned up then it becomes
	// input UpdateTypeInput {
	//		filter: TypeFilter!
	// }
	// In this case, UpdateTypeInput should also be deleted.
	if len(def.Fields) == 0 || (strings.HasPrefix(def.Name, "Update") && len(def.Fields) == 1) {
		delete(sch.Types, def.Name)
	}
}

func cleanSchema(sch *ast.Schema) {
	// Let's go over inputs of the type TRef, TPatch AddTInput, UpdateTInput and delete the ones which
	// don't have field inside them.
	for k := range sch.Types {
		if strings.HasSuffix(k, "Ref") || strings.HasSuffix(k, "Patch") ||
			((strings.HasPrefix(k, "Add") || strings.HasPrefix(k, "Update")) && strings.HasSuffix(k, "Input")) {
			cleanupInput(sch, sch.Types[k], map[string]bool{})
		}
	}

	// Let's go over mutations and cleanup those which don't have AddTInput/UpdateTInput defined in the schema
	// anymore.
	i := 0 // helps us overwrite the array with valid entries.
	for _, field := range sch.Mutation.Fields {
		custom := field.Directives.ForName("custom")
		// We would only modify add/update
		if custom != nil || !(strings.HasPrefix(field.Name, "add") || strings.HasPrefix(field.Name, "update")) {
			sch.Mutation.Fields[i] = field
			i++
			continue
		}

		// addT / updateT type mutations have an input which is AddTInput / UpdateTInput so if that doesn't exist anymore,
		// we can delete the AddTPayload / UpdateTPayload and also skip this mutation.

		var typeName, input string
		if strings.HasPrefix(field.Name, "add") {
			typeName = field.Name[3:]
			input = "Add" + typeName + "Input"
		} else if strings.HasPrefix(field.Name, "update") {
			typeName = field.Name[6:]
			input = "Update" + typeName + "Input"
		}

		if sch.Types[input] == nil {
			delete(sch.Types, input)
			continue
		}
		sch.Mutation.Fields[i] = field
		i++

	}
	sch.Mutation.Fields = sch.Mutation.Fields[:i]
}

func addUnionReferenceType(schema *ast.Schema, defn *ast.Definition) {
	refTypeName := defn.Name + "Ref"
	refType := &ast.Definition{
		Kind: ast.InputObject,
		Name: refTypeName,
	}
	for _, typName := range defn.Types {
		refType.Fields = append(refType.Fields, &ast.FieldDefinition{
			Name: CamelCase(typName) + "Ref",
			// the TRef for every member type is guaranteed to exist because member types can
			// only be objects types, and a TRef is always generated for an object type
			Type: &ast.Type{NamedType: typName + "Ref"},
		})
	}
	schema.Types[refTypeName] = refType
}

func addUnionFilterType(schema *ast.Schema, defn *ast.Definition) {
	filterName := defn.Name + "Filter"
	filter := &ast.Definition{
		Kind: ast.InputObject,
		Name: defn.Name + "Filter",
		Fields: []*ast.FieldDefinition{
			// field for selecting the union member type to report back
			{
				Name: "memberTypes",
				Type: &ast.Type{Elem: &ast.Type{NamedType: defn.Name + "Type", NonNull: true}},
			},
		},
	}
	// adding fields for specifying type filter for each union member type
	for _, typName := range defn.Types {
		filter.Fields = append(filter.Fields, &ast.FieldDefinition{
			Name: CamelCase(typName) + "Filter",
			// the TFilter for every member type is guaranteed to exist because each member type
			// will either have an ID field or some other kind of field causing it to have hasFilter
			Type: &ast.Type{NamedType: typName + "Filter"},
		})
	}
	schema.Types[filterName] = filter
}

func addUnionMemberTypeEnum(schema *ast.Schema, defn *ast.Definition) {
	enumName := defn.Name + "Type"
	enum := &ast.Definition{
		Kind: ast.Enum,
		Name: enumName,
	}
	for _, typName := range defn.Types {
		enum.EnumValues = append(enum.EnumValues, &ast.EnumValueDefinition{Name: typName})
	}
	schema.Types[enumName] = enum
}

// For extended Type definition, if Field with ID type is also field with @key directive then
// it should be present in the addTypeInput as it should not be generated automatically by dgraph
// but determined by the value of field in the GraphQL service where the type is defined.
func addInputType(schema *ast.Schema, defn *ast.Definition) {
	field := getFieldsWithoutIDType(schema, defn)
	if hasExtends(defn) {
		idField := getIDField(defn)
		field = append(idField, field...)
	}

	if len(field) != 0 {
		schema.Types["Add"+defn.Name+"Input"] = &ast.Definition{
			Kind:   ast.InputObject,
			Name:   "Add" + defn.Name + "Input",
			Fields: field,
		}
	}
}

func addReferenceType(schema *ast.Schema, defn *ast.Definition) {
	var flds ast.FieldList
	if defn.Kind == ast.Interface {
		if !hasID(defn) && !hasXID(defn) {
			return
		}
		flds = append(getIDField(defn), getXIDField(defn)...)
	} else {
		flds = append(getIDField(defn), getFieldsWithoutIDType(schema, defn)...)
	}

	if len(flds) == 1 && (hasID(defn) || hasXID(defn)) {
		flds[0].Type.NonNull = true
	} else {
		for _, fld := range flds {
			fld.Type.NonNull = false
		}
	}

	if len(flds) != 0 {
		schema.Types[defn.Name+"Ref"] = &ast.Definition{
			Kind:   ast.InputObject,
			Name:   defn.Name + "Ref",
			Fields: flds,
		}
	}
}

func addUpdateType(schema *ast.Schema, defn *ast.Definition) {
	if !hasFilterable(defn) {
		return
	}
	if _, ok := schema.Types[defn.Name+"Patch"]; !ok {
		return
	}

	updType := &ast.Definition{
		Kind: ast.InputObject,
		Name: "Update" + defn.Name + "Input",
		Fields: append(
			ast.FieldList{&ast.FieldDefinition{
				Name: "filter",
				Type: &ast.Type{
					NamedType: defn.Name + "Filter",
					NonNull:   true,
				},
			}},
			&ast.FieldDefinition{
				Name: "set",
				Type: &ast.Type{
					NamedType: defn.Name + "Patch",
				},
			},
			&ast.FieldDefinition{
				Name: "remove",
				Type: &ast.Type{
					NamedType: defn.Name + "Patch",
				},
			}),
	}
	schema.Types["Update"+defn.Name+"Input"] = updType
}

func addPatchType(schema *ast.Schema, defn *ast.Definition) {
	if !hasFilterable(defn) {
		return
	}

	nonIDFields := getNonIDFields(schema, defn)
	if len(nonIDFields) == 0 {
		// The user might just have an external id field and nothing else. We don't generate patch
		// type in that case.
		return
	}

	patchDefn := &ast.Definition{
		Kind:   ast.InputObject,
		Name:   defn.Name + "Patch",
		Fields: nonIDFields,
	}
	schema.Types[defn.Name+"Patch"] = patchDefn

	for _, fld := range patchDefn.Fields {
		fld.Type.NonNull = false
	}
}

// addFieldFilters adds field arguments that allow filtering to all fields of
// defn that can be searched.  For example, if there's another type
// `type R { ... f: String @search(by: [term]) ... }`
// and defn has a field of type R, e.g. if defn is like
// `type T { ... g: R ... }`
// then a query should be able to filter on g by term search on f, like
// query {
//   getT(id: 0x123) {
//     ...
//     g(filter: { f: { anyofterms: "something" } }, first: 10) { ... }
//     ...
//   }
// }
func addFieldFilters(schema *ast.Schema, defn *ast.Definition, apolloServiceQuery bool) {
	for _, fld := range defn.Fields {
		// Filtering and ordering for fields with @custom/@lambda directive is handled by the remote
		// endpoint.
		if hasCustomOrLambda(fld) {
			continue
		}

		// Don't add Filters for @extended types as they can't be filtered.
		if apolloServiceQuery && hasExtends(schema.Types[fld.Type.Name()]) {
			continue
		}

		// Filtering makes sense both for lists (= return only items that match
		// this filter) and for singletons (= only have this value in the result
		// if it satisfies this filter)
		addFilterArgument(schema, fld)

		// Ordering and pagination, however, only makes sense for fields of
		// list types (not scalar lists or enum lists).
		if isTypeList(fld) && !isEnumList(fld, schema) {
			addOrderArgument(schema, fld)

			// Pagination even makes sense when there's no orderables because
			// Dgraph will do UID order by default.
			addPaginationArguments(fld)
		}
	}
}

// addAggregateFields adds aggregate fields for fields which are of
// type list of object. eg. If defn is like
// type T {fiedldA : [A]}
// The following aggregate field is added to type T
// fieldAAggregate(filter : AFilter) : AAggregateResult
// These fields are added to support aggregate queries like count, avg, min
func addAggregateFields(schema *ast.Schema, defn *ast.Definition, apolloServiceQuery bool) {
	for _, fld := range defn.Fields {

		// Don't generate Aggregate Queries for field whose types are extended
		// in the schema.
		if apolloServiceQuery && hasExtends(schema.Types[fld.Type.Name()]) {
			continue
		}
		// Aggregate Fields only makes sense for fields of
		// list types of kind Object or Interface
		// (not scalar lists or not singleton types or lists of other kinds).
		if isTypeList(fld) && !hasCustomOrLambda(fld) &&
			(schema.Types[fld.Type.Name()].Kind == ast.Object ||
				schema.Types[fld.Type.Name()].Kind == ast.Interface) {
			aggregateField := &ast.FieldDefinition{
				Name: fld.Name + "Aggregate",
				Type: &ast.Type{
					NamedType: fld.Type.Name() + "AggregateResult",
				},
			}
			addFilterArgumentForField(schema, aggregateField, fld.Type.Name())
			defn.Fields = append(defn.Fields, aggregateField)
		}
	}
}

func addFilterArgument(schema *ast.Schema, fld *ast.FieldDefinition) {
	addFilterArgumentForField(schema, fld, fld.Type.Name())
}

func addFilterArgumentForField(schema *ast.Schema, fld *ast.FieldDefinition, fldTypeName string) {
	// Don't add filters for inbuilt types like String, Point, Polygon ...
	if _, ok := inbuiltTypeToDgraph[fldTypeName]; ok {
		return
	}

	fldType := schema.Types[fldTypeName]
	if fldType.Kind == ast.Union || hasFilterable(fldType) {
		fld.Arguments = append(fld.Arguments,
			&ast.ArgumentDefinition{
				Name: "filter",
				Type: &ast.Type{NamedType: fldTypeName + "Filter"},
			})
	}
}

// addTypeHasFilter adds `enum TypeHasFilter {...}` to the Schema
// if the object/interface has a field other than the ID field
func addTypeHasFilter(schema *ast.Schema, defn *ast.Definition) {
	filterName := defn.Name + "HasFilter"
	filter := &ast.Definition{
		Kind: ast.Enum,
		Name: filterName,
	}

	for _, fld := range defn.Fields {
		if isID(fld) || hasCustomOrLambda(fld) {
			continue
		}
		// Ignore Fields with @external directives also excluding those which are present
		// as an argument in @key directive
		if hasExternal(fld) && !isKeyField(fld, defn) {
			continue
		}
		filter.EnumValues = append(filter.EnumValues,
			&ast.EnumValueDefinition{Name: fld.Name})
	}

	// Interfaces could have just ID field but Types cannot for eg:
	// interface I {
	// 	 id: ID!
	// }
	// is a valid interface but it do not have any field which can
	// be filtered using has filter

	if len(filter.EnumValues) > 0 {
		schema.Types[filterName] = filter
	}
}

func addOrderArgument(schema *ast.Schema, fld *ast.FieldDefinition) {
	fldType := fld.Type.Name()
	if hasOrderables(schema.Types[fldType]) {
		fld.Arguments = append(fld.Arguments,
			&ast.ArgumentDefinition{
				Name: "order",
				Type: &ast.Type{NamedType: fldType + "Order"},
			})
	}
}

func addPaginationArguments(fld *ast.FieldDefinition) {
	fld.Arguments = append(fld.Arguments,
		&ast.ArgumentDefinition{Name: "first", Type: &ast.Type{NamedType: "Int"}},
		&ast.ArgumentDefinition{Name: "offset", Type: &ast.Type{NamedType: "Int"}},
	)
}

// getFilterTypes converts search arguments of a field to graphql filter types.
func getFilterTypes(schema *ast.Schema, fld *ast.FieldDefinition, filterName string) []string {
	searchArgs := getSearchArgs(fld)
	filterNames := make([]string, len(searchArgs))

	for i, search := range searchArgs {
		filterNames[i] = builtInFilters[search]

		// For enum type, if the index is "hash" or "exact", we construct filter named
		// enumTypeName_hash/ enumTypeName_exact from StringHashFilter/StringExactFilter
		// by replacing the Argument type.
		if (search == "hash" || search == "exact") && schema.Types[fld.Type.Name()].Kind == ast.Enum {
			stringFilterName := fmt.Sprintf("String%sFilter", strings.Title(search))
			var l ast.FieldList

			for _, i := range schema.Types[stringFilterName].Fields {
				enumTypeName := fld.Type.Name()
				var typ *ast.Type

				if i.Type.Elem == nil {
					typ = &ast.Type{
						NamedType: enumTypeName,
					}
				} else {
					typ = &ast.Type{
						Elem: &ast.Type{NamedType: enumTypeName},
					}
				}

				l = append(l, &ast.FieldDefinition{
					Name:         i.Name,
					Type:         typ,
					Description:  i.Description,
					DefaultValue: i.DefaultValue,
				})
			}

			filterNames[i] = fld.Type.Name() + "_" + search
			schema.Types[filterNames[i]] = &ast.Definition{
				Kind:   ast.InputObject,
				Name:   filterNames[i],
				Fields: l,
			}
		}
	}

	return filterNames
}

// mergeAndAddFilters merges multiple filterTypes into one and adds it to the schema.
func mergeAndAddFilters(filterTypes []string, schema *ast.Schema, filterName string) {
	if len(filterTypes) <= 1 {
		// Filters only require to be merged if there are alteast 2
		return
	}

	var fieldList ast.FieldList
	for _, typeName := range filterTypes {
		fieldList = append(fieldList, schema.Types[typeName].Fields...)
	}

	schema.Types[filterName] = &ast.Definition{
		Kind:   ast.InputObject,
		Name:   filterName,
		Fields: fieldList,
	}
}

// addFilterType add a `input TFilter { ... }` type to the schema, if defn
// is a type that has fields that can be filtered on.  This type filter is used
// in constructing the corresponding query
// queryT(filter: TFilter, ... )
// and in adding search to any fields of this type, like:
// type R {
//   f(filter: TFilter, ... ): T
//   ...
// }
func addFilterType(schema *ast.Schema, defn *ast.Definition) {
	filterName := defn.Name + "Filter"
	filter := &ast.Definition{
		Kind: ast.InputObject,
		Name: filterName,
	}

	for _, fld := range defn.Fields {
		// Ignore Fields with @external directives also excluding those which are present
		// as an argument in @key directive
		if hasExternal(fld) && !isKeyField(fld, defn) {
			continue
		}

		if isID(fld) {
			filter.Fields = append(filter.Fields,
				&ast.FieldDefinition{
					Name: fld.Name,
					Type: ast.ListType(&ast.Type{
						NamedType: IDType,
						NonNull:   true,
					}, nil),
				})
			continue
		}

		filterTypes := getFilterTypes(schema, fld, filterName)
		if len(filterTypes) > 0 {
			filterName := strings.Join(filterTypes, "_")
			filter.Fields = append(filter.Fields,
				&ast.FieldDefinition{
					Name: fld.Name,
					Type: &ast.Type{
						NamedType: filterName,
					},
				})

			mergeAndAddFilters(filterTypes, schema, filterName)
		}
	}

	// Has filter makes sense only if there is atleast one non ID field in the defn
	if len(getFieldsWithoutIDType(schema, defn)) > 0 {
		filter.Fields = append(filter.Fields,
			&ast.FieldDefinition{Name: "has", Type: &ast.Type{Elem: &ast.Type{NamedType: defn.Name + "HasFilter"}}},
		)
	}

	// Not filter makes sense even if the filter has only one field. And/Or would only make sense
	// if the filter has more than one field or if it has one non-id field.
	if (len(filter.Fields) == 1 && !isID(filter.Fields[0])) || len(filter.Fields) > 1 {
		filter.Fields = append(filter.Fields,
			&ast.FieldDefinition{Name: "and", Type: &ast.Type{Elem: &ast.Type{NamedType: filterName}}},
			&ast.FieldDefinition{Name: "or", Type: &ast.Type{Elem: &ast.Type{NamedType: filterName}}},
		)
	}

	// filter must have atleast one field. So not filter should be there.
	// For eg, if defn has only one field,2 cases are possible:-
	// 1- it is of ID type : then it contains filter of id type
	// 2- it is of non-ID type :  then it will have 'has' filter
	filter.Fields = append(filter.Fields,
		&ast.FieldDefinition{Name: "not", Type: &ast.Type{NamedType: filterName}},
	)

	schema.Types[filterName] = filter
}

// hasFilterable Returns whether TypeFilter for a defn will be generated or not.
// It returns true if any field have search arguments or it is an `ID` field or
// there is atleast one non-custom filter which would be the part of the has filter.
func hasFilterable(defn *ast.Definition) bool {
	return fieldAny(defn.Fields,
		func(fld *ast.FieldDefinition) bool {
			return len(getSearchArgs(fld)) != 0 || isID(fld) || !hasCustomOrLambda(fld)
		})
}

// Returns if given field is a list of type
// This returns true for list of all non scalar types
func isTypeList(fld *ast.FieldDefinition) bool {
	_, scalar := inbuiltTypeToDgraph[fld.Type.Name()]
	return !scalar && fld.Type.Elem != nil
}

// Returns true if given field is a list of enum
func isEnumList(fld *ast.FieldDefinition, sch *ast.Schema) bool {
	typeDefn := sch.Types[fld.Type.Name()]
	return typeDefn.Kind == "ENUM" && fld.Type.Elem != nil
}

func hasOrderables(defn *ast.Definition) bool {
	return fieldAny(defn.Fields, func(fld *ast.FieldDefinition) bool {
		return isOrderable(fld, defn)
	})
}

func isOrderable(fld *ast.FieldDefinition, defn *ast.Definition) bool {
	// lists can't be ordered and NamedType will be empty for lists,
	// so it will return false for list fields
	// External field can't be ordered except when it is a @key field
	if !hasExternal(fld) {
		return orderable[fld.Type.NamedType] && !hasCustomOrLambda(fld)
	}
	return isKeyField(fld, defn)
}

// Returns true if the field is of type which can be summed. Eg: int, int64, float
func isSummable(fld *ast.FieldDefinition) bool {
	return summable[fld.Type.NamedType] && !hasCustomOrLambda(fld)
}

func hasID(defn *ast.Definition) bool {
	return fieldAny(nonExternalAndKeyFields(defn), isID)
}

func hasXID(defn *ast.Definition) bool {
	return fieldAny(nonExternalAndKeyFields(defn), hasIDDirective)
}

// fieldAny returns true if any field in fields satisfies pred
func fieldAny(fields ast.FieldList, pred func(*ast.FieldDefinition) bool) bool {
	for _, fld := range fields {
		if pred(fld) {
			return true
		}
	}
	return false
}

func addHashIfRequired(fld *ast.FieldDefinition, indexes []string) []string {
	id := fld.Directives.ForName(idDirective)
	if id != nil {
		// If @id directive is applied along with @search, we check if the search has hash as an
		// arg. If it doesn't, then we add it.
		containsHash := false
		for _, index := range indexes {
			if index == "hash" {
				containsHash = true
			}
		}
		if !containsHash {
			indexes = append(indexes, "hash")
		}
	}
	return indexes
}

func getDefaultSearchIndex(fldName string) string {
	if search, ok := defaultSearches[fldName]; ok {
		return search
	}
	// it's an enum - always has hash index
	return "hash"

}

// getSearchArgs returns the name of the search applied to fld, or ""
// if fld doesn't have a search directive.
func getSearchArgs(fld *ast.FieldDefinition) []string {
	search := fld.Directives.ForName(searchDirective)
	id := fld.Directives.ForName(idDirective)
	fldType := fld.Type.Name()
	if search == nil {
		if id == nil {
			return nil
		}
		switch fldType {
		// If search directive wasn't supplied but id was, then hash is the only index
		// that we apply for string.
		case "String":
			return []string{"hash"}
		default:
			return []string{getDefaultSearchIndex(fldType)}
		}
	}
	if len(search.Arguments) == 0 ||
		len(search.Arguments.ForName(searchArgs).Value.Children) == 0 {
		return []string{getDefaultSearchIndex(fldType)}
	}
	val := search.Arguments.ForName(searchArgs).Value
	res := make([]string, len(val.Children))

	for i, child := range val.Children {
		res[i] = child.Value.Raw
	}

	res = addHashIfRequired(fld, res)
	sort.Strings(res)
	return res
}

// addTypeOrderable adds an input type that allows ordering in query.
// Two things are added: an enum with the names of all the orderable fields,
// for a type T that's called TOrderable; and an input type that allows saying
// order asc or desc, for type T that's called TOrder.
// TOrder's fields are TOrderable's.  So you
// might get:
// enum PostOrderable { datePublished, numLikes, ... }, and
// input PostOrder { asc : PostOrderable, desc: PostOrderable ...}
// Together they allow things like
// order: { asc: datePublished }
// and
// order: { asc: datePublished, then: { desc: title } }
//
// Dgraph allows multiple orderings `orderasc: datePublished, orderasc: title`
// to order by datePublished and then by title when dataPublished is the same.
// GraphQL doesn't allow the same field to be repeated, so
// `orderasc: datePublished, orderasc: title` wouldn't be valid.  Instead, our
// GraphQL orderings are given by the structure
// `order: { asc: datePublished, then: { asc: title } }`.
// a further `then` would be a third ordering, etc.
func addTypeOrderable(schema *ast.Schema, defn *ast.Definition) {
	if !hasOrderables(defn) {
		return
	}

	orderName := defn.Name + "Order"
	orderableName := defn.Name + "Orderable"

	schema.Types[orderName] = &ast.Definition{
		Kind: ast.InputObject,
		Name: orderName,
		Fields: ast.FieldList{
			&ast.FieldDefinition{Name: "asc", Type: &ast.Type{NamedType: orderableName}},
			&ast.FieldDefinition{Name: "desc", Type: &ast.Type{NamedType: orderableName}},
			&ast.FieldDefinition{Name: "then", Type: &ast.Type{NamedType: orderName}},
		},
	}

	order := &ast.Definition{
		Kind: ast.Enum,
		Name: orderableName,
	}

	for _, fld := range defn.Fields {

		if isOrderable(fld, defn) {
			order.EnumValues = append(order.EnumValues,
				&ast.EnumValueDefinition{Name: fld.Name})
		}
	}

	schema.Types[orderableName] = order
}

func addAddPayloadType(schema *ast.Schema, defn *ast.Definition) {
	qry := &ast.FieldDefinition{
		Name: CamelCase(defn.Name),
		Type: ast.ListType(&ast.Type{
			NamedType: defn.Name,
		}, nil),
	}

	addFilterArgument(schema, qry)
	addOrderArgument(schema, qry)
	addPaginationArguments(qry)
	if schema.Types["Add"+defn.Name+"Input"] != nil {
		schema.Types["Add"+defn.Name+"Payload"] = &ast.Definition{
			Kind:   ast.Object,
			Name:   "Add" + defn.Name + "Payload",
			Fields: []*ast.FieldDefinition{qry, numUids},
		}
	}
}

func addUpdatePayloadType(schema *ast.Schema, defn *ast.Definition) {
	if !hasFilterable(defn) {
		return
	}

	// This covers the case where the Type only had one field (which had @id directive).
	// Since we don't allow updating the field with @id directive we don't need to generate any
	// update payload.
	if _, ok := schema.Types[defn.Name+"Patch"]; !ok {
		return
	}

	qry := &ast.FieldDefinition{
		Name: CamelCase(defn.Name),
		Type: &ast.Type{
			Elem: &ast.Type{
				NamedType: defn.Name,
			},
		},
	}

	addFilterArgument(schema, qry)
	addOrderArgument(schema, qry)
	addPaginationArguments(qry)

	schema.Types["Update"+defn.Name+"Payload"] = &ast.Definition{
		Kind: ast.Object,
		Name: "Update" + defn.Name + "Payload",
		Fields: []*ast.FieldDefinition{
			qry, numUids,
		},
	}
}

func addDeletePayloadType(schema *ast.Schema, defn *ast.Definition) {
	if !hasFilterable(defn) {
		return
	}

	qry := &ast.FieldDefinition{
		Name: CamelCase(defn.Name),
		Type: ast.ListType(&ast.Type{
			NamedType: defn.Name,
		}, nil),
	}

	addFilterArgument(schema, qry)
	addOrderArgument(schema, qry)
	addPaginationArguments(qry)

	msg := &ast.FieldDefinition{
		Name: "msg",
		Type: &ast.Type{NamedType: "String"},
	}

	schema.Types["Delete"+defn.Name+"Payload"] = &ast.Definition{
		Kind:   ast.Object,
		Name:   "Delete" + defn.Name + "Payload",
		Fields: []*ast.FieldDefinition{qry, msg, numUids},
	}
}

func addAggregationResultType(schema *ast.Schema, defn *ast.Definition) {
	aggregationResultTypeName := defn.Name + "AggregateResult"

	var aggregateFields []*ast.FieldDefinition

	countField := &ast.FieldDefinition{
		Name: "count",
		Type: &ast.Type{NamedType: "Int"},
	}

	aggregateFields = append(aggregateFields, countField)

	// Add Maximum and Minimum fields for fields which have an ordering defined
	// Maximum and Minimum fields are added for fields which are of type int, int64,
	// float, string, datetime .
	for _, fld := range defn.Fields {
		// Creating aggregateFieldType to store type of the aggregate fields like
		// max, min, avg, sum of scalar fields.
		aggregateFieldType := &ast.Type{
			NamedType: fld.Type.NamedType,
			NonNull:   false,
			// Explicitly setting NonNull to false as AggregateResultType is not used
			// as input type and the fields may not be always needed.
		}

		// Adds titleMax, titleMin fields for a field of name title.
		if isOrderable(fld, defn) {
			minField := &ast.FieldDefinition{
				Name: fld.Name + "Min",
				Type: aggregateFieldType,
			}
			maxField := &ast.FieldDefinition{
				Name: fld.Name + "Max",
				Type: aggregateFieldType,
			}
			aggregateFields = append(aggregateFields, minField, maxField)
		}

		// Adds scoreSum and scoreAvg field for a field of name score.
		// The type of scoreAvg is Float irrespective of the type of score.
		if isSummable(fld) {
			sumField := &ast.FieldDefinition{
				Name: fld.Name + "Sum",
				Type: aggregateFieldType,
			}
			avgField := &ast.FieldDefinition{
				Name: fld.Name + "Avg",
				Type: &ast.Type{
					// Average should always be of type Float
					NamedType: "Float",
					NonNull:   false,
				},
			}
			aggregateFields = append(aggregateFields, sumField, avgField)
		}
	}

	schema.Types[aggregationResultTypeName] = &ast.Definition{
		Kind:   ast.Object,
		Name:   aggregationResultTypeName,
		Fields: aggregateFields,
	}
}

func addGetQuery(schema *ast.Schema, defn *ast.Definition, generateSubscription bool) {
	hasIDField := hasID(defn)
	hasXIDField := hasXID(defn)
	if !hasIDField && (defn.Kind == "INTERFACE" || !hasXIDField) {
		return
	}
	qry := &ast.FieldDefinition{
		Name: "get" + defn.Name,
		Type: &ast.Type{
			NamedType: defn.Name,
		},
	}

	// If the defn, only specified one of ID/XID field, they they are mandatory. If it specified
	// both, then they are optional.
	if hasIDField {
		fields := getIDField(defn)
		qry.Arguments = append(qry.Arguments, &ast.ArgumentDefinition{
			Name: fields[0].Name,
			Type: &ast.Type{
				NamedType: idTypeFor(defn),
				NonNull:   !hasXIDField,
			},
		})
	}
	if hasXIDField && defn.Kind != "INTERFACE" {
		name, dtype := xidTypeFor(defn)
		qry.Arguments = append(qry.Arguments, &ast.ArgumentDefinition{
			Name: name,
			Type: &ast.Type{
				NamedType: dtype,
				NonNull:   !hasIDField,
			},
		})
	}
	schema.Query.Fields = append(schema.Query.Fields, qry)
	subs := defn.Directives.ForName(subscriptionDirective)
	if subs != nil || generateSubscription {
		schema.Subscription.Fields = append(schema.Subscription.Fields, qry)
	}
}

func addFilterQuery(schema *ast.Schema, defn *ast.Definition, generateSubscription bool) {
	qry := &ast.FieldDefinition{
		Name: "query" + defn.Name,
		Type: &ast.Type{
			Elem: &ast.Type{
				NamedType: defn.Name,
			},
		},
	}
	addFilterArgument(schema, qry)
	addOrderArgument(schema, qry)
	addPaginationArguments(qry)

	schema.Query.Fields = append(schema.Query.Fields, qry)
	subs := defn.Directives.ForName(subscriptionDirective)
	if subs != nil || generateSubscription {
		schema.Subscription.Fields = append(schema.Subscription.Fields, qry)
	}

}

func addAggregationQuery(schema *ast.Schema, defn *ast.Definition, generateSubscription bool) {
	qry := &ast.FieldDefinition{
		Name: "aggregate" + defn.Name,
		Type: &ast.Type{
			NamedType: defn.Name + "AggregateResult",
		},
	}
	addFilterArgumentForField(schema, qry, defn.Name)

	schema.Query.Fields = append(schema.Query.Fields, qry)
	subs := defn.Directives.ForName(subscriptionDirective)
	if subs != nil || generateSubscription {
		schema.Subscription.Fields = append(schema.Subscription.Fields, qry)
	}

}

func addPasswordQuery(schema *ast.Schema, defn *ast.Definition) {
	hasIDField := hasID(defn)
	hasXIDField := hasXID(defn)
	if !hasIDField && !hasXIDField {
		return
	}

	idField := getIDField(defn)
	if !hasIDField {
		idField = getXIDField(defn)
	}
	passwordField := getPasswordField(defn)
	if passwordField == nil {
		return
	}

	qry := &ast.FieldDefinition{
		Name: "check" + defn.Name + "Password",
		Type: &ast.Type{
			NamedType: defn.Name,
		},
		Arguments: []*ast.ArgumentDefinition{
			{
				Name: idField[0].Name,
				Type: idField[0].Type,
			},
			{
				Name: passwordField.Name,
				Type: &ast.Type{
					NamedType: "String",
					NonNull:   true,
				},
			},
		},
	}
	schema.Query.Fields = append(schema.Query.Fields, qry)
}

func addQueries(schema *ast.Schema, defn *ast.Definition, params *GenerateDirectiveParams) {
	if params.generateGetQuery {
		addGetQuery(schema, defn, params.generateSubscription)
	}

	if params.generatePasswordQuery {
		addPasswordQuery(schema, defn)
	}

	if params.generateFilterQuery {
		addFilterQuery(schema, defn, params.generateSubscription)
	}

	if params.generateAggregateQuery {
		addAggregationQuery(schema, defn, params.generateSubscription)
	}
}

func addAddMutation(schema *ast.Schema, defn *ast.Definition) {
	if schema.Types["Add"+defn.Name+"Input"] == nil {
		return
	}

	add := &ast.FieldDefinition{
		Name: "add" + defn.Name,
		Type: &ast.Type{
			NamedType: "Add" + defn.Name + "Payload",
		},
		Arguments: []*ast.ArgumentDefinition{
			{
				Name: "input",
				Type: &ast.Type{
					NamedType: "[Add" + defn.Name + "Input!]",
					NonNull:   true,
				},
			},
		},
	}
	if hasXID(defn) {
		add.Arguments = append(add.Arguments,
			&ast.ArgumentDefinition{
				Name: "upsert",
				Type: &ast.Type{NamedType: "Boolean"},
			})
	}

	schema.Mutation.Fields = append(schema.Mutation.Fields, add)

}

func addUpdateMutation(schema *ast.Schema, defn *ast.Definition) {
	if !hasFilterable(defn) {
		return
	}

	if _, ok := schema.Types[defn.Name+"Patch"]; !ok {
		return
	}

	upd := &ast.FieldDefinition{
		Name: "update" + defn.Name,
		Type: &ast.Type{
			NamedType: "Update" + defn.Name + "Payload",
		},
		Arguments: []*ast.ArgumentDefinition{
			{
				Name: "input",
				Type: &ast.Type{
					NamedType: "Update" + defn.Name + "Input",
					NonNull:   true,
				},
			},
		},
	}
	schema.Mutation.Fields = append(schema.Mutation.Fields, upd)
}

func addDeleteMutation(schema *ast.Schema, defn *ast.Definition) {
	if !hasFilterable(defn) {
		return
	}

	del := &ast.FieldDefinition{
		Name: "delete" + defn.Name,
		Type: &ast.Type{
			NamedType: "Delete" + defn.Name + "Payload",
		},
		Arguments: []*ast.ArgumentDefinition{
			{
				Name: "filter",
				Type: &ast.Type{NamedType: defn.Name + "Filter", NonNull: true},
			},
		},
	}
	schema.Mutation.Fields = append(schema.Mutation.Fields, del)
}

func addMutations(schema *ast.Schema, defn *ast.Definition, params *GenerateDirectiveParams) {
	if params.generateAddMutation {
		addAddMutation(schema, defn)
	}
	if params.generateUpdateMutation {
		addUpdateMutation(schema, defn)
	}
	if params.generateDeleteMutation {
		addDeleteMutation(schema, defn)
	}
}

func createField(schema *ast.Schema, fld *ast.FieldDefinition) *ast.FieldDefinition {
	fieldTypeKind := schema.Types[fld.Type.Name()].Kind
	if fieldTypeKind == ast.Object || fieldTypeKind == ast.Interface || fieldTypeKind == ast.Union {
		newDefn := &ast.FieldDefinition{
			Name: fld.Name,
		}

		newDefn.Type = &ast.Type{}
		newDefn.Type.NonNull = fld.Type.NonNull
		if fld.Type.NamedType != "" {
			newDefn.Type.NamedType = fld.Type.Name() + "Ref"
		} else {
			newDefn.Type.Elem = &ast.Type{
				NamedType: fld.Type.Name() + "Ref",
				NonNull:   fld.Type.Elem.NonNull,
			}
		}

		return newDefn
	}

	newFld := *fld
	newFldType := *fld.Type
	newFld.Type = &newFldType
	newFld.Directives = nil
	newFld.Arguments = nil
	return &newFld
}

func getNonIDFields(schema *ast.Schema, defn *ast.Definition) ast.FieldList {
	fldList := make([]*ast.FieldDefinition, 0)
	for _, fld := range defn.Fields {
		if isIDField(defn, fld) || hasIDDirective(fld) {
			continue
		}

		// Ignore Fields with @external directives also as they shouldn't be present
		// in the Patch Type Also.
		if hasExternal(fld) {
			continue
		}
		// Fields with @custom/@lambda directive should not be part of mutation input,
		// hence we skip them.
		if hasCustomOrLambda(fld) {
			continue
		}

		// Remove edges which have a reverse predicate as they should only be updated through their
		// forward edge.
		fname := fieldName(fld, defn.Name)
		if strings.HasPrefix(fname, "~") || strings.HasPrefix(fname, "<~") {
			continue
		}

		// Even if a field isn't referenceable with an ID or XID, it can still go into an
		// input/update type because it can be created (but not linked by reference) as
		// part of the mutation.
		//
		// But if it's an interface, that can't happen because you can't directly create
		// interfaces - only the types that implement them
		if schema.Types[fld.Type.Name()].Kind == ast.Interface &&
			(!hasID(schema.Types[fld.Type.Name()]) && !hasXID(schema.Types[fld.Type.Name()])) {
			continue
		}

		fldList = append(fldList, createField(schema, fld))
	}

	pd := getPasswordField(defn)
	if pd == nil {
		return fldList
	}
	return append(fldList, pd)
}

func getFieldsWithoutIDType(schema *ast.Schema, defn *ast.Definition) ast.FieldList {
	fldList := make([]*ast.FieldDefinition, 0)
	for _, fld := range defn.Fields {
		if isIDField(defn, fld) {
			continue
		}

		// Ignore Fields with @external directives and excluding those which are present
		// as an argument in @key directive
		if hasExternal(fld) && !isKeyField(fld, defn) {
			continue
		}

		// Fields with @custom/@lambda directive should not be part of mutation input,
		// hence we skip them.
		if hasCustomOrLambda(fld) {
			continue
		}

		// Remove edges which have a reverse predicate as they should only be updated through their
		// forward edge.
		fname := fieldName(fld, defn.Name)
		if strings.HasPrefix(fname, "~") || strings.HasPrefix(fname, "<~") {
			continue
		}

		// see also comment in getNonIDFields
		if schema.Types[fld.Type.Name()].Kind == ast.Interface &&
			(!hasID(schema.Types[fld.Type.Name()]) && !hasXID(schema.Types[fld.Type.Name()])) {
			continue
		}
		fldList = append(fldList, createField(schema, fld))
	}

	pd := getPasswordField(defn)
	if pd == nil {
		return fldList
	}
	return append(fldList, pd)
}

func getIDField(defn *ast.Definition) ast.FieldList {
	fldList := make([]*ast.FieldDefinition, 0)
	for _, fld := range defn.Fields {
		if isIDField(defn, fld) {
			// Excluding those fields which are external and are not @key.
			if hasExternal(fld) && !isKeyField(fld, defn) {
				continue
			}
			newFld := *fld
			newFldType := *fld.Type
			newFld.Type = &newFldType
			newFld.Directives = nil
			newFld.Arguments = nil
			fldList = append(fldList, &newFld)
			break
		}
	}
	return fldList
}

func getPasswordField(defn *ast.Definition) *ast.FieldDefinition {
	var fldList *ast.FieldDefinition
	for _, directive := range defn.Directives {
		fd := convertPasswordDirective(directive)
		if fd == nil {
			continue
		}
		fldList = fd
	}
	return fldList
}

func getXIDField(defn *ast.Definition) ast.FieldList {
	fldList := make([]*ast.FieldDefinition, 0)
	for _, fld := range defn.Fields {
		if hasIDDirective(fld) {
			// Excluding those fields which are external and are not @key.
			if hasExternal(fld) && !isKeyField(fld, defn) {
				continue
			}
			newFld := *fld
			newFldType := *fld.Type
			newFld.Type = &newFldType
			newFld.Directives = nil
			newFld.Arguments = nil
			fldList = append(fldList, &newFld)
			break
		}
	}
	return fldList
}

func genArgumentsDefnString(args ast.ArgumentDefinitionList) string {
	if len(args) == 0 {
		return ""
	}

	argStrs := make([]string, len(args))
	for i, arg := range args {
		argStrs[i] = genArgumentDefnString(arg)
	}

	return fmt.Sprintf("(%s)", strings.Join(argStrs, ", "))
}

func genArgumentsString(args ast.ArgumentList) string {
	if len(args) == 0 {
		return ""
	}

	argStrs := make([]string, len(args))
	for i, arg := range args {
		argStrs[i] = genArgumentString(arg)
	}

	return fmt.Sprintf("(%s)", strings.Join(argStrs, ", "))
}

func genDirectivesString(direcs ast.DirectiveList) string {
	if len(direcs) == 0 {
		return ""
	}

	direcArgs := make([]string, len(direcs))
	idx := 0

	for _, dir := range direcs {
		if directiveValidators[dir.Name] == nil {
			continue
		}
		direcArgs[idx] = fmt.Sprintf("@%s%s", dir.Name, genArgumentsString(dir.Arguments))
		idx++
	}
	if idx == 0 {
		return ""
	}
	direcArgs = direcArgs[:idx]

	return " " + strings.Join(direcArgs, " ")
}

func genFieldsString(flds ast.FieldList) string {
	if flds == nil {
		return ""
	}

	var sch strings.Builder

	for _, fld := range flds {
		// Some extra types are generated by gqlparser for internal purpose.
		if !strings.HasPrefix(fld.Name, "__") {
			if d := generateDescription(fld.Description); d != "" {
				x.Check2(sch.WriteString(fmt.Sprintf("\t%s", d)))
			}
			x.Check2(sch.WriteString(genFieldString(fld)))
		}
	}

	return sch.String()
}

func genFieldString(fld *ast.FieldDefinition) string {
	return fmt.Sprintf(
		"\t%s%s: %s%s\n", fld.Name, genArgumentsDefnString(fld.Arguments),
		fld.Type.String(), genDirectivesString(fld.Directives))
}

func genArgumentDefnString(arg *ast.ArgumentDefinition) string {
	return fmt.Sprintf("%s: %s", arg.Name, arg.Type.String())
}

func genArgumentString(arg *ast.Argument) string {
	return fmt.Sprintf("%s: %s", arg.Name, arg.Value.String())
}

func generateInputString(typ *ast.Definition) string {
	return fmt.Sprintf("%sinput %s%s {\n%s}\n",
		generateDescription(typ.Description), typ.Name, genDirectivesString(typ.Directives),
		genFieldsString(typ.Fields))
}

func generateEnumString(typ *ast.Definition) string {
	var sch strings.Builder

	x.Check2(sch.WriteString(fmt.Sprintf("%senum %s {\n", generateDescription(typ.Description),
		typ.Name)))
	for _, val := range typ.EnumValues {
		if !strings.HasPrefix(val.Name, "__") {
			if d := generateDescription(val.Description); d != "" {
				x.Check2(sch.WriteString(fmt.Sprintf("\t%s", d)))
			}
			x.Check2(sch.WriteString(fmt.Sprintf("\t%s\n", val.Name)))
		}
	}
	x.Check2(sch.WriteString("}\n"))

	return sch.String()
}

func generateDescription(description string) string {
	if description == "" {
		return ""
	}

	return fmt.Sprintf("\"\"\"%s\"\"\"\n", description)
}

func generateInterfaceString(typ *ast.Definition) string {
	return fmt.Sprintf("%sinterface %s%s {\n%s}\n",
		generateDescription(typ.Description), typ.Name, genDirectivesString(typ.Directives),
		genFieldsString(typ.Fields))
}

func generateObjectString(typ *ast.Definition) string {
	if len(typ.Interfaces) > 0 {
		interfaces := strings.Join(typ.Interfaces, " & ")
		return fmt.Sprintf("%stype %s implements %s%s {\n%s}\n",
			generateDescription(typ.Description), typ.Name, interfaces,
			genDirectivesString(typ.Directives), genFieldsString(typ.Fields))
	}
	return fmt.Sprintf("%stype %s%s {\n%s}\n",
		generateDescription(typ.Description), typ.Name, genDirectivesString(typ.Directives),
		genFieldsString(typ.Fields))
}

func generateUnionString(typ *ast.Definition) string {
	return fmt.Sprintf("%sunion %s%s = %s\n",
		generateDescription(typ.Description), typ.Name, genDirectivesString(typ.Directives),
		strings.Join(typ.Types, " | "))
}

// Stringify the schema as a GraphQL SDL string.  It's assumed that the schema was
// built by completeSchema, and so contains an original set of definitions, the
// definitions from schemaExtras and generated types, queries and mutations.
//
// Any types in originalTypes are printed first, followed by the schemaExtras,
// and then all generated types, scalars, enums, directives, query and
// mutations all in alphabetical order.
// var "apolloServiceQuery" is used to distinguish Schema String from what should be
// returned as a result of apollo service query. In case of Apollo service query, Schema
// removes some of the directive definitions which are currently not supported at the gateway.
func Stringify(schema *ast.Schema, originalTypes []string, apolloServiceQuery bool) string {
	var sch, original, object, input, enum strings.Builder

	if schema.Types == nil {
		return ""
	}

	printed := make(map[string]bool)
	// Marked "_Service" type as printed as it will be printed in the
	// Extended Apollo Definitions
	printed["_Service"] = true
	// original defs can only be interface, type, union, enum or input.
	// print those in the same order as the original schema.
	for _, typName := range originalTypes {
		if isQueryOrMutation(typName) {
			// These would be printed later in schema.Query and schema.Mutation
			continue
		}
		typ := schema.Types[typName]
		switch typ.Kind {
		case ast.Interface:
			x.Check2(original.WriteString(generateInterfaceString(typ) + "\n"))
		case ast.Object:
			x.Check2(original.WriteString(generateObjectString(typ) + "\n"))
		case ast.Union:
			x.Check2(original.WriteString(generateUnionString(typ) + "\n"))
		case ast.Enum:
			x.Check2(original.WriteString(generateEnumString(typ) + "\n"))
		case ast.InputObject:
			x.Check2(original.WriteString(generateInputString(typ) + "\n"))
		}
		printed[typName] = true
	}

	// schemaExtras gets added to the result as a string, but we need to mark
	// off all it's contents as printed, so nothing in there gets printed with
	// the generated definitions.
	// In case of ApolloServiceQuery, schemaExtras is little different.
	// It excludes some of the directive definitions.
	schemaExtras := schemaInputs + directiveDefs + filterInputs
	if apolloServiceQuery {
		schemaExtras = schemaInputs + apolloSupportedDirectiveDefs + filterInputs
	}
	docExtras, gqlErr := parser.ParseSchema(&ast.Source{Input: schemaExtras})
	if gqlErr != nil {
		x.Panic(gqlErr)
	}
	for _, defn := range docExtras.Definitions {
		printed[defn.Name] = true
	}

	// schema.Types is all type names (types, inputs, enums, etc.).
	// The original schema defs have already been printed, and everything in
	// schemaExtras is marked as printed.  So build typeNames as anything
	// left to be printed.
	typeNames := make([]string, 0, len(schema.Types)-len(printed))
	for typName, typDef := range schema.Types {
		if isQueryOrMutation(typName) {
			// These would be printed later in schema.Query and schema.Mutation
			continue
		}
		if typDef.BuiltIn {
			// These are the types that are coming from ast.Prelude
			continue
		}
		if !printed[typName] {
			typeNames = append(typeNames, typName)
		}
	}
	sort.Strings(typeNames)

	// Now consider the types generated by completeSchema, which can only be
	// types, inputs and enums
	for _, typName := range typeNames {
		typ := schema.Types[typName]
		switch typ.Kind {
		case ast.Object:
			x.Check2(object.WriteString(generateObjectString(typ) + "\n"))
		case ast.InputObject:
			x.Check2(input.WriteString(generateInputString(typ) + "\n"))
		case ast.Enum:
			x.Check2(enum.WriteString(generateEnumString(typ) + "\n"))
		}
	}

	x.Check2(sch.WriteString(
		"#######################\n# Input Schema\n#######################\n\n"))
	x.Check2(sch.WriteString(original.String()))
	x.Check2(sch.WriteString(
		"#######################\n# Extended Definitions\n#######################\n"))
	x.Check2(sch.WriteString(schemaExtras))
	x.Check2(sch.WriteString("\n"))
	// Add Apollo Extras to the schema only when "_Entity" union is generated.
	if schema.Types["_Entity"] != nil {
		x.Check2(sch.WriteString(
			"#######################\n# Extended Apollo Definitions\n#######################\n"))
		x.Check2(sch.WriteString(generateUnionString(schema.Types["_Entity"])))
		x.Check2(sch.WriteString(apolloSchemaExtras))
		x.Check2(sch.WriteString("\n"))
	}
	if object.Len() > 0 {
		x.Check2(sch.WriteString(
			"#######################\n# Generated Types\n#######################\n\n"))
		x.Check2(sch.WriteString(object.String()))
	}
	if enum.Len() > 0 {
		x.Check2(sch.WriteString(
			"#######################\n# Generated Enums\n#######################\n\n"))
		x.Check2(sch.WriteString(enum.String()))
	}
	if input.Len() > 0 {
		x.Check2(sch.WriteString(
			"#######################\n# Generated Inputs\n#######################\n\n"))
		x.Check2(sch.WriteString(input.String()))
	}

	if len(schema.Query.Fields) > 0 {
		x.Check2(sch.WriteString(
			"#######################\n# Generated Query\n#######################\n\n"))
		x.Check2(sch.WriteString(generateObjectString(schema.Query) + "\n"))
	}

	if len(schema.Mutation.Fields) > 0 {
		x.Check2(sch.WriteString(
			"#######################\n# Generated Mutations\n#######################\n\n"))
		x.Check2(sch.WriteString(generateObjectString(schema.Mutation) + "\n"))
	}

	if schema.Subscription != nil && len(schema.Subscription.Fields) > 0 {
		x.Check2(sch.WriteString(
			"#######################\n# Generated Subscriptions\n#######################\n\n"))
		x.Check2(sch.WriteString(generateObjectString(schema.Subscription)))
	}

	return sch.String()
}

func isIDField(defn *ast.Definition, fld *ast.FieldDefinition) bool {
	return fld.Type.Name() == idTypeFor(defn)
}

func idTypeFor(defn *ast.Definition) string {
	return "ID"
}

func xidTypeFor(defn *ast.Definition) (string, string) {
	for _, fld := range nonExternalAndKeyFields(defn) {
		if hasIDDirective(fld) {
			return fld.Name, fld.Type.Name()
		}
	}
	return "", ""
}

func appendIfNotNull(errs []*gqlerror.Error, err *gqlerror.Error) gqlerror.List {
	if err != nil {
		errs = append(errs, err)
	}

	return errs
}

func isGraphqlSpecScalar(typ string) bool {
	_, ok := graphqlSpecScalars[typ]
	return ok
}

func CamelCase(x string) string {
	if x == "" {
		return ""
	}

	return strings.ToLower(x[:1]) + x[1:]
}
