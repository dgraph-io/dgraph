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
	"sort"
	"strings"

	"github.com/vektah/gqlparser/ast"
)

// GenDgSchema generates Dgraph schema from a valid graphql schema.
func GenDgSchema(gqlSch *ast.Schema) string {
	// TODO: extract out as todo's below are done
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
			// It is assuming that there are no types with name having "Add" as a prefix.
			// Should we have this assumption ?
			if def.Name == "Query" ||
				def.Name == "Mutation" ||
				((strings.HasPrefix(def.Name, "Add") ||
					strings.HasPrefix(def.Name, "Delete") ||
					strings.HasPrefix(def.Name, "Update")) &&
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
					var typStr string

					typStr = "uid"
					if f.Type.Elem != nil {
						typStr = "[" + typStr + "]"
					}

					fmt.Fprintf(&typeDef, "  %s.%s: %s\n", def.Name, f.Name, typStr)
					fmt.Fprintf(&preds, "%s.%s: %s .\n", def.Name, f.Name, typStr)
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
			fmt.Fprintf(&typeDef, "}")

			typeStrings = append(typeStrings, fmt.Sprintf("%s\n%s", typeDef.String(), preds.String()))

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

	return strings.Join(typeStrings, "\n")
}
