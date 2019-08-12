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
	defnValidations = append(defnValidations, dataTypeCheck, nameCheck)

	typeValidations = append(typeValidations, idCountCheck)
	fieldValidations = append(fieldValidations, listValidityCheck)
}

func dataTypeCheck(defn *ast.Definition) *gqlerror.Error {

	if defn.Kind != ast.Object && defn.Kind != ast.Enum {
		return gqlerror.ErrorPosf(
			defn.Position,
			"You can't add %s definitions. Only type and enums are allowed in initial schema.",
			strings.ToLower(string(defn.Kind)),
		)
	}
	return nil
}

func nameCheck(defn *ast.Definition) *gqlerror.Error {

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

		return gqlerror.ErrorPosf(defn.Position, errMesg)
	}

	return nil
}

func idCountCheck(typ *ast.Definition) *gqlerror.Error {
	var idFields []*ast.FieldDefinition
	for _, field := range typ.Fields {
		if isIDField(typ, field) {
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
			fieldNamesString, typ.Name, typ.Name,
		)

		return &gqlerror.Error{
			Message:   errMessage,
			Locations: errLocations,
		}
	}

	return nil
}

// [Posts]! -> invalid; [Posts!]!, [Posts!] -> valid
func listValidityCheck(field *ast.FieldDefinition) *gqlerror.Error {

	if field.Type.Elem != nil && field.Type.NonNull && !field.Type.Elem.NonNull {
		return gqlerror.ErrorPosf(
			field.Position,
			fmt.Sprintf(
				"[%s]! lists are invalid. Valid options are [%s!]! and [%s!].",
				field.Type.Name(), field.Type.Name(), field.Type.Name(),
			),
		)
	}

	return nil
}

func hasInverseValidation(typ *ast.Definition, field *ast.FieldDefinition,
	dir *ast.Directive, sch *ast.Schema) *gqlerror.Error {

	invTypeName := field.Type.Name()
	if sch.Types[invTypeName].Kind != ast.Object {
		return gqlerror.ErrorPosf(
			field.Position,
			"%s.%s is of type %s, but @hasInverse directive isn't allowed"+
				" on non object type field.", typ.Name, field.Name, invTypeName,
		)
	}

	invFieldArg := dir.Arguments.ForName("field")
	if invFieldArg == nil {
		// This check can be removed once gqlparser bug
		// #107(https://github.com/vektah/gqlparser/issues/107) is fixed.
		return gqlerror.ErrorPosf(
			dir.Position,
			"hasInverse directive at %s.%s doesn't have field argument.",
			typ.Name, field.Name,
		)
	}

	invFieldName := invFieldArg.Value.Raw
	invType := sch.Types[invTypeName]
	invField := invType.Fields.ForName(invFieldName)
	if invField == nil {
		return gqlerror.ErrorPosf(
			dir.Position,
			"Unknown field %s.%s, inverse field of %s.%s(%[1]s.%[2]s) doesn't exist.",
			invTypeName, invFieldName, typ.Name, field.Name,
		)
	}

	if !isInverse(typ.Name, field.Name, invField) {
		return gqlerror.ErrorPosf(
			dir.Position,
			// @TODO: Error message should be more informative.
			"%s.%s have @hasInverse directive to %s.%s, which doesn't point back to it.",
			typ.Name, field.Name, invTypeName, invFieldName,
		)
	}

	return nil
}

func isInverse(expectedInvType, expectedInvField string, field *ast.FieldDefinition) bool {

	invType := field.Type.Name()
	if invType != expectedInvType {
		return false
	}

	invDirective := field.Directives.ForName("hasInverse")
	if invDirective == nil {
		return false
	}

	invFieldArg := invDirective.Arguments.ForName("field")
	if invFieldArg == nil || invFieldArg.Value.Raw != expectedInvField {
		return false
	}

	return true
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
