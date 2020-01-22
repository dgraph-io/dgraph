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
	"github.com/vektah/gqlparser/validator"
)

func init() {
	defnValidations = append(defnValidations, dataTypeCheck, nameCheck)

	typeValidations = append(typeValidations, idCountCheck, dgraphDirectiveTypeValidation)
	fieldValidations = append(fieldValidations, listValidityCheck, fieldArgumentCheck,
		fieldNameCheck, isValidFieldForList)

	validator.AddRule("Check variable type is correct", variableTypeCheck)
	validator.AddRule("Check for list type value", listTypeCheck)

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

func dgraphDirectiveTypeValidation(typ *ast.Definition) *gqlerror.Error {
	dir := typ.Directives.ForName(dgraphDirective)
	if dir == nil {
		return nil
	}

	typeArg := dir.Arguments.ForName(dgraphTypeArg)
	if typeArg == nil || typeArg.Value.Raw == "" {
		return gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; type argument for @dgraph directive should not be empty.", typ.Name)
	}
	if typeArg.Value.Kind != ast.StringValue {
		return gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; type argument for @dgraph directive should of type String.", typ.Name)
	}
	return nil
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

	if errMsg := isInverse(sch, typ.Name, field.Name, invTypeName, invField); errMsg != "" {
		return gqlerror.ErrorPosf(dir.Position, errMsg)
	}

	invDirective := invField.Directives.ForName(inverseDirective)
	if invDirective == nil {
		addDirective := func(fld *ast.FieldDefinition) {
			fld.Directives = append(fld.Directives, &ast.Directive{
				Name: inverseDirective,
				Arguments: []*ast.Argument{
					{
						Name: inverseArg,
						Value: &ast.Value{
							Raw:      field.Name,
							Position: dir.Position,
							Kind:     ast.EnumValue,
						},
					},
				},
				Position: dir.Position,
			})
		}

		addDirective(invField)

		// If it was an interface, we also need to copy the @hasInverse directive
		// to all implementing types
		if invType.Kind == ast.Interface {
			for _, t := range sch.Types {
				if implements(t, invType) {
					f := t.Fields.ForName(invFieldName)
					if f != nil {
						addDirective(f)
					}
				}
			}
		}
	}

	return nil
}

func implements(typ *ast.Definition, intfc *ast.Definition) bool {
	for _, t := range typ.Interfaces {
		if t == intfc.Name {
			return true
		}
	}
	return false
}

func isInverse(sch *ast.Schema, expectedInvType, expectedInvField, typeName string,
	field *ast.FieldDefinition) string {

	// We might have copied this directive in from an interface we are implementing.
	// If so, make the check for that interface.
	parentInt := parentInterface(sch, sch.Types[expectedInvType], expectedInvField)
	if parentInt != nil {
		fld := parentInt.Fields.ForName(expectedInvField)
		if fld.Directives != nil && fld.Directives.ForName(inverseDirective) != nil {
			expectedInvType = parentInt.Name
		}
	}

	invType := field.Type.Name()
	if invType != expectedInvType {
		return fmt.Sprintf(
			"Type %s; Field %s: @hasInverse is required to link the fields"+
				" of same type, but the field %s is of the type %s instead of"+
				" %[1]s. To link these make sure the fields are of the same type.",
			expectedInvType, expectedInvField, field.Name, field.Type,
		)
	}

	invDirective := field.Directives.ForName(inverseDirective)
	if invDirective == nil {
		return ""
	}

	invFieldArg := invDirective.Arguments.ForName("field")
	if invFieldArg == nil || invFieldArg.Value.Raw != expectedInvField {
		return fmt.Sprintf(
			"Type %s; Field %s: @hasInverse should be consistant."+
				" %[1]s.%[2]s is the inverse of %[3]s.%[4]s, but"+
				" %[3]s.%[4]s is the inverse of %[1]s.%[5]s.",
			expectedInvType, expectedInvField, typeName, field.Name,
			invFieldArg.Value.Raw,
		)
	}

	return ""
}

// validateSearchArg checks that the argument for search is valid and compatible
// with the type it is applied to.
func validateSearchArg(searchArg string,
	sch *ast.Schema,
	typ *ast.Definition,
	field *ast.FieldDefinition,
	dir *ast.Directive) *gqlerror.Error {

	isEnum := sch.Types[field.Type.Name()].Kind == ast.Enum
	search, ok := supportedSearches[searchArg]
	switch {
	case !ok:
		// This check can be removed once gqlparser bug
		// #107(https://github.com/vektah/gqlparser/issues/107) is fixed.
		return gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; Field %s: the argument to @search %s isn't valid."+
				"Fields of type %s %s.",
			typ.Name, field.Name, searchArg, field.Type.Name(), searchMessage(sch, field))

	case search.gqlType != field.Type.Name() && !isEnum:
		return gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; Field %s: has the @search directive but the argument %s "+
				"doesn't apply to field type %s.  Search by %[3]s applies to fields of type %[5]s. "+
				"Fields of type %[4]s %[6]s.",
			typ.Name, field.Name, searchArg, field.Type.Name(),
			supportedSearches[searchArg].gqlType, searchMessage(sch, field))

	case isEnum && !enumDirectives[searchArg]:
		return gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; Field %s: has the @search directive but the argument %s "+
				"doesn't apply to field type %s which is an Enum. Enum only supports "+
				"hash, exact, regexp and trigram",
			typ.Name, field.Name, searchArg, field.Type.Name())
	}

	return nil
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
		if err := validateSearchArg(searchArg, sch, typ, field, dir); err != nil {
			return err
		}

		// Checks that the filter indexes aren't repeated and they
		// don't clash with each other.
		searchIndex := builtInFilters[searchArg]
		if val, ok := searchIndexes[searchIndex]; ok {
			if field.Type.Name() == "String" || sch.Types[field.Type.Name()].Kind == ast.Enum {
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

func dgraphDirectiveValidation(sch *ast.Schema, typ *ast.Definition, field *ast.FieldDefinition,
	dir *ast.Directive) *gqlerror.Error {

	if isID(field) {
		return gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; Field %s: has the @dgraph directive but fields of type ID "+
				"can't have the @dgraph directive.", typ.Name, field.Name)
	}

	predArg := dir.Arguments.ForName(dgraphPredArg)
	if predArg == nil || predArg.Value.Raw == "" {
		return gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; Field %s: pred argument for @dgraph directive should not be empty.",
			typ.Name, field.Name,
		)
	}
	if predArg.Value.Kind != ast.StringValue {
		return gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; Field %s: pred argument for @dgraph directive should of type String.",
			typ.Name, field.Name,
		)
	}
	if strings.HasPrefix(predArg.Value.Raw, "~") || strings.HasPrefix(predArg.Value.Raw, "<~") {
		return gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; Field %s: reverse pred argument for @dgraph directive is not supported.",
			typ.Name, field.Name,
		)
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

	switch {
	case len(possibleSearchArgs) == 1 || sch.Types[field.Type.Name()].Kind == ast.Enum:
		return "are searchable by just @search"
	case len(possibleSearchArgs) == 0:
		return "can't have the @search directive"
	default:
		sort.Strings(possibleSearchArgs)
		return fmt.Sprintf(
			"can have @search by %s and %s",
			strings.Join(possibleSearchArgs[:len(possibleSearchArgs)-1], ", "),
			possibleSearchArgs[len(possibleSearchArgs)-1])
	}
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
