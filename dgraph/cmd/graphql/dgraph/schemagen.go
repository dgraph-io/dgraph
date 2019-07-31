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

package dgraph

import (
	"fmt"
	"sort"
	"strings"

	"github.com/vektah/gqlparser/ast"
)

// GenDgSchema generates Dgraph schema from a valid graphql schema.
func GenDgSchema(gqlSch *ast.Schema) string {
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
