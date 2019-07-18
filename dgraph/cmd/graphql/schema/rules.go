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
			return &gqlerror.Error{
				Message: "Only type and enums are allowed in initial schema.",
			}
		}
	}

	return nil
}

func idCountCheck(sch *ast.SchemaDocument) *gqlerror.Error {
	var flag bool
	for _, typeVal := range sch.Definitions {
		flag = false
		for _, fields := range typeVal.Fields {
			if fields.Type.NamedType == "ID" {
				if flag {
					return &gqlerror.Error{
						Message: "More than one ID field for type " + typeVal.Name,
					}
				}

				flag = true
			}
		}
	}

	return nil
}

func nameCheck(sch *ast.SchemaDocument) *gqlerror.Error {
	for _, defn := range sch.Definitions {
		if isReservedKeyWord(defn.Name) {
			return &gqlerror.Error{
				Message: defn.Name + " is reserved keyword. You can't declare" +
					"type with this name",
			}
		}
	}

	return nil
}

func listValidityCheck(sch *ast.SchemaDocument) *gqlerror.Error {
	for _, typ := range sch.Definitions {
		for _, fld := range typ.Fields {
			if fld.Type.Elem != nil && fld.Type.NonNull && !fld.Type.Elem.NonNull {
				return &gqlerror.Error{
					Message: "[" + fld.Type.Name() + "]! type of lists are invalid",
				}
			}
		}
	}

	return nil
}

func isReservedKeyWord(name string) bool {
	if name == string(INT) || name == string(BOOLEAN) ||
		name == string(FLOAT) || name == string(STRING) ||
		name == string(DATETIME) || name == string(ID) || name == "Query" || name == "Mutation" {
		return true
	}

	return false
}
