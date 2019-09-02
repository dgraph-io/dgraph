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

func hasInverseValidation(sch *ast.Schema, typ *ast.Definition,
	field *ast.FieldDefinition, dir *ast.Directive) *gqlerror.Error {

	invTypeName := field.Type.Name()
	if sch.Types[invTypeName].Kind != ast.Object {
		return gqlerror.ErrorPosf(
			field.Position,
			"Type %s; Field %s: Field %[2]s is of type %s, but @hasInverse directive only applies"+
				" to fields with object types.", typ.Name, field.Name, invTypeName,
		)
	}

	invFieldArg := dir.Arguments.ForName("field")
	if invFieldArg == nil {
		// This check can be removed once gqlparser bug
		// #107(https://github.com/vektah/gqlparser/issues/107) is fixed.
		return gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; Field %s: @hasInverse directive doesn't have field argument.",
			typ.Name, field.Name,
		)
	}

	invFieldName := invFieldArg.Value.Raw
	invType := sch.Types[invTypeName]
	invField := invType.Fields.ForName(invFieldName)
	if invField == nil {
		return gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; Field %s: inverse field %s doesn't exist for type %s.",
			typ.Name, field.Name, invFieldName, invTypeName,
		)
	}

	if !isInverse(typ.Name, field.Name, invField) {
		return gqlerror.ErrorPosf(
			dir.Position,
			// @TODO: Error message should be more informative.
			"Type %s; Field %s: @hasInverse is required in both the directions to link the fields"+
				", but field %s of type %s doesn't have @hasInverse directive pointing to field"+
				" %[2]s of type %[1]s. To link these add @hasInverse in both directions.",
			typ.Name, field.Name, invFieldName, invTypeName,
		)
	}

	return nil
}

func isInverse(expectedInvType, expectedInvField string, field *ast.FieldDefinition) bool {

	invType := field.Type.Name()
	if invType != expectedInvType {
		return false
	}

	invDirective := field.Directives.ForName(inverseDirective)
	if invDirective == nil {
		return false
	}

	invFieldArg := invDirective.Arguments.ForName("field")
	if invFieldArg == nil || invFieldArg.Value.Raw != expectedInvField {
		return false
	}

	return true
}

func searchableValidation(
	sch *ast.Schema,
	typ *ast.Definition,
	field *ast.FieldDefinition,
	dir *ast.Directive) *gqlerror.Error {

	arg := dir.Arguments.ForName(searchableArg)
	if arg == nil {
		// If there's no arg, then it can be an enum or has to be a scalar that's
		// not ID. The schema generation will add the default searchable
		// for that type.
		if sch.Types[field.Type.Name()].Kind == ast.Enum ||
			(sch.Types[field.Type.Name()].Kind == ast.Scalar && !isIDField(typ, field)) {
			return nil
		}

		return gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; Field %s: has the @searchable directive but fields of type %s "+
				"are not searchable.",
			typ.Name, field.Name, field.Type.Name())
	}

	if _, ok := supportedSearchables[arg.Value.Raw]; !ok {
		// This check can be removed once gqlparser bug
		// #107(https://github.com/vektah/gqlparser/issues/107) is fixed.
		return gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; Field %s: the argument to @searchable %s isn't valid."+
				"Fields of type %s are %s.",
			typ.Name, field.Name, arg.Value.Raw, field.Type.Name(), searchableMessage(sch, field))
	}

	if supportedSearchables[arg.Value.Raw] != field.Type.Name() {
		return gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; Field %s: has the @searchable directive but the argument %s "+
				"doesn't apply to field type %s.  Searchable %[3]s applies to fields of type %[5]s. "+
				"Fields of type %[4]s are %[6]s.",
			typ.Name, field.Name, arg.Value.Raw, field.Type.Name(),
			supportedSearchables[arg.Value.Raw], searchableMessage(sch, field))
	}

	return nil
}

func searchableMessage(sch *ast.Schema, field *ast.FieldDefinition) string {
	var possibleSearchables []string
	for name, typ := range supportedSearchables {
		if typ == field.Type.Name() {
			possibleSearchables = append(possibleSearchables, name)
		}
	}

	if len(possibleSearchables) == 1 || sch.Types[field.Type.Name()].Kind == ast.Enum {
		return "searchable by just @searchable"
	} else if len(possibleSearchables) == 0 {
		return "not searchable"
	}

	sort.Strings(possibleSearchables)
	return fmt.Sprintf(
		"searchable by %s and %s",
		strings.Join(possibleSearchables[:len(possibleSearchables)-1], ", "),
		possibleSearchables[len(possibleSearchables)-1])
}

func isScalar(s string) bool {
	_, ok := scalarToDgraph[s]
	return ok
}

func isReservedKeyWord(name string) bool {
	if isScalar(name) || name == "Query" || name == "Mutation" {
		return true
	}

	return false
}
