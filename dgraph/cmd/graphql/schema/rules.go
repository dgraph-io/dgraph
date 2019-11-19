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
	fieldValidations = append(fieldValidations, listValidityCheck, fieldArgumentCheck,
		fieldNameCheck, isValidFieldForList)
}

func dataTypeCheck(defn *ast.Definition) *gqlerror.Error {
	if defn.Kind == ast.Object || defn.Kind == ast.Enum || defn.Kind == ast.Interface {
		return nil
	}
	return gqlerror.ErrorPosf(
		defn.Position,
		"You can't add %s definitions. "+
			"Only type, interface and enums are allowed in initial schema.",
		strings.ToLower(string(defn.Kind)),
	)
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

func collectFieldNames(idFields []*ast.FieldDefinition) (string, []gqlerror.Location) {
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
	return fieldNamesString, errLocations
}

func idCountCheck(typ *ast.Definition) *gqlerror.Error {
	var idFields []*ast.FieldDefinition
	var idDirectiveFields []*ast.FieldDefinition
	for _, field := range typ.Fields {
		if isIDField(typ, field) {
			idFields = append(idFields, field)
		}
		if d := field.Directives.ForName(idDirective); d != nil {
			idDirectiveFields = append(idDirectiveFields, field)
		}
	}

	if len(idFields) > 1 {
		fieldNamesString, errLocations := collectFieldNames(idFields)
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

	if len(idDirectiveFields) > 1 {
		fieldNamesString, errLocations := collectFieldNames(idDirectiveFields)
		errMessage := fmt.Sprintf(
			"Type %s: fields %s have the @id directive, "+
				"but a type can have only one field with @id. "+
				"Pick a single field with @id for type %s.",
			typ.Name, fieldNamesString, typ.Name,
		)

		return &gqlerror.Error{
			Message:   errMessage,
			Locations: errLocations,
		}
	}

	return nil
}

func isValidFieldForList(typ *ast.Definition, field *ast.FieldDefinition) *gqlerror.Error {
	if field.Type.Elem == nil && field.Type.NamedType != "" {
		return nil
	}

	// ID and Boolean list are not allowed.
	// [Boolean] is not allowed as dgraph schema doesn't support [bool] yet.
	switch field.Type.Elem.Name() {
	case
		"ID",
		"Boolean":
		return gqlerror.ErrorPosf(
			field.Position, "Type %s; Field %s: %s lists are invalid.",
			typ.Name, field.Name, field.Type.Elem.Name())
	}
	return nil
}

func fieldArgumentCheck(typ *ast.Definition, field *ast.FieldDefinition) *gqlerror.Error {
	if field.Arguments != nil {
		return gqlerror.ErrorPosf(
			field.Position,
			"Type %s; Field %s: You can't give arguments to fields.",
			typ.Name, field.Name,
		)
	}
	return nil
}

func fieldNameCheck(typ *ast.Definition, field *ast.FieldDefinition) *gqlerror.Error {
	//field name cannot be a reserved word
	if isReservedKeyWord(field.Name) {
		return gqlerror.ErrorPosf(
			field.Position, "Type %s; Field %s: %s is a reserved keyword and "+
				"you cannot declare a field with this name.",
			typ.Name, field.Name, field.Name)
	}

	return nil
}

func listValidityCheck(typ *ast.Definition, field *ast.FieldDefinition) *gqlerror.Error {
	if field.Type.Elem == nil && field.Type.NamedType != "" {
		return nil
	}

	// Nested lists are not allowed.
	if field.Type.Elem.Elem != nil {
		return gqlerror.ErrorPosf(field.Position,
			"Type %s; Field %s: Nested lists are invalid.",
			typ.Name, field.Name)
	}

	return nil
}

func hasInverseValidation(sch *ast.Schema, typ *ast.Definition,
	field *ast.FieldDefinition, dir *ast.Directive) *gqlerror.Error {

	invTypeName := field.Type.Name()
	if sch.Types[invTypeName].Kind != ast.Object && sch.Types[invTypeName].Kind != ast.Interface {
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

func searchValidation(
	sch *ast.Schema,
	typ *ast.Definition,
	field *ast.FieldDefinition,
	dir *ast.Directive) *gqlerror.Error {

	arg := dir.Arguments.ForName(searchArgs)
	if arg == nil {
		// If there's no arg, then it can be an enum or has to be a scalar that's
		// not ID. The schema generation will add the default search
		// for that type.
		if sch.Types[field.Type.Name()].Kind == ast.Enum ||
			(sch.Types[field.Type.Name()].Kind == ast.Scalar && !isIDField(typ, field)) {
			return nil
		}

		return gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; Field %s: has the @search directive but fields of type %s "+
				"can't have the @search directive.",
			typ.Name, field.Name, field.Type.Name())
	}

	// This check can be removed once gqlparser bug
	// #107(https://github.com/vektah/gqlparser/issues/107) is fixed.
	if arg.Value.Kind != ast.ListValue {
		return gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; Field %s: the @search directive requires a list argument, like @search(by: [hash])",
			typ.Name, field.Name)
	}

	searchArgs := getSearchArgs(field)
	searchIndexes := make(map[string]string)
	for _, searchArg := range searchArgs {
		// Checks that the argument for search is valid and compatible
		// with the type it is applied to.
		if search, ok := supportedSearches[searchArg]; !ok {
			// This check can be removed once gqlparser bug
			// #107(https://github.com/vektah/gqlparser/issues/107) is fixed.
			return gqlerror.ErrorPosf(
				dir.Position,
				"Type %s; Field %s: the argument to @search %s isn't valid."+
					"Fields of type %s %s.",
				typ.Name, field.Name, searchArg, field.Type.Name(), searchMessage(sch, field))

		} else if search.gqlType != field.Type.Name() {
			return gqlerror.ErrorPosf(
				dir.Position,
				"Type %s; Field %s: has the @search directive but the argument %s "+
					"doesn't apply to field type %s.  Search by %[3]s applies to fields of type %[5]s. "+
					"Fields of type %[4]s %[6]s.",
				typ.Name, field.Name, searchArg, field.Type.Name(),
				supportedSearches[searchArg].gqlType, searchMessage(sch, field))
		}

		// Checks that the filter indexes aren't repeated and they
		// don't clash with each other.
		searchIndex := builtInFilters[searchArg]
		if val, ok := searchIndexes[searchIndex]; ok {
			if field.Type.Name() == "String" {
				return gqlerror.ErrorPosf(
					dir.Position,
					"Type %s; Field %s: the argument to @search '%s' is the same "+
						"as the index '%s' provided before and shouldn't "+
						"be used together",
					typ.Name, field.Name, searchArg, val)
			}

			return gqlerror.ErrorPosf(
				dir.Position,
				"Type %s; Field %s: has the search directive on %s. %s "+
					"allows only one argument for @search.",
				typ.Name, field.Name, field.Type.Name(), field.Type.Name())
		}

		for _, index := range filtersCollisions[searchIndex] {
			if val, ok := searchIndexes[index]; ok {
				return gqlerror.ErrorPosf(
					dir.Position,
					"Type %s; Field %s: the arguments '%s' and '%s' can't "+
						"be used together as arguments to @search.",
					typ.Name, field.Name, searchArg, val)
			}
		}

		searchIndexes[searchIndex] = searchArg
	}

	return nil
}

func idValidation(sch *ast.Schema,
	typ *ast.Definition,
	field *ast.FieldDefinition,
	dir *ast.Directive) *gqlerror.Error {

	if field.Type.String() != "String!" {
		return gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; Field %s: with @id directive must be of type String!, not %s",
			typ.Name, field.Name, field.Type.String())
	}
	return nil
}

func searchMessage(sch *ast.Schema, field *ast.FieldDefinition) string {
	var possibleSearchArgs []string
	for name, typ := range supportedSearches {
		if typ.gqlType == field.Type.Name() {
			possibleSearchArgs = append(possibleSearchArgs, name)
		}
	}

	if len(possibleSearchArgs) == 1 || sch.Types[field.Type.Name()].Kind == ast.Enum {
		return "are searchable by just @search"
	} else if len(possibleSearchArgs) == 0 {
		return "can't have the @search directive"
	}

	sort.Strings(possibleSearchArgs)
	return fmt.Sprintf(
		"can have @search by %s and %s",
		strings.Join(possibleSearchArgs[:len(possibleSearchArgs)-1], ", "),
		possibleSearchArgs[len(possibleSearchArgs)-1])
}

func isScalar(s string) bool {
	_, ok := scalarToDgraph[s]
	return ok
}

func isReservedKeyWord(name string) bool {
	if isScalar(name) || name == "Query" || name == "Mutation" || name == "uid" {
		return true
	}

	return false
}
