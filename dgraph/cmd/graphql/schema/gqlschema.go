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

	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/gqlerror"
	"github.com/vektah/gqlparser/parser"
)

const (
	inverseDirective = "hasInverse"
	inverseArg       = "field"

	searchDirective = "search"
	searchArgs      = "by"

	// schemaExtras is everything that gets added to an input schema to make it
	// GraphQL valid and for the completion algorithm to use to build in search
	// capability into the schema.
	schemaExtras = `
scalar DateTime

enum DgraphIndex {
	int
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
}

directive @hasInverse(field: String!) on FIELD_DEFINITION
directive @search(by: [DgraphIndex!]) on FIELD_DEFINITION

input IntFilter {
	eq: Int
	le: Int
	lt: Int
	ge: Int
	gt: Int
}

input FloatFilter {
	eq: Float
	le: Float
	lt: Float
	ge: Float
	gt: Float
}

input DateTimeFilter {
	eq: DateTime
	le: DateTime
	lt: DateTime
	ge: DateTime
	gt: DateTime
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
	le: String
	lt: String
	ge: String
	gt: String
}

input StringHashFilter {
	eq: String
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
	dir *ast.Directive) *gqlerror.Error

type searchTypeIndex struct {
	gqlType string
	dgIndex string
}

// search arg -> supported GraphQL type
// == supported Dgraph index -> GraphQL type it applies to
var supportedSearches = map[string]searchTypeIndex{
	"int":      {"Int", "int"},
	"float":    {"Float", "float"},
	"bool":     {"Boolean", "bool"},
	"hash":     {"String", "hash"},
	"exact":    {"String", "exact"},
	"term":     {"String", "term"},
	"fulltext": {"String", "fulltext"},
	"trigram":  {"String", "trigram"},
	"regexp":   {"String", "trigram"},
	"year":     {"DateTime", "year"},
	"month":    {"DateTime", "month"},
	"day":      {"DateTime", "day"},
	"hour":     {"DateTime", "hour"},
}

// GraphQL scalar type -> default Dgraph index (/search)
// used if the schema specifies @search without an arg
var defaultSearches = map[string]string{
	"Boolean":  "bool",
	"Int":      "int",
	"Float":    "float",
	"String":   "term",
	"DateTime": "year",
}

// Dgraph index filters that have contains intersecting filter
// directive.
var filtersCollisions = map[string][]string{
	"StringHashFilter":  []string{"StringExactFilter"},
	"StringExactFilter": []string{"StringHashFilter"},
}

// GraphQL types that can be used for ordering in orderasc and orderdesc.
var orderable = map[string]bool{
	"Int":      true,
	"Float":    true,
	"String":   true,
	"DateTime": true,
}

// index name -> GraphQL input filter for that index
var builtInFilters = map[string]string{
	"bool":     "Boolean",
	"int":      "IntFilter",
	"float":    "FloatFilter",
	"year":     "DateTimeFilter",
	"month":    "DateTimeFilter",
	"day":      "DateTimeFilter",
	"hour":     "DateTimeFilter",
	"term":     "StringTermFilter",
	"trigram":  "StringRegExpFilter",
	"regexp":   "StringRegExpFilter",
	"fulltext": "StringFullTextFilter",
	"exact":    "StringExactFilter",
	"hash":     "StringHashFilter",
}

// GraphQL scalar -> Dgraph scalar
var scalarToDgraph = map[string]string{
	"ID":       "uid",
	"Boolean":  "bool",
	"Int":      "int",
	"Float":    "float",
	"String":   "string",
	"DateTime": "dateTime",
}

var directiveValidators = map[string]directiveValidator{
	inverseDirective: hasInverseValidation,
	searchDirective:  searchValidation,
}

var defnValidations, typeValidations []func(defn *ast.Definition) *gqlerror.Error
var fieldValidations []func(field *ast.FieldDefinition) *gqlerror.Error

func copyAstFieldDef(src *ast.FieldDefinition) *ast.FieldDefinition {
	// Lets leave out copying the arguments as types in input schemas are not supposed to contain
	// them. We add arguments for filters and order statements later.
	dst := &ast.FieldDefinition{
		Description:  src.Description,
		Name:         src.Name,
		DefaultValue: src.DefaultValue,
		Type:         src.Type,
		Directives:   src.Directives,
		Position:     src.Position,
	}
	return dst
}

func expandSchema(doc *ast.SchemaDocument) {
	docExtras, gqlErr := parser.ParseSchema(&ast.Source{Input: schemaExtras})
	if gqlErr != nil {
		panic(gqlErr)
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
			for _, implements := range defn.Interfaces {
				i, ok := interfaces[implements]
				if !ok {
					// This would fail schema validation later.
					continue
				}
				fields := make([]*ast.FieldDefinition, 0, len(i.Fields))
				for _, field := range i.Fields {
					// Creating a copy here is important, otherwise arguments like filter, order
					// etc. are added multiple times if the pointer is shared.
					fields = append(fields, copyAstFieldDef(field))
				}
				defn.Fields = append(fields, defn.Fields...)
			}
		}
	}

	for _, defn := range docExtras.Definitions {
		doc.Definitions = append(doc.Definitions, defn)
	}

	for _, dir := range docExtras.Directives {
		doc.Directives = append(doc.Directives, dir)
	}
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
		errs = append(errs, applyDefnValidations(defn, defnValidations)...)
	}

	return errs
}

// postGQLValidation validates schema after gql validation.  Some validations
// are easier to run once we know that the schema is GraphQL valid and that validation
// has fleshed out the schema structure; we just need to check if it also satisfies
// the extra rules.
func postGQLValidation(schema *ast.Schema, definitions []string) gqlerror.List {
	var errs []*gqlerror.Error

	for _, defn := range definitions {
		typ := schema.Types[defn]

		errs = append(errs, applyDefnValidations(typ, typeValidations)...)

		for _, field := range typ.Fields {
			errs = append(errs, applyFieldValidations(field)...)

			for _, dir := range field.Directives {
				errs = appendIfNotNull(errs,
					directiveValidators[dir.Name](schema, typ, field, dir))
			}
		}
	}

	return errs
}

func applyDefnValidations(defn *ast.Definition,
	rules []func(defn *ast.Definition) *gqlerror.Error) gqlerror.List {
	var errs []*gqlerror.Error

	for _, rule := range rules {
		errs = appendIfNotNull(errs, rule(defn))
	}

	return errs
}

func applyFieldValidations(field *ast.FieldDefinition) gqlerror.List {
	var errs []*gqlerror.Error

	for _, rule := range fieldValidations {
		errs = appendIfNotNull(errs, rule(field))
	}

	return errs
}

// completeSchema generates all the required types and fields for
// query/mutation/update for all the types mentioned the the schema.
func completeSchema(sch *ast.Schema, definitions []string) {

	sch.Query = &ast.Definition{
		Kind:        ast.Object,
		Description: "Root Query type",
		Name:        "Query",
		Fields:      make([]*ast.FieldDefinition, 0),
	}

	sch.Mutation = &ast.Definition{
		Kind:        ast.Object,
		Description: "Root Mutation type",
		Name:        "Mutation",
		Fields:      make([]*ast.FieldDefinition, 0),
	}

	for _, key := range definitions {
		defn := sch.Types[key]

		if defn.Kind != ast.Interface && defn.Kind != ast.Object {
			continue
		}

		// Common types to both Interface and Object.
		addReferenceType(sch, defn)
		addPatchType(sch, defn)
		addUpdateType(sch, defn)
		addUpdatePayloadType(sch, defn)
		addDeletePayloadType(sch, defn)

		if defn.Kind == ast.Interface {
			// addInputType doesn't make sense as interface is like an abstract class and we can't
			// create objects of its type.
			addUpdateMutation(sch, defn)
			addDeleteMutation(sch, defn)

		} else if defn.Kind == ast.Object {
			// types and inputs needed for mutations
			addInputType(sch, defn)
			addAddPayloadType(sch, defn)
			addMutations(sch, defn)
		}

		// types and inputs needed for query and search
		addFilterType(sch, defn)
		addTypeOrderable(sch, defn)
		addFieldFilters(sch, defn)
		addQueries(sch, defn)
	}
}

func addInputType(schema *ast.Schema, defn *ast.Definition) {
	schema.Types[defn.Name+"Input"] = &ast.Definition{
		Kind:   ast.InputObject,
		Name:   defn.Name + "Input",
		Fields: getNonIDFields(schema, defn),
	}
}

func addReferenceType(schema *ast.Schema, defn *ast.Definition) {
	if !hasID(defn) {
		return
	}

	schema.Types[defn.Name+"Ref"] = &ast.Definition{
		Kind:   ast.InputObject,
		Name:   defn.Name + "Ref",
		Fields: getIDField(defn),
	}
}

func addUpdateType(schema *ast.Schema, defn *ast.Definition) {
	if !hasID(defn) {
		return
	}

	updType := &ast.Definition{
		Kind: ast.InputObject,
		Name: "Update" + defn.Name + "Input",
		Fields: append(
			getIDField(defn),
			&ast.FieldDefinition{
				Name: "patch",
				Type: &ast.Type{
					NamedType: "Patch" + defn.Name,
					NonNull:   true,
				},
			}),
	}
	schema.Types["Update"+defn.Name+"Input"] = updType
}

func addPatchType(schema *ast.Schema, defn *ast.Definition) {
	if !hasID(defn) {
		return
	}

	patchDefn := &ast.Definition{
		Kind:   ast.InputObject,
		Name:   "Patch" + defn.Name,
		Fields: getNonIDFields(schema, defn),
	}
	schema.Types["Patch"+defn.Name] = patchDefn

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
func addFieldFilters(schema *ast.Schema, defn *ast.Definition) {
	for _, fld := range defn.Fields {
		// Filtering makes sense both for lists (= return only items that match
		// this filter) and for singletons (= only have this value in the result
		// if it satisfies this filter)
		addFilterArgument(schema, fld)

		// Ordering and pagination, however, only makes sense for fields of
		// list types (not scalar lists).
		if _, scalar := scalarToDgraph[fld.Type.Name()]; !scalar && fld.Type.Elem != nil {
			addOrderArgument(schema, fld)

			// Pagination even makes sense when there's no orderables because
			// Dgraph will do UID order by default.
			addPaginationArguments(fld)
		}
	}
}

func addFilterArgument(schema *ast.Schema, fld *ast.FieldDefinition) {
	fldType := fld.Type.Name()
	if hasFilterable(schema.Types[fldType]) {
		fld.Arguments = append(fld.Arguments,
			&ast.ArgumentDefinition{
				Name: "filter",
				Type: &ast.Type{NamedType: fldType + "Filter"},
			})
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
	if !hasFilterable(defn) {
		return
	}

	filterName := defn.Name + "Filter"
	filter := &ast.Definition{
		Kind: ast.InputObject,
		Name: filterName,
	}

	for _, fld := range defn.Fields {
		if isID(fld) {
			filter.Fields = append(filter.Fields,
				&ast.FieldDefinition{
					Name: "ids",
					Type: ast.ListType(&ast.Type{
						NamedType: IDType,
						NonNull:   true,
					}, nil),
				})
			continue
		}

		filterTypeNameUnion := []string{}
		for _, search := range getSearchArgs(fld) {
			filterTypeName := builtInFilters[search]
			if schema.Types[fld.Type.Name()].Kind == ast.Enum {
				// If the field is an enum type, we don't generate a filter type.
				// Instead we allow to write `typeName: enumValue` in the filter.
				// So, for example : `filter: { postType: Answer }`
				// rather than : `filter: { postType: { eq: Answer } }`
				//
				// Booleans are the same, allowing:
				// `filter: { isPublished: true }
				// but that case is already handled by builtInFilters
				filterTypeName = fld.Type.Name()
			}

			filterTypeNameUnion = append(filterTypeNameUnion, filterTypeName)
		}

		if len(filterTypeNameUnion) > 0 {
			filterTypeName := strings.Join(filterTypeNameUnion, "_")
			filter.Fields = append(filter.Fields,
				&ast.FieldDefinition{
					Name: fld.Name,
					Type: &ast.Type{
						NamedType: filterTypeName,
					},
				})

			//create a schema type
			if len(filterTypeNameUnion) > 1 {
				fieldList := ast.FieldList{}
				for _, typeName := range filterTypeNameUnion {
					fieldList = append(fieldList, schema.Types[typeName].Fields...)
				}

				schema.Types[filterTypeName] = &ast.Definition{
					Kind:   ast.InputObject,
					Name:   filterTypeName,
					Fields: fieldList,
				}
			}
		}
	}

	// Not filter makes sense even if the filter has only one field. And/Or would only make sense
	// if the filter has more than one field or if it has one non-id field.
	if (len(filter.Fields) == 1 && filter.Fields[0].Name != "ids") || len(filter.Fields) > 1 {
		filter.Fields = append(filter.Fields,
			&ast.FieldDefinition{Name: "and", Type: &ast.Type{NamedType: filterName}},
			&ast.FieldDefinition{Name: "or", Type: &ast.Type{NamedType: filterName}},
		)
	}

	filter.Fields = append(filter.Fields,
		&ast.FieldDefinition{Name: "not", Type: &ast.Type{NamedType: filterName}})
	schema.Types[filterName] = filter
}

func hasFilterable(defn *ast.Definition) bool {
	return fieldAny(defn.Fields,
		func(fld *ast.FieldDefinition) bool {
			return getSearchArgs(fld) != nil || isID(fld)
		})
}

func hasOrderables(defn *ast.Definition) bool {
	return fieldAny(defn.Fields,
		func(fld *ast.FieldDefinition) bool { return orderable[fld.Type.Name()] })
}

func hasID(defn *ast.Definition) bool {
	return fieldAny(defn.Fields,
		func(fld *ast.FieldDefinition) bool { return isID(fld) })
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

// getSearchArgs returns the name of the search applied to fld, or ""
// if fld doesn't have a search directive.
func getSearchArgs(fld *ast.FieldDefinition) []string {
	search := fld.Directives.ForName(searchDirective)
	if search == nil {
		return nil
	}
	if len(search.Arguments) == 0 {
		if search, ok := defaultSearches[fld.Type.Name()]; ok {
			return []string{search}
		}
		// it's an enum - always has exact index
		return []string{"exact"}
	}

	val := search.Arguments.ForName(searchArgs).Value
	res := make([]string, len(val.Children))

	for i, child := range val.Children {
		res[i] = child.Value.Raw
	}

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
		if orderable[fld.Type.Name()] {
			order.EnumValues = append(order.EnumValues,
				&ast.EnumValueDefinition{Name: fld.Name})
		}
	}

	schema.Types[orderableName] = order
}

func addAddPayloadType(schema *ast.Schema, defn *ast.Definition) {
	schema.Types["Add"+defn.Name+"Payload"] = &ast.Definition{
		Kind: ast.Object,
		Name: "Add" + defn.Name + "Payload",
		Fields: []*ast.FieldDefinition{
			&ast.FieldDefinition{
				Name: strings.ToLower(defn.Name),
				Type: &ast.Type{
					NamedType: defn.Name,
				},
			},
		},
	}
}

func addUpdatePayloadType(schema *ast.Schema, defn *ast.Definition) {
	if !hasID(defn) {
		return
	}

	schema.Types["Update"+defn.Name+"Payload"] = &ast.Definition{
		Kind: ast.Object,
		Name: "Update" + defn.Name + "Payload",
		Fields: []*ast.FieldDefinition{
			&ast.FieldDefinition{
				Name: strings.ToLower(defn.Name),
				Type: &ast.Type{
					NamedType: defn.Name,
				},
			},
		},
	}
}

func addDeletePayloadType(schema *ast.Schema, defn *ast.Definition) {
	if !hasFilterable(defn) {
		return
	}

	schema.Types["Delete"+defn.Name+"Payload"] = &ast.Definition{
		Kind: ast.Object,
		Name: "Delete" + defn.Name + "Payload",
		Fields: []*ast.FieldDefinition{
			&ast.FieldDefinition{
				Name: "msg",
				Type: &ast.Type{
					NamedType: "String",
				},
			},
		},
	}
}

func addGetQuery(schema *ast.Schema, defn *ast.Definition) {
	if !hasID(defn) {
		return
	}

	qry := &ast.FieldDefinition{
		Description: "Get " + defn.Name + " by ID",
		Name:        "get" + defn.Name,
		Type: &ast.Type{
			NamedType: defn.Name,
		},
		Arguments: []*ast.ArgumentDefinition{
			&ast.ArgumentDefinition{
				Name: "id",
				Type: &ast.Type{
					NamedType: idTypeFor(defn),
					NonNull:   true,
				},
			},
		},
	}
	schema.Query.Fields = append(schema.Query.Fields, qry)
}

func addFilterQuery(schema *ast.Schema, defn *ast.Definition) {
	qry := &ast.FieldDefinition{
		Description: "Query " + defn.Name,
		Name:        "query" + defn.Name,
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
}

func addQueries(schema *ast.Schema, defn *ast.Definition) {
	addGetQuery(schema, defn)
	addFilterQuery(schema, defn)
}

func addAddMutation(schema *ast.Schema, defn *ast.Definition) {
	add := &ast.FieldDefinition{
		Description: "Add a " + defn.Name,
		Name:        "add" + defn.Name,
		Type: &ast.Type{
			NamedType: "Add" + defn.Name + "Payload",
		},
		Arguments: []*ast.ArgumentDefinition{
			&ast.ArgumentDefinition{
				Name: "input",
				Type: &ast.Type{
					NamedType: defn.Name + "Input",
					NonNull:   true,
				},
			},
		},
	}
	schema.Mutation.Fields = append(schema.Mutation.Fields, add)
}

func addUpdateMutation(schema *ast.Schema, defn *ast.Definition) {
	if !hasID(defn) {
		return
	}

	upd := &ast.FieldDefinition{
		Description: "Update a " + defn.Name,
		Name:        "update" + defn.Name,
		Type: &ast.Type{
			NamedType: "Update" + defn.Name + "Payload",
		},
		Arguments: []*ast.ArgumentDefinition{
			&ast.ArgumentDefinition{
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
		Description: "Delete a " + defn.Name,
		Name:        "delete" + defn.Name,
		Type: &ast.Type{
			NamedType: "Delete" + defn.Name + "Payload",
		},
		Arguments: []*ast.ArgumentDefinition{
			&ast.ArgumentDefinition{
				Name: "filter",
				Type: &ast.Type{NamedType: defn.Name + "Filter", NonNull: true},
			},
		},
	}
	schema.Mutation.Fields = append(schema.Mutation.Fields, del)
}

func addMutations(schema *ast.Schema, defn *ast.Definition) {
	addAddMutation(schema, defn)
	addUpdateMutation(schema, defn)
	addDeleteMutation(schema, defn)
}

func getNonIDFields(schema *ast.Schema, defn *ast.Definition) ast.FieldList {
	fldList := make([]*ast.FieldDefinition, 0)
	for _, fld := range defn.Fields {
		if isIDField(defn, fld) {
			continue
		}

		if schema.Types[fld.Type.Name()].Kind == ast.Object &&
			!hasID(schema.Types[fld.Type.Name()]) { // types without ID, can't be referenced
			continue
		}

		if schema.Types[fld.Type.Name()].Kind == ast.Object {
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

			fldList = append(fldList, newDefn)
		} else {
			newFld := *fld
			newFldType := *fld.Type
			newFld.Type = &newFldType
			newFld.Directives = nil
			fldList = append(fldList, &newFld)
		}
	}
	return fldList
}

func getIDField(defn *ast.Definition) ast.FieldList {
	fldList := make([]*ast.FieldDefinition, 0)
	for _, fld := range defn.Fields {
		if isIDField(defn, fld) {
			// Deepcopy is not required because we don't modify values other than nonull
			newFld := *fld
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

	for idx, dir := range direcs {
		direcArgs[idx] = fmt.Sprintf("@%s%s", dir.Name, genArgumentsString(dir.Arguments))
	}

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
			sch.WriteString(genFieldString(fld))
		}
	}

	return sch.String()
}

func genFieldString(fld *ast.FieldDefinition) string {
	return fmt.Sprintf(
		"\t%s%s: %s%s\n", fld.Name, genArgumentsDefnString(fld.Arguments),
		fld.Type.String(), genDirectivesString(fld.Directives),
	)
}

func genArgumentDefnString(arg *ast.ArgumentDefinition) string {
	return fmt.Sprintf("%s: %s", arg.Name, arg.Type.String())
}

func genArgumentString(arg *ast.Argument) string {
	return fmt.Sprintf("%s: %s", arg.Name, arg.Value.String())
}

func generateInputString(typ *ast.Definition) string {
	return fmt.Sprintf("input %s {\n%s}\n", typ.Name, genFieldsString(typ.Fields))
}

func generateEnumString(typ *ast.Definition) string {
	var sch strings.Builder

	sch.WriteString(fmt.Sprintf("enum %s {\n", typ.Name))
	for _, val := range typ.EnumValues {
		if !strings.HasPrefix(val.Name, "__") {
			sch.WriteString(fmt.Sprintf("\t%s\n", val.Name))
		}
	}
	sch.WriteString("}\n")

	return sch.String()
}

func generateInterfaceString(typ *ast.Definition) string {
	return fmt.Sprintf("interface %s {\n%s}\n", typ.Name, genFieldsString(typ.Fields))
}

func generateObjectString(typ *ast.Definition) string {
	if len(typ.Interfaces) > 0 {
		interfaces := strings.Join(typ.Interfaces, " & ")
		return fmt.Sprintf("type %s implements %s {\n%s}\n", typ.Name, interfaces,
			genFieldsString(typ.Fields))
	}
	return fmt.Sprintf("type %s {\n%s}\n", typ.Name, genFieldsString(typ.Fields))
}

func generateScalarString(typ *ast.Definition) string {
	var sch strings.Builder

	sch.WriteString(fmt.Sprintf("scalar %s\n", typ.Name))
	return sch.String()
}

// Stringify the schema as a GraphQL SDL string.  It's assumed that the schema was
// built by completeSchema, and so contains an original set of definitions, the
// definitions from schemaExtras and generated types, queries and mutations.
//
// Any types in originalTypes are printed first, followed by the schemaExtras,
// and then all generated types, scalars, enums, directives, query and
// mutations all in alphabetical order.
func Stringify(schema *ast.Schema, originalTypes []string) string {
	var sch, original, object, input, enum strings.Builder

	if schema.Types == nil {
		return ""
	}

	printed := make(map[string]bool)

	// original defs can only be types and enums, print those in the same order
	// as the original schema.
	for _, typName := range originalTypes {
		typ := schema.Types[typName]
		if typ.Kind == ast.Interface {
			original.WriteString(generateInterfaceString(typ) + "\n")
		} else if typ.Kind == ast.Object {
			original.WriteString(generateObjectString(typ) + "\n")
		} else if typ.Kind == ast.Enum {
			original.WriteString(generateEnumString(typ) + "\n")
		}
		printed[typName] = true
	}

	// schemaExtras gets added to the result as a string, but we need to mark
	// off all it's contents as printed, so nothing in there gets printed with
	// the generated definitions.
	docExtras, gqlErr := parser.ParseSchema(&ast.Source{Input: schemaExtras})
	if gqlErr != nil {
		panic(gqlErr)
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
		if typ.Kind == ast.Object {
			object.WriteString(generateObjectString(typ) + "\n")
		} else if typ.Kind == ast.InputObject {
			input.WriteString(generateInputString(typ) + "\n")
		} else if typ.Kind == ast.Enum {
			enum.WriteString(generateEnumString(typ) + "\n")
		}
	}

	sch.WriteString("#######################\n# Input Schema\n#######################\n\n")
	sch.WriteString(original.String())
	sch.WriteString("#######################\n# Extended Definitions\n#######################\n")
	sch.WriteString(schemaExtras)
	sch.WriteString("\n")
	sch.WriteString("#######################\n# Generated Types\n#######################\n\n")
	sch.WriteString(object.String())
	sch.WriteString("#######################\n# Generated Enums\n#######################\n\n")
	sch.WriteString(enum.String())
	sch.WriteString("#######################\n# Generated Inputs\n#######################\n\n")
	sch.WriteString(input.String())
	sch.WriteString("#######################\n# Generated Query\n#######################\n\n")
	sch.WriteString(generateObjectString(schema.Query) + "\n")
	sch.WriteString("#######################\n# Generated Mutations\n#######################\n\n")
	sch.WriteString(generateObjectString(schema.Mutation))

	return sch.String()
}

func isIDField(defn *ast.Definition, fld *ast.FieldDefinition) bool {
	return fld.Type.Name() == idTypeFor(defn)
}

func idTypeFor(defn *ast.Definition) string {
	// Placeholder till more ID types are introduced.
	return "ID"
}

func appendIfNotNull(errs []*gqlerror.Error, err *gqlerror.Error) gqlerror.List {
	if err != nil {
		errs = append(errs, err)
	}

	return errs
}
