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
	"github.com/vektah/gqlparser/validator"
)

type SchemaHandler interface {
	DGSchema() string
	GQLSchema() string
	Definitions() []string
}

type schemaHandler struct {
	Input          string
	completeSchema *ast.Schema
	dgraphSchema   string
	definitions    []string
}

func (s schemaHandler) GQLSchema() string {
	return Stringify(s.completeSchema)
}

func (s schemaHandler) DGSchema() string {
	return s.dgraphSchema
}

func (s schemaHandler) Definitions() []string {
	return s.definitions
}

// NewSchemaHandler processes the input schema, returns errorlist if any
// and the schemaHandler object.
func NewSchemaHandler(input string) (SchemaHandler, error) {
	if input == "" {
		return nil, gqlerror.Errorf("No schema specified")
	}

	handler := schemaHandler{Input: input}

	doc, gqlErr := parser.ParseSchema(&ast.Source{Input: input})
	if gqlErr != nil {
		return nil, gqlErr
	}

	gqlErrList := preGQLValidation(doc)
	if gqlErrList != nil {
		return nil, gqlErrList
	}

	addScalars(doc)

	defns := make([]string, len(doc.Definitions))

	for i, defn := range doc.Definitions {
		defns[i] = defn.Name
	}

	handler.definitions = defns

	sch, gqlErr := validator.ValidateSchemaDocument(doc)
	if gqlErr != nil {
		return nil, gqlErr
	}

	gqlErrList = postGQLValidation(sch, handler.definitions)
	if gqlErrList != nil {
		return nil, gqlErrList
	}

	handler.dgraphSchema = genDgSchema(sch)

	generateCompleteSchema(sch)
	handler.completeSchema = sch
	return handler, nil
}

// genDgSchema generates Dgraph schema from a valid graphql schema.
func genDgSchema(gqlSch *ast.Schema) string {
	var typeStrings []string

	// Sorting the keys so that the schema generated is always in the same order.
	var keys []string
	for k := range gqlSch.Types {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	for _, key := range keys {
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
