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

package dgschema

import (
	"fmt"
	"strings"

	"github.com/vektah/gqlparser/ast"
)

// GenDgSchema generates Dgraph schema from a valid graphql schema.
func GenDgSchema(gqlSch *ast.Schema) string {
	// TODO: extract out as todo's below are done
	var schemaB strings.Builder
	for _, def := range gqlSch.Types {
		switch def.Kind {
		case ast.Object:
			// It is assuming that there are no types with name having "Add" as a prefix.
			// Should we have this assumption ?
			if def.Name == "Query" ||
				def.Name == "Mutation" ||
				((strings.HasPrefix(def.Name, "Add") ||
					strings.HasPrefix(def.Name, "Delete") ||
					strings.HasPrefix(def.Name, "Mutate")) &&
					strings.HasSuffix(def.Name, "Payload")) {
				continue
			}

			var typeDef, preds strings.Builder
			fmt.Fprintf(&typeDef, "type %s {\n", def.Name)
			for _, f := range def.Fields {
				if f.Type.Name() == "ID" {
					continue
				}

				switch gqlSch.Types[f.Type.Name()].Kind {
				case ast.Object:
					// TODO: still need to write [] ! and reverse in here
					var typStr, direcStr string

					typStr = "uid"
					if f.Type.Elem != nil {
						typStr = "[" + typStr + "]"
					}

					direcStr = genDirecStr(f)

					fmt.Fprintf(&typeDef, "  %s.%s: %s %s\n", def.Name, f.Name, typStr, direcStr)
					fmt.Fprintf(&preds, "%s.%s: %s %s .\n", def.Name, f.Name, typStr, direcStr)
				case ast.Scalar:
					// TODO: indexes needed here
					fmt.Fprintf(&typeDef, "  %s.%s: %s\n",
						def.Name, f.Name, strings.ToLower(f.Type.Name()))
					fmt.Fprintf(&preds, "%s.%s: %s .\n",
						def.Name, f.Name, strings.ToLower(f.Type.Name()))
				case ast.Enum:
					fmt.Fprintf(&typeDef, "  %s.%s: string\n", def.Name, f.Name)
					fmt.Fprintf(&preds, "%s.%s: string @index(exact) .\n", def.Name, f.Name)
				}
			}
			fmt.Fprintf(&typeDef, "}\n")

			// Why are we printing both typedefs and predicates ?
			fmt.Fprintf(&schemaB, "%s%s\n", typeDef.String(), preds.String())

		case ast.Scalar:
			// nothing to do here?  There should only be known scalars, and that
			// should have been checked by the validation.
			// fmt.Printf("Got a scalar: %v %v\n", name, def)
		case ast.Enum:
			// ignore this? it's handled by the edges
			// fmt.Printf("Got an enum: %v %v\n", name, def)
		default:
			// ignore anything else?
			// fmt.Printf("Got something else: %v %v\n", name, def)
		}
	}

	return schemaB.String()
}

func genDirecStr(fld *ast.FieldDefinition) string {
	var sch strings.Builder

	for _, dir := range fld.Directives {
		if dir.Name == "hasInverse" {
			sch.WriteString("@reverse")
		}
	}

	return sch.String()
}
