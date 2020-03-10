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
	"strings"

	"github.com/pkg/errors"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"github.com/vektah/gqlparser/v2/parser"
	"github.com/vektah/gqlparser/v2/validator"
)

// A Handler can produce valid GraphQL and Dgraph schemas given an input of
// types and relationships
type Handler interface {
	DGSchema() string
	GQLSchema() string
}

type handler struct {
	input          string
	originalDefs   []string
	completeSchema *ast.Schema
	dgraphSchema   string
}

// FromString builds a GraphQL Schema from input string, or returns any parsing
// or validation errors.
func FromString(schema string) (Schema, error) {
	// validator.Prelude includes a bunch of predefined types which help with schema introspection
	// queries, hence we include it as part of the schema.
	doc, gqlErr := parser.ParseSchemas(validator.Prelude, &ast.Source{Input: schema})
	if gqlErr != nil {
		return nil, errors.Wrap(gqlErr, "while parsing GraphQL schema")
	}

	gqlSchema, gqlErr := validator.ValidateSchemaDocument(doc)
	if gqlErr != nil {
		return nil, errors.Wrap(gqlErr, "while validating GraphQL schema")
	}

	return AsSchema(gqlSchema), nil
}

func (s *handler) GQLSchema() string {
	return Stringify(s.completeSchema, s.originalDefs)
}

func (s *handler) DGSchema() string {
	return s.dgraphSchema
}

// NewHandler processes the input schema.  If there are no errors, it returns
// a valid Handler, otherwise it returns nil and an error.
func NewHandler(input string) (Handler, error) {
	if input == "" {
		return nil, gqlerror.Errorf("No schema specified")
	}

	// The input schema contains just what's required to describe the types,
	// relationships and searchability - but that's not enough to define a
	// valid GraphQL schema: e.g. we allow an input schema file like
	//
	// type T {
	//   f: Int @search
	// }
	//
	// But, that's not valid GraphQL unless there's also definitions of scalars
	// (Int, String, etc) and definitions of the directives (@search, etc).
	// We don't want to make the user have those in their file and then we have
	// to check that they've made the right definitions, etc, etc.
	//
	// So we parse the original input of just types and relationships and
	// run a validation to make sure it only contains things that it should.
	// To that we add all the scalars and other definitions we always require.
	//
	// Then, we GraphQL validate to make sure their definitions plus our additions
	// is GraphQL valid.  At this point we know the definitions are GraphQL valid,
	// but we need to check if it makes sense to our layer.
	//
	// The next final validation ensures that the definitions are made
	// in such a way that our GraphQL API will be able to interpret the schema
	// correctly.
	//
	// Then we can complete the process by adding in queries and mutations etc. to
	// make the final full GraphQL schema.

	doc, gqlErr := parser.ParseSchemas(validator.Prelude, &ast.Source{Input: input})
	if gqlErr != nil {
		return nil, gqlerror.List{gqlErr}
	}

	gqlErrList := preGQLValidation(doc)
	if gqlErrList != nil {
		return nil, gqlErrList
	}

	defns := make([]string, 0, len(doc.Definitions))
	for _, defn := range doc.Definitions {
		if defn.BuiltIn {
			continue
		}
		defns = append(defns, defn.Name)
	}

	expandSchema(doc)

	sch, gqlErr := validator.ValidateSchemaDocument(doc)
	if gqlErr != nil {
		return nil, gqlerror.List{gqlErr}
	}

	gqlErrList = postGQLValidation(sch, defns)
	if gqlErrList != nil {
		return nil, gqlErrList
	}

	dgSchema := genDgSchema(sch, defns)
	completeSchema(sch, defns)

	return &handler{
		input:          input,
		dgraphSchema:   dgSchema,
		completeSchema: sch,
		originalDefs:   defns,
	}, nil
}

func getAllSearchIndexes(val *ast.Value) []string {
	res := make([]string, len(val.Children))

	for i, child := range val.Children {
		res[i] = supportedSearches[child.Value.Raw].dgIndex
	}

	return res
}

func typeName(def *ast.Definition) string {
	name := def.Name
	dir := def.Directives.ForName(dgraphDirective)
	if dir == nil {
		return name
	}
	typeArg := dir.Arguments.ForName(dgraphTypeArg)
	if typeArg == nil {
		return name
	}
	return typeArg.Value.Raw
}

// fieldName returns the dgraph predicate corresponding to a field.
// If the field had a dgraph directive, then it returns the value of the name field otherwise
// it returns typeName + "." + fieldName.
func fieldName(def *ast.FieldDefinition, typName string) string {
	name := typName + "." + def.Name
	dir := def.Directives.ForName(dgraphDirective)
	if dir == nil {
		return name
	}
	predArg := dir.Arguments.ForName(dgraphPredArg)
	if predArg == nil {
		return name
	}
	return predArg.Value.Raw
}

// genDgSchema generates Dgraph schema from a valid graphql schema.
func genDgSchema(gqlSch *ast.Schema, definitions []string) string {
	var typeStrings []string

	type dgPred struct {
		typ     string
		index   string
		upsert  string
		reverse string
	}

	type field struct {
		name string
		// true if the field was inherited from an interface, we don't add the predicate schema
		// for it then as the it would already have been added with the interface.
		inherited bool
	}

	type dgType struct {
		name   string
		fields []field
	}

	dgTypes := make([]dgType, 0, len(definitions))
	dgPreds := make(map[string]dgPred)

	for _, key := range definitions {
		def := gqlSch.Types[key]
		switch def.Kind {
		case ast.Object, ast.Interface:
			typName := typeName(def)

			typ := dgType{name: typName}
			var typeDef strings.Builder
			fmt.Fprintf(&typeDef, "type %s {\n", typName)
			fd := getPasswordField(def)

			for _, f := range def.Fields {
				if f.Type.Name() == "ID" {
					continue
				}

				typName = typeName(def)
				// This field could have originally been defined in an interface that this type
				// implements. If we get a parent interface, then we should prefix the field name
				// with it instead of def.Name.
				parentInt := parentInterface(gqlSch, def, f.Name)
				if parentInt != nil {
					typName = typeName(parentInt)
				}
				fname := fieldName(f, typName)

				var prefix, suffix string
				if f.Type.Elem != nil {
					prefix = "["
					suffix = "]"
				}

				var typStr string
				switch gqlSch.Types[f.Type.Name()].Kind {
				case ast.Object:
					typStr = fmt.Sprintf("%suid%s", prefix, suffix)

					if parentInt == nil {
						if strings.HasPrefix(fname, "~") {
							// remove ~
							forwardEdge := fname[1:]
							forwardPred := dgPreds[forwardEdge]
							forwardPred.reverse = "@reverse "
							dgPreds[forwardEdge] = forwardPred
						} else {
							edge := dgPreds[fname]
							edge.typ = typStr
							dgPreds[fname] = edge
						}
					}
					typ.fields = append(typ.fields, field{fname, parentInt != nil})
				case ast.Scalar:
					typStr = fmt.Sprintf(
						"%s%s%s",
						prefix, scalarToDgraph[f.Type.Name()], suffix,
					)

					indexStr := ""
					upsertStr := ""
					search := f.Directives.ForName(searchDirective)
					id := f.Directives.ForName(idDirective)
					if id != nil {
						upsertStr = "@upsert "
					}

					if search != nil {
						arg := search.Arguments.ForName(searchArgs)
						if arg != nil {
							indexes := getAllSearchIndexes(arg.Value)
							indexes = addHashIfRequired(f, indexes)
							indexStr = fmt.Sprintf(" @index(%s)", strings.Join(indexes, ", "))
						} else {
							indexStr = fmt.Sprintf(" @index(%s)", defaultSearches[f.Type.Name()])
						}
					} else if id != nil {
						indexStr = fmt.Sprintf(" @index(hash)")
					}

					if parentInt == nil {
						dgPreds[fname] = dgPred{
							typ:    typStr,
							index:  indexStr,
							upsert: upsertStr,
						}
					}
					typ.fields = append(typ.fields, field{fname, parentInt != nil})
				case ast.Enum:
					typStr = fmt.Sprintf("%s%s%s", prefix, "string", suffix)

					indexStr := " @index(hash)"
					search := f.Directives.ForName(searchDirective)
					if search != nil {
						arg := search.Arguments.ForName(searchArgs)
						if arg != nil {
							indexes := getAllSearchIndexes(arg.Value)
							indexStr = fmt.Sprintf(" @index(%s)", strings.Join(indexes, ", "))
						}
					}
					if parentInt == nil {
						dgPreds[fname] = dgPred{
							typ:   typStr,
							index: indexStr,
						}
					}
					typ.fields = append(typ.fields, field{fname, parentInt != nil})
				}
			}
			if fd != nil {
				parentInt := parentInterface(gqlSch, def, fd.Name)
				if parentInt != nil {
					typName = typeName(parentInt)
				}
				fname := fieldName(fd, typName)

				if parentInt == nil {
					dgPreds[fname] = dgPred{typ: "password"}
				}

				typ.fields = append(typ.fields, field{fname, parentInt != nil})
			}
			dgTypes = append(dgTypes, typ)
		}
	}

	for _, typ := range dgTypes {
		var typeDef, preds strings.Builder
		fmt.Fprintf(&typeDef, "type %s {\n", typ.name)
		for _, fld := range typ.fields {
			f, ok := dgPreds[fld.name]
			if !ok {
				continue
			}
			fmt.Fprintf(&typeDef, "  %s\n", fld.name)
			if !fld.inherited {
				fmt.Fprintf(&preds, "%s: %s%s %s%s.\n", fld.name, f.typ, f.index, f.upsert,
					f.reverse)
			}
		}
		fmt.Fprintf(&typeDef, "}\n")
		typeStrings = append(
			typeStrings,
			fmt.Sprintf("%s%s", typeDef.String(), preds.String()),
		)
	}

	return strings.Join(typeStrings, "")
}
