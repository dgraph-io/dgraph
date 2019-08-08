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
)

func init() {
	addPreRule("OnlyTypeEnumInInitialSchema", dataTypeCheck)
	addPreRule("TypeNameCantBeReservedKeyWords", nameCheck)

	addPostRule("OneIDPerType", idCountCheck)
	addPostRule("ValidListType", listValidityCheck)
	addPostRule("DirectivesValidation", directivesValidation)
}

func dataTypeCheck(sch *ast.SchemaDocument) gqlerror.List {
	var errs []*gqlerror.Error

	for _, typ := range sch.Definitions {
		if typ.Kind != ast.Object && typ.Kind != ast.Enum {
			errs = append(errs, gqlerror.ErrorPosf(
				typ.Position,
				"You can't add %s definitions. Only type and enums are allowed in initial schema.",
				strings.ToLower(string(typ.Kind)),
			))
		}
	}

	return errs
}

func nameCheck(sch *ast.SchemaDocument) gqlerror.List {
	var errs []*gqlerror.Error

	for _, defn := range sch.Definitions {
		if (defn.Kind == ast.Object || defn.Kind == ast.Enum) && isReservedKeyWord(defn.Name) {
			var errMesg string

			if defn.Name == "Query" || defn.Name == "Mutation" {
				errMesg = "You don't need to define the GraphQL Query or Mutation types." +
					" Those are built automatically for you."
			} else {
				errMesg = fmt.Sprintf(
					"%s is a reserved word, so you can't declare a type with this name. "+
						"Pick a different name for the type.", defn.Name,
				)
			}

			errs = append(errs, gqlerror.ErrorPosf(defn.Position, errMesg))
		}
	}

	return errs
}

func idCountCheck(sch *ast.Schema) gqlerror.List {
	var errs []*gqlerror.Error

	keys := make([]string, 0, len(sch.Types))
	for k := range sch.Types {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		typeVal := sch.Types[key]

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

			fieldNamesString := fmt.Sprintf(
				"%s and %s",
				strings.Join(fieldNames[:len(fieldNames)-1], ", "), fieldNames[len(fieldNames)-1],
			)
			errMessage := fmt.Sprintf(
				"Fields %s are listed as IDs for type %s, "+
					"but a type can have only one ID field. "+
					"Pick a single field as the ID for type %s.",
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

// [Posts]! -> invalid; [Posts!]!, [Posts!] -> valid
func listValidityCheck(sch *ast.Schema) gqlerror.List {
	var errs []*gqlerror.Error

	keys := make([]string, 0, len(sch.Types))
	for k := range sch.Types {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		typ := sch.Types[key]

		for _, field := range typ.Fields {
			if field.Type.Elem != nil && field.Type.NonNull && !field.Type.Elem.NonNull {
				errs = append(errs, gqlerror.ErrorPosf(
					field.Position,
					fmt.Sprintf(
						"[%s]! lists are invalid. Valid options are [%s!]! and [%s!].",
						field.Type.Name(), field.Type.Name(), field.Type.Name(),
					),
				))
			}
		}
	}

	return errs
}

func directivesValidation(sch *ast.Schema) gqlerror.List {
	var errs []*gqlerror.Error

	for _, dir := range supportedDirectives {
		errs = append(errs, dir.validationFunc(sch)...)
	}

	return errs
}

func hasInverseValidation(sch *ast.Schema) gqlerror.List {
	var errs []*gqlerror.Error

	keys := make([]string, 0, len(sch.Types))
	for k := range sch.Types {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		typ := sch.Types[key]

		for _, field := range typ.Fields {
			invDirective := field.Directives.ForName("hasInverse")

			if invDirective == nil {
				continue
			}

			if sch.Types[field.Type.Name()].Kind != ast.Object {
				errs = append(errs,
					gqlerror.ErrorPosf(
						invDirective.Position,
						"%s.%s is of type %s, but @hasInverse directive isn't allowed"+
							" on non object type field.", typ.Name, field.Name, field.Type.Name(),
					),
				)
				continue
			}

			fieldArg := invDirective.Arguments.ForName("field")
			if fieldArg == nil {
				errs = append(errs,
					gqlerror.ErrorPosf(
						invDirective.Position,
						"hasInverse directive at %s.%s doesn't have field argument.",
						typ.Name, field.Name,
					),
				)
				continue
			}

			invType := sch.Types[field.Type.Name()]

			invField := invType.Fields.ForName(fieldArg.Value.Raw)
			if invField == nil {
				errs = append(errs,
					gqlerror.ErrorPosf(
						invDirective.Position, "Inverse field for %s.%s doesn't exist.",
						typ.Name, field.Name,
					),
				)
				continue
			}

			if !isInverse(typ.Name, field.Name, invField) {
				errs = append(errs,
					gqlerror.ErrorPosf(
						invDirective.Position,
						"%s.%s have @hasInverse directive to %s.%s, "+
							"which doesn't point back to it.", typ.Name, field.Name,
						invType.Name, invField.Name,
					),
				)
			}
		}
	}

	return errs
}

func isScalar(s string) bool {
	_, ok := supportedScalars[s]
	return ok
}

func isReservedKeyWord(name string) bool {
	if isScalar(name) || name == "Query" || name == "Mutation" {
		return true
	}

	return false
}

func isInverse(expectedInvType, expectedInvField string, baseField *ast.FieldDefinition) bool {

	invDirective := baseField.Directives.ForName("hasInverse")
	if invDirective == nil {
		return false
	}

	invTypeArg := invDirective.Arguments.ForName("type")
	if invTypeArg == nil || invTypeArg.Value.Raw != expectedInvType {
		return false
	}

	invFieldArg := invDirective.Arguments.ForName("field")
	if invFieldArg == nil || invFieldArg.Value.Raw != expectedInvField {
		return false
	}

	return true
}
