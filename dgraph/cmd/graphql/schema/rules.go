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
)

func init() {
	AddRule("OnlyTypeEnumInInitialSchema", dataTypeCheck)
	AddRule("OneIDPerType", idCountCheck)
	AddRule("TypeNameCantBeReservedKeyWords", nameCheck)
	AddRule("ValidListType", listValidityCheck)
}

func dataTypeCheck(sch *ast.SchemaDocument) gqlerror.List {
	var errs []*gqlerror.Error

	for _, typ := range sch.Definitions {
		if typ.Kind != ast.Object && typ.Kind != ast.Enum {
			errs = append(errs, gqlerror.ErrorPosf(
				typ.Position,
				"Only type and enums are allowed in initial schema.",
			))
		}
	}

	return errs
}

func idCountCheck(sch *ast.SchemaDocument) gqlerror.List {
	var errs []*gqlerror.Error

	for _, typeVal := range sch.Definitions {
		var idFields []*ast.FieldDefinition
		for _, field := range typeVal.Fields {
			if isIDField(typeVal, field) {
				idFields = append(idFields, field)
			}
		}

		if len(idFields) > 1 {
			var fieldNames []string
			var errLocations []gqlerror.Location

			for _, f := range idFields {
				fieldNames = append(fieldNames, f.Name)
				errLocations = append(errLocations, gqlerror.Location{
					Line:   f.Position.Line,
					Column: f.Position.Column,
				})
			}

			fieldNamesString := strings.Join(fieldNames, ", ")
			errMessage := fmt.Sprintf(
				"Fields %s are listed as IDs for type %s,"+
					"but we can have only one ID for any type."+
					"Pick a single field as the ID for type %s",
				fieldNamesString, typeVal.Name, typeVal.Name,
			)

			errs = append(errs, &gqlerror.Error{
				Message:   errMessage,
				Locations: errLocations,
			})
		}
	}

	return errs
}

func nameCheck(sch *ast.SchemaDocument) gqlerror.List {
	var errs []*gqlerror.Error

	for _, defn := range sch.Definitions {
		if isReservedKeyWord(defn.Name) {
			errs = append(errs, gqlerror.ErrorPosf(
				defn.Position,
				fmt.Sprintf(
					"%s is a reserved word, so you can't declare a type with this name."+
						"Pick a different name for the type. You also don't need to define the "+
						"Query or Mutation types - those are built automatically for you.",
					defn.Name,
				),
			))
		}
	}

	return errs
}

// [Posts]! -> invalid, [Posts!]! -> valid
func listValidityCheck(sch *ast.SchemaDocument) gqlerror.List {
	var errs []*gqlerror.Error

	for _, typ := range sch.Definitions {
		for _, field := range typ.Fields {
			if field.Type.Elem != nil && field.Type.NonNull && !field.Type.Elem.NonNull {
				errs = append(errs, gqlerror.ErrorPosf(
					field.Position,
					fmt.Sprintf(
						"[%s]! type of lists are invalid. Valid options are [%s!]! and [%s!].",
						field.Type.Name(), field.Type.Name(), field.Type.Name(),
					),
				))
			}
		}
	}

	return errs
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
