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
	"net/url"
	"sort"
	"strings"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"github.com/vektah/gqlparser/v2/validator"
)

func init() {
	defnValidations = append(defnValidations, dataTypeCheck, nameCheck)

	typeValidations = append(typeValidations, idCountCheck, dgraphDirectiveTypeValidation,
		passwordDirectiveValidation)
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
			for _, fld := range defn.Fields {
				// If we find any query or mutation field defined without a @custom directive, that
				// is an error for us.
				custom := fld.Directives.ForName("custom")
				if custom == nil {
					errMesg = "GraphQL Query and Mutation types are only allowed to have fields " +
						"with @custom directive. Other fields are built automatically for you."
					break
				}
			}
			if errMesg == "" {
				return nil
			}
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

func passwordDirectiveValidation(typ *ast.Definition) *gqlerror.Error {
	dirs := make([]string, 0)

	for _, dir := range typ.Directives {
		if dir.Name != "secret" {
			continue
		}
		val := dir.Arguments.ForName("field").Value.Raw
		if val == "" {
			return gqlerror.ErrorPosf(typ.Position,
				`Type %s; Argument "field" of secret directive is empty`, typ.Name)
		}
		dirs = append(dirs, val)
	}

	if len(dirs) > 1 {
		val := strings.Join(dirs, ",")
		return gqlerror.ErrorPosf(typ.Position,
			"Type %s; has more than one secret fields %s", typ.Name, val)
	}

	if len(dirs) == 0 {
		return nil
	}

	val := dirs[0]
	for _, f := range typ.Fields {
		if f.Name == val {
			return gqlerror.ErrorPosf(typ.Position,
				"Type %s; has a secret directive and field of the same name %s",
				typ.Name, val)
		}
	}

	return nil
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
	if typ.Name == "Query" || typ.Name == "Mutation" {
		return nil
	}
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
	// field name cannot be a reserved word
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
		if sch.Types[typ.Name].Kind == ast.Interface {
			// We don't want to consider the field of an interface but only the fields with
			// ~ in concrete types.
			return nil
		}
		// The inverse directive is not required on this field as given that the dgraph field name
		// starts with ~ we already know this field has to be a reverse edge of some other field.
		invDirective := field.Directives.ForName(inverseDirective)
		if invDirective != nil {
			return gqlerror.ErrorPosf(
				dir.Position,
				"Type %s; Field %s: @hasInverse directive is not allowed when pred argument in "+
					"@dgraph directive starts with a ~.",
				typ.Name, field.Name,
			)
		}

		forwardEdgePred := strings.Trim(predArg.Value.Raw, "<~>")
		invTypeName := field.Type.Name()
		if sch.Types[invTypeName].Kind != ast.Object &&
			sch.Types[invTypeName].Kind != ast.Interface {
			return gqlerror.ErrorPosf(
				field.Position,
				"Type %s; Field %s is of type %s, but reverse predicate in @dgraph"+
					" directive only applies to fields with object types.", typ.Name, field.Name,
				invTypeName)
		}

		if field.Type.NamedType != "" {
			return gqlerror.ErrorPosf(dir.Position,
				"Type %s; Field %s: with a dgraph directive that starts with ~ should be of type "+
					"list.", typ.Name, field.Name)
		}

		invType := sch.Types[invTypeName]
		forwardFound := false
		// We need to loop through all the fields of the invType and see if we find a field which
		// is a forward edge field for this reverse field.
		for _, fld := range invType.Fields {
			dir := fld.Directives.ForName(dgraphDirective)
			if dir == nil {
				continue
			}
			predArg := dir.Arguments.ForName(dgraphPredArg)
			if predArg == nil || predArg.Value.Raw == "" {
				continue
			}
			if predArg.Value.Raw == forwardEdgePred {
				if fld.Type.Name() != typ.Name {
					return gqlerror.ErrorPosf(dir.Position, "Type %s; Field %s: should be of"+
						" type %s to be compatible with @dgraph reverse directive but is of"+
						" type %s.", invTypeName, fld.Name, typ.Name, fld.Type.Name())
				}
				invDirective := fld.Directives.ForName(inverseDirective)
				if invDirective != nil {
					return gqlerror.ErrorPosf(
						dir.Position,
						"Type %s; Field %s: @hasInverse directive is not allowed is not allowed "+
							"because field is forward edge of another field with reverse directive.",
						invType.Name, fld.Name,
					)
				}
				forwardFound = true
				break
			}
		}
		if !forwardFound {
			return gqlerror.ErrorPosf(
				dir.Position,
				"Type %s; Field %s: pred argument: %s is not supported as forward edge doesn't "+
					"exist for type %s.", typ.Name, field.Name, predArg.Value.Raw, invTypeName,
			)
		}
	}
	return nil
}

func passwordValidation(sch *ast.Schema,
	typ *ast.Definition,
	field *ast.FieldDefinition,
	dir *ast.Directive) *gqlerror.Error {

	return passwordDirectiveValidation(typ)
}

func customDirectiveValidation(sch *ast.Schema,
	typ *ast.Definition,
	field *ast.FieldDefinition,
	dir *ast.Directive) *gqlerror.Error {
	arg := dir.Arguments.ForName("http")
	if arg == nil || arg.Value.String() == "" {
		return gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; Field %s: http argument for @custom directive should not be empty.",
			typ.Name, field.Name,
		)
	}

	if arg.Value.Kind != ast.ObjectValue {
		return gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; Field %s: http argument for @custom directive should of type Object.",
			typ.Name, field.Name,
		)
	}
	u := arg.Value.Children.ForName("url")
	if u == nil {
		return gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; Field %s; url field inside @custom directive is mandatory.", typ.Name,
			field.Name)
	}
	if _, err := url.ParseRequestURI(u.Raw); err != nil {
		return gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; Field %s; url field inside @custom directive is invalid.", typ.Name,
			field.Name)
	}

	method := arg.Value.Children.ForName("method")
	if method == nil {
		return gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; Field %s; method field inside @custom directive is mandatory.", typ.Name,
			field.Name)
	}

	if method.Raw != "GET" && method.Raw != "POST" {
		return gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; Field %s; method field inside @custom directive can only be GET/POST.",
			typ.Name, field.Name)
	}

	graphqlArg := dir.Arguments.ForName("graphql")
	if graphqlArg != nil {
		// This is remote graphql so validate remote graphql end point.
		return validateRemoteGraphqlCall(&remoteGraphqlEndpoint{
			graphqlArg: graphqlArg,
			schema:     sch,
			field:      field,
			directive:  dir,
			rootQuery:  typ,
			url:        u.Raw,
		})
	}
	return nil

	// Validate for graphql
	remoteMethod, err := parseGraphqlMethod(method.Raw)
	if err != nil {
		return gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; Field %s; %s", typ.Name, field.Name, err.Error(),
		)
	}

	// Check whether we have remote arugment value in our schema
	for _, remoteArgValue := range remoteMethod.args {
		localArg := field.Arguments.ForName(remoteArgValue[1:])
		if localArg == nil {
			return gqlerror.ErrorPosf(
				dir.Position,
				"Type %s; Field %s; %s argument is not defined in the local query",
				typ.Name, field.Name, remoteArgValue,
			)
		}
	}

	// Now get the remote schema to check whether query exist or not.
	remoteSchema, err := introspectRemoteSchema(u.Raw)
	if err != nil {
		return gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; Field %s; unable to introspect remote schema(%s): %s",
			typ.Name, field.Name, u.Raw, err.Error(),
		)
	}

	validQuery := false
	remoteInputTypeNames := []struct {
		name     string
		kind     string
		typeName string
	}{}

	var remoteQuery GqlField

	for _, remoteType := range remoteSchema.Data.Schema.Types {
		if remoteType.Name != "Query" && remoteType.Kind != "OBJECT" {
			continue
		}
		for _, query := range remoteType.Fields {
			if query.Name != remoteMethod.name {
				continue
			}

			if len(query.Args) != len(remoteMethod.args) {
				// Invalid number of arguments
				return gqlerror.ErrorPosf(
					dir.Position,
					"Type %s; Field %s; invalid number of argument for the remote query %s",
					typ.Name, field.Name, remoteMethod.name,
				)
			}

			for _, arg := range query.Args {
				if _, ok := remoteMethod.args[arg.Name]; !ok {
					return gqlerror.ErrorPosf(
						dir.Position,
						"Type %s; Field %s;%s Argument value is not present for",
						typ.Name, field.Name, arg.Name,
					)
				}
				remoteInputTypeNames = append(remoteInputTypeNames, struct {
					name     string
					kind     string
					typeName string
				}{name: arg.Name, kind: arg.Type.Kind, typeName: arg.Type.Name})
			}
			validQuery = true
			remoteQuery = query
			break
		}
		break
	}

	if !validQuery {
		return gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; Field %s; remote method %s is not present in the remote schema",
			typ.Name, field.Name, remoteMethod.name,
		)
	}

	customRemoteTypes := make(map[string]interface{})
	// Let's get all the type for the input.
	for _, inputArg := range remoteInputTypeNames {
		if _, ok := graphqlScalarType[inputArg.typeName]; ok {
			continue
		}
		// We need to check only custom type.
		customRemoteTypes[inputArg.typeName] = 0
	}

	// Add return type as well.
	if _, ok := graphqlScalarType[remoteQuery.Type.Name]; !ok {
		customRemoteTypes[remoteQuery.Type.Name] = 0
	}

	// Input type may have embedded type in it. So, we have to expand the type into flat structure
	// to avoid recursion check.
	// Eg:
	// type Continent {
	//   code: String
	//   name: String
	//   countries: [Country]
	// }
	// Expanded as
	// Continent, Country.
	remoteTypes := make(map[string]Types)

	// THIS need recursion check embedded type may have embedded type.
	for _, remoteType := range remoteSchema.Data.Schema.Types {
		if _, ok := customRemoteTypes[remoteType.Name]; !ok {
			remoteTypes[remoteType.Name] = remoteType
			// Expand remote type if necessary.

		}
	}

	// fmt.Println(string(body))

	// // Local inrospection.
	// doc, gqlErr := parser.ParseQuery(&ast.Source{Input: introspectionQuery})

	// if gqlErr != nil {
	// 	return gqlErr
	// }
	// op := doc.Operations.ForName("")
	// oper := &operation{op: op,
	// 	vars:     map[string]interface{}{},
	// 	query:    introspectionQuery,
	// 	doc:      doc,
	// 	inSchema: &schema{schema: sch},
	// }

	// listErr := validator.Validate(sch, doc)
	// if err != nil {
	// 	panic(listErr)
	// }
	// queries := oper.Queries()
	// localIntrospection, err := Introspect(queries[0])
	// if err != nil {
	// 	return gqlerror.Errorf(err.Error())
	// }

	// fmt.Println("local: ", string(localIntrospection))
	return nil
}

func expandTypes(schema IntrospectedSchema,
	typeNames map[string]interface{},
	remoteTypes map[string]*Types) error {

	for typeName := range typeNames {
		if _, ok := remoteTypes[typeName]; ok {
			// We already retrived this type So, keep moving.
			continue
		}
		for _, remoteType := range schema.Data.Schema.Types {
			if remoteType.Name != typeName {
				continue
			}
			remoteTypes[remoteType.Name] = &remoteType
			// Expand fileds.
			for _, field := range remoteType.Fields {
				if _, ok := graphqlScalarType[field.Type.Name]; !ok {
					// scalar type so continue.
					continue
				}
			}
		}
	}

	return nil
}

type graphqlMethod struct {
	name string
	args map[string]string
}

func parseGraphqlMethod(method string) (*graphqlMethod, error) {
	position := strings.Index(method, "(")
	if position < 0 {
		return nil, fmt.Errorf("method field inside @custom directive is invalid")
	}
	name := method[:position]

	// Consume (
	position++
	isPositionValid := func() error {
		if position > len(method) {
			return fmt.Errorf("method field inside @custom directive is invalid")
		}
		return nil
	}

	if err := isPositionValid(); err != nil {
		return nil, err
	}
	arg := make(map[string]string)

	for {
		colonIndex := strings.Index(method[position:], ":")
		if colonIndex < 0 {
			return nil, fmt.Errorf("method field inside @custom directive is invalid")
		}
		fieldName := strings.TrimSpace(method[position : position+colonIndex])

		position += colonIndex
		// Consume :
		position++
		if err := isPositionValid(); err != nil {
			return nil, err
		}

		commaIndex := strings.Index(method[position:], ",")
		if commaIndex < 0 {
			// No more argument so check for closing brace.
			braceIndex := strings.Index(method[position:], ")")
			if braceIndex < 0 {
				return nil, fmt.Errorf("method field inside @custom directive is invalid")
			}
			argVal := strings.TrimSpace(method[position : position+braceIndex])
			if argVal == "" || len(argVal) == 1 || argVal[0] != '$' {
				return nil, fmt.Errorf("method field inside @custom directive is invalid")
			}
			arg[fieldName] = argVal
			break
		}

		argVal := strings.TrimSpace(method[position : position+commaIndex])
		if argVal == "" || len(argVal) == 1 || argVal[0] != '$' {
			return nil, fmt.Errorf("method field inside @custom directive is invalid")
		}
		arg[fieldName] = argVal

		position += commaIndex
		// Consume ,
		position++
		if err := isPositionValid(); err != nil {
			return nil, err
		}
	}
	return &graphqlMethod{name: name, args: arg}, nil
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
