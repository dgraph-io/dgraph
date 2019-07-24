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

				var prefix, suffix string
				if f.Type.Elem != nil {
					prefix = "["
					suffix = "]"
				}

				switch gqlSch.Types[f.Type.Name()].Kind {
				case ast.Object:
					typStr := fmt.Sprintf("%suid%s", prefix, suffix)

					fmt.Fprintf(&typeDef, "  %s.%s: %s\n", def.Name, f.Name, typStr)
					fmt.Fprintf(&preds, "%s.%s: %s .\n", def.Name, f.Name, typStr)
				case ast.Scalar:
					var typStr string
					if f.Type.Elem != nil {
						typStr = strings.ToLower(f.Type.Elem.Name())
					} else {
						typStr = strings.ToLower(f.Type.Name())
					}

					typStr = fmt.Sprintf("%s%s%s", prefix, typStr, suffix)

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

			fmt.Fprintf(&schemaB, "%s%s\n", typeDef.String(), preds.String())
		}
	}

	return schemaB.String()
}
