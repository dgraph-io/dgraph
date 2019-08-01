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

	"github.com/mohae/deepcopy"
	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/gqlerror"
	"github.com/vektah/gqlparser/parser"
	"github.com/vektah/gqlparser/validator"
)

type SchemaHandler struct {
	Input          string
	initSchema     *ast.Schema
	completeSchema *ast.Schema
	errs           gqlerror.List
}

func (s *SchemaHandler) GQLSchema() (string, gqlerror.List) {
	s.bootStrap()

	if s.errs != nil {
		return "", s.errs
	}

	return Stringify(s.completeSchema), nil
}

func (s *SchemaHandler) DGSchema() (string, gqlerror.List) {
	s.bootStrap()

	if s.errs != nil {
		return "", s.errs
	}

	return genDgSchema(s.initSchema), nil
}

func (s *SchemaHandler) bootStrap() {
	if s.Input == "" {
		s.errs = append(s.errs, gqlerror.Errorf("No schema specified"))
		return
	}

	if s.completeSchema != nil {
		return
	}

	doc, gqlErr := parser.ParseSchema(&ast.Source{Input: s.Input})
	if gqlErr != nil {
		s.errs = append(s.errs, gqlErr)
		return
	}

	gqlErrList := validateSchema(doc)
	if gqlErrList != nil {
		s.errs = append(s.errs, gqlErrList...)
		return
	}

	addScalars(doc)

	sch, gqlErr := validator.ValidateSchemaDocument(doc)
	if gqlErr != nil {
		s.errs = append(s.errs, gqlErr)
		return
	}

	s.initSchema = deepcopy.Copy(sch).(*ast.Schema)

	GenerateCompleteSchema(sch)
	s.completeSchema = sch
}

// GenDgSchema generates Dgraph schema from a valid graphql schema.
func genDgSchema(gqlSch *ast.Schema) string {
	var typeStrings []string

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
					// TODO: still need to reverse in here

					typStr = fmt.Sprintf("%suid%s", prefix, suffix)

					fmt.Fprintf(&typeDef, "  %s.%s: %s\n", def.Name, f.Name, typStr)
					fmt.Fprintf(&preds, "%s.%s: %s .\n", def.Name, f.Name, typStr)
				case ast.Scalar:
					typStr = fmt.Sprintf("%s%s%s", prefix, strings.ToLower(f.Type.Name()), suffix)
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

			typeStrings = append(typeStrings, fmt.Sprintf("%s%s", typeDef.String(), preds.String()))
		}
	}

	return strings.Join(typeStrings, "")
}
