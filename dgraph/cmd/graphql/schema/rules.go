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

	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/gqlerror"
)

func init() {
	AddRule("OnlyTypeEnumInInitialSchema", dataTypeCheck)
	AddRule("OneIDPerType", idCountCheck)
	AddRule("TypeNameCantBeReservedKeyWords", nameCheck)
	AddRule("ValidListType", listValidityCheck)
}

func dataTypeCheck(sch *ast.SchemaDocument) *gqlerror.Error {
	for _, typ := range sch.Definitions {
		if typ.Kind != ast.Object && typ.Kind != ast.Enum {
			return gqlerror.ErrorPosf(typ.Position,
				"Only type and enums are allowed in initial schema.")
		}
	}

	return nil
}

func idCountCheck(sch *ast.SchemaDocument) *gqlerror.Error {
	var found bool
	for _, typeVal := range sch.Definitions {
		found = false
		for _, fld := range typeVal.Fields {
			if isIDField(typeVal, fld) {
				if found {
					return gqlerror.ErrorPosf(
						fld.Position,
						fmt.Sprintf("More than one ID field found for type %s.", typeVal.Name),
					)
				}

				found = true
			}
		}
	}

	return nil
}

func nameCheck(sch *ast.SchemaDocument) *gqlerror.Error {
	for _, defn := range sch.Definitions {
		if isReservedKeyWord(defn.Name) {
			return gqlerror.ErrorPosf(
				defn.Position,
				fmt.Sprintf(
					"%s is reserved keyword. You can't declare type with this name.", defn.Name,
				),
			)
		}
	}

	return nil
}

// [Posts]! -> invalid, [Posts!]! -> valid
func listValidityCheck(sch *ast.SchemaDocument) *gqlerror.Error {
	for _, typ := range sch.Definitions {
		for _, fld := range typ.Fields {
			if fld.Type.Elem != nil && fld.Type.NonNull && !fld.Type.Elem.NonNull {
				return gqlerror.ErrorPosf(
					fld.Position,
					fmt.Sprintf(
						"[%s]! type of lists are invalid. Valid options are [%s!]! and [%s!].",
						fld.Type.Name(), fld.Type.Name(), fld.Type.Name(),
					),
				)
			}
		}
	}

	return nil
}

func isScalar(s string) bool {
	for _, sc := range supportedScalars {
		if s == sc.name {
			return true
		}
	}
	return false
}

func isReservedKeyWord(name string) bool {
	if isScalar(name) || name == "Query" || name == "Mutation" {
		return true
	}

	return false
}
