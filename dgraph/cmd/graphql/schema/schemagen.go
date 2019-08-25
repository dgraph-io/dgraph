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

	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/gqlerror"
	"github.com/vektah/gqlparser/parser"
	"github.com/vektah/gqlparser/validator"
)

// A Handler can produce valid GraphQL and Dgraph schemas given an input of
// types and relationships
type Handler interface {
	DGSchema() string
	GQLSchema() string
}

type handler struct {
	Input          string
	completeSchema *ast.Schema
	dgraphSchema   string
}

func (s *handler) GQLSchema() string {
	return Stringify(s.completeSchema)
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

	// The input schema should contain just what's required to describe the types,
	// relationships and searchability ... but that's not really enough to make a
	// valid GraphQL schema: e.g. we allow a schema file like
	//
	// type T {
	//   f: Int @searchable
	// }
	//
	// But, that's not valid GraphQL unless there's also definitions of scalars
	// (Int, String, etc) and definitions of the directives (@searchable, etc).
	// We don't want to make the user have those in their file and then we have
	// to check that they've made the right definitions, etc, etc.
	//
	// So we allow input of just what the user cares about.  We parse that and
	// run a validation to make sure it only contains things that it should.
	// To that we add all the scalars and other definitions we set as defaults.
	// Then, we GraphQL validate to make sure their definitions plus our additions
	// is GraphQL valid.  Our next validation ensure that the definitions are made
	// in such a way that our GraphQL API will be able to interpret the schema
	// correctly.
	//
	// Then we can complete the process by adding in queries and mutations etc to
	// make the final full GraphQL schema.

	doc, gqlErr := parser.ParseSchema(&ast.Source{Input: input})
	if gqlErr != nil {
		return nil, gqlErr
	}

	gqlErrList := preGQLValidation(doc)
	if gqlErrList != nil {
		return nil, gqlErrList
	}

	addScalars(doc)
	addDirectives(doc)

	defns := make([]string, len(doc.Definitions))
	for i, defn := range doc.Definitions {
		defns[i] = defn.Name
	}

	sch, gqlErr := validator.ValidateSchemaDocument(doc)
	if gqlErr != nil {
		return nil, gqlErr
	}

	gqlErrList = postGQLValidation(sch, defns)
	if gqlErrList != nil {
		return nil, gqlErrList
	}

	handler := &handler{
		Input:          input,
		dgraphSchema:   genDgSchema(sch, defns),
		completeSchema: sch,
	}
	completeSchema(sch, defns)
	return handler, nil
}

// genDgSchema generates Dgraph schema from a valid graphql schema.
func genDgSchema(gqlSch *ast.Schema, definitions []string) string {
	var typeStrings []string

	for _, key := range definitions {
		def := gqlSch.Types[key]
		switch def.Kind {
		case ast.Object:
			var prefix, suffix string
			var typeDef, preds strings.Builder
			fmt.Fprintf(&typeDef, "type %s {\n", def.Name)
			for _, f := range def.Fields {
				if f.Type.Name() == "ID" {
					continue
				}

				if f.Type.Elem != nil {
					prefix = "["
					suffix = "]"
				}

				var typStr string
				switch gqlSch.Types[f.Type.Name()].Kind {
				case ast.Object:
					typStr = fmt.Sprintf("%suid%s", prefix, suffix)

					fmt.Fprintf(&typeDef, "  %s.%s: %s\n", def.Name, f.Name, typStr)
					fmt.Fprintf(&preds, "%s.%s: %s .\n", def.Name, f.Name, typStr)
				case ast.Scalar:
					typStr = fmt.Sprintf(
						"%s%s%s",
						prefix, supportedScalars[f.Type.Name()].dgraphType, suffix,
					)
					// TODO: indexes needed here
					fmt.Fprintf(&typeDef, "  %s.%s: %s\n",
						def.Name, f.Name, typStr)
					fmt.Fprintf(&preds, "%s.%s: %s .\n",
						def.Name, f.Name, typStr)
				case ast.Enum:
					fmt.Fprintf(&typeDef, "  %s.%s: string\n", def.Name, f.Name)
					fmt.Fprintf(&preds, "%s.%s: string @index(exact) .\n", def.Name, f.Name)
				}
			}
			fmt.Fprintf(&typeDef, "}\n")

			typeStrings = append(
				typeStrings,
				fmt.Sprintf("%s%s", typeDef.String(), preds.String()),
			)
		}
	}

	return strings.Join(typeStrings, "")
}
