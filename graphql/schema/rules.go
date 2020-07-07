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
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/dgraph-io/dgraph/x"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"github.com/vektah/gqlparser/v2/parser"
	"github.com/vektah/gqlparser/v2/validator"
)

func init() {
	schemaDocValidations = append(schemaDocValidations, inputTypeNameValidation,
		customQueryNameValidation, customMutationNameValidation)
	defnValidations = append(defnValidations, dataTypeCheck, nameCheck)

	schemaValidations = append(schemaValidations, dgraphDirectivePredicateValidation)
	typeValidations = append(typeValidations, idCountCheck, dgraphDirectiveTypeValidation,
		passwordDirectiveValidation, conflictingDirectiveValidation, nonIdFieldsCheck,
		remoteTypeValidation)
	fieldValidations = append(fieldValidations, listValidityCheck, fieldArgumentCheck,
		fieldNameCheck, isValidFieldForList, hasAuthDirective)

	validator.AddRule("Check variable type is correct", variableTypeCheck)
	validator.AddRule("Check for list type value", listTypeCheck)

}

func dgraphDirectivePredicateValidation(gqlSch *ast.Schema, definitions []string) gqlerror.List {
	var errs []*gqlerror.Error

	type pred struct {
		name       string
		parentName string
		typ        string
		position   *ast.Position
		isId       bool
		isSecret   bool
	}

	preds := make(map[string]pred)
	interfacePreds := make(map[string]map[string]bool)

	secretError := func(secretPred, newPred pred) *gqlerror.Error {
		return gqlerror.ErrorPosf(newPred.position,
			"Type %s; Field %s: has the @dgraph predicate, but that conflicts with type %s "+
				"@secret directive on the same predicate. @secret predicates are stored encrypted"+
				" and so the same predicate can't be used as a %s.", newPred.parentName,
			newPred.name, secretPred.parentName, newPred.typ)
	}

	typeError := func(existingPred, newPred pred) *gqlerror.Error {
		return gqlerror.ErrorPosf(newPred.position,
			"Type %s; Field %s: has type %s, which is different to type %s; field %s, which has "+
				"the same @dgraph directive but type %s. These fields must have either the same "+
				"GraphQL types, or use different Dgraph predicates.", newPred.parentName,
			newPred.name, newPred.typ, existingPred.parentName, existingPred.name,
			existingPred.typ)
	}

	idError := func(idPred, newPred pred) *gqlerror.Error {
		return gqlerror.ErrorPosf(newPred.position,
			"Type %s; Field %s: doesn't have @id directive, which conflicts with type %s; field "+
				"%s, which has the same @dgraph directive along with @id directive. Both these "+
				"fields must either use @id directive, or use different Dgraph predicates.",
			newPred.parentName, newPred.name, idPred.parentName, idPred.name)
	}

	existingInterfaceFieldError := func(interfacePred, newPred pred) *gqlerror.Error {
		return gqlerror.ErrorPosf(newPred.position,
			"Type %s; Field %s: has the @dgraph directive, which conflicts with interface %s; "+
				"field %s, that this type implements. These fields must use different Dgraph "+
				"predicates.", newPred.parentName, newPred.name, interfacePred.parentName,
			interfacePred.name)
	}

	conflictingFieldsInImplementedInterfacesError := func(def *ast.Definition,
		interfaces []string, pred string) *gqlerror.Error {
		return gqlerror.ErrorPosf(def.Position,
			"Type %s; implements interfaces %v, all of which have fields with @dgraph predicate:"+
				" %s. These fields must use different Dgraph predicates.", def.Name, interfaces,
			pred)
	}

	checkExistingInterfaceFieldError := func(def *ast.Definition, existingPred, newPred pred) {
		for _, defName := range def.Interfaces {
			if existingPred.parentName == defName {
				errs = append(errs, existingInterfaceFieldError(existingPred, newPred))
			}
		}
	}

	checkConflictingDirectivesOnInterface := func(def *ast.Definition) {
		for _, directive := range def.Directives {
			if directive.Name == authDirective {
				errs = append(errs, gqlerror.ErrorPosf(def.Position,
					"Interface %s; @auth directive is not allowed on interfaces.", def.Name))
			}
		}
	}

	checkConflictingFieldsInImplementedInterfacesError := func(typ *ast.Definition) {
		fieldsToReport := make(map[string][]string)
		interfaces := typ.Interfaces

		for i := 0; i < len(interfaces); i++ {
			intr1 := interfaces[i]
			interfacePreds1 := interfacePreds[intr1]
			for j := i + 1; j < len(interfaces); j++ {
				intr2 := interfaces[j]
				for fname := range interfacePreds[intr2] {
					if interfacePreds1[fname] {
						if len(fieldsToReport[fname]) == 0 {
							fieldsToReport[fname] = append(fieldsToReport[fname], intr1)
						}
						fieldsToReport[fname] = append(fieldsToReport[fname], intr2)
					}
				}
			}
		}

		for fname, interfaces := range fieldsToReport {
			errs = append(errs, conflictingFieldsInImplementedInterfacesError(typ, interfaces,
				fname))
		}
	}

	// make sure all the interfaces are validated before validating any concrete types
	// this is required when validating that a type if implements two interfaces, then none of the
	// fields in those interfaces has the same dgraph predicate
	var interfaces, concreteTypes []string
	for _, def := range definitions {
		if gqlSch.Types[def].Kind == ast.Interface {
			interfaces = append(interfaces, def)
		} else {
			concreteTypes = append(concreteTypes, def)
		}
	}
	definitions = append(interfaces, concreteTypes...)

	for _, key := range definitions {
		def := gqlSch.Types[key]
		switch def.Kind {
		case ast.Object, ast.Interface:
			typName := typeName(def)
			if def.Kind == ast.Interface {
				interfacePreds[def.Name] = make(map[string]bool)
				checkConflictingDirectivesOnInterface(def)
			} else {
				checkConflictingFieldsInImplementedInterfacesError(def)
			}

			for _, f := range def.Fields {
				if f.Type.Name() == "ID" {
					continue
				}

				fname := fieldName(f, typName)
				// this field could have originally been defined in an interface that this type
				// implements. If we get a parent interface, that means this field gets validated
				// during the validation of that interface. So, no need to validate this field here.
				if parentInterface(gqlSch, def, f.Name) == nil {
					if def.Kind == ast.Interface {
						interfacePreds[def.Name][fname] = true
					}

					var prefix, suffix string
					if f.Type.Elem != nil {
						prefix = "["
						suffix = "]"
					}

					thisPred := pred{
						name:       f.Name,
						parentName: def.Name,
						typ:        fmt.Sprintf("%s%s%s", prefix, f.Type.Name(), suffix),
						position:   f.Position,
						isId:       f.Directives.ForName(idDirective) != nil,
						isSecret:   false,
					}

					if pred, ok := preds[fname]; ok {
						if pred.isSecret {
							errs = append(errs, secretError(pred, thisPred))
						} else if thisPred.typ != pred.typ {
							errs = append(errs, typeError(pred, thisPred))
						}
						if pred.isId != thisPred.isId {
							if pred.isId {
								errs = append(errs, idError(pred, thisPred))
							} else {
								errs = append(errs, idError(thisPred, pred))
							}
						}
						if def.Kind == ast.Object {
							checkExistingInterfaceFieldError(def, pred, thisPred)
						}
					} else {
						preds[fname] = thisPred
					}
				}
			}

			pwdField := getPasswordField(def)
			if pwdField != nil {
				fname := fieldName(pwdField, typName)
				if getDgraphDirPredArg(pwdField) != nil && parentInterfaceForPwdField(gqlSch, def,
					pwdField.Name) == nil {
					thisPred := pred{
						name:       pwdField.Name,
						parentName: def.Name,
						typ:        pwdField.Type.Name(),
						position:   pwdField.Position,
						isId:       false,
						isSecret:   true,
					}

					if pred, ok := preds[fname]; ok {
						if thisPred.typ != pred.typ || !pred.isSecret {
							errs = append(errs, secretError(thisPred, pred))
						}
						if def.Kind == ast.Object {
							checkExistingInterfaceFieldError(def, pred, thisPred)
						}
					} else {
						preds[fname] = thisPred
					}
				}
			}
		}
	}

	return errs
}

func inputTypeNameValidation(schema *ast.SchemaDocument) gqlerror.List {
	var errs []*gqlerror.Error
	forbiddenInputTypeNames := map[string]bool{
		// The types that we define in schemaExtras
		"DateTime":             true,
		"DgraphIndex":          true,
		"HTTPMethod":           true,
		"CustomHTTP":           true,
		"CustomGraphQL":        true,
		"IntFilter":            true,
		"FloatFilter":          true,
		"DateTimeFilter":       true,
		"StringTermFilter":     true,
		"StringRegExpFilter":   true,
		"StringFullTextFilter": true,
		"StringExactFilter":    true,
		"StringHashFilter":     true,
	}
	definedInputTypes := make([]*ast.Definition, 0)

	for _, defn := range schema.Definitions {
		defName := defn.Name
		if isQueryOrMutation(defName) {
			continue
		}
		if defn.Kind == ast.InputObject {
			definedInputTypes = append(definedInputTypes, defn)
			continue
		}
		if defn.Kind != ast.Object && defn.Kind != ast.Interface {
			continue
		}

		// types that are generated by us
		forbiddenInputTypeNames[defName+"Ref"] = true
		forbiddenInputTypeNames[defName+"Patch"] = true
		forbiddenInputTypeNames["Update"+defName+"Input"] = true
		forbiddenInputTypeNames["Update"+defName+"Payload"] = true
		forbiddenInputTypeNames["Delete"+defName+"Input"] = true

		if defn.Kind == ast.Object {
			forbiddenInputTypeNames["Add"+defName+"Input"] = true
			forbiddenInputTypeNames["Add"+defName+"Payload"] = true
		}

		forbiddenInputTypeNames[defName+"Filter"] = true
		forbiddenInputTypeNames[defName+"Order"] = true
		forbiddenInputTypeNames[defName+"Orderable"] = true
	}

	for _, inputType := range definedInputTypes {
		if forbiddenInputTypeNames[inputType.Name] {
			errs = append(errs, gqlerror.ErrorPosf(inputType.Position,
				"%s is a reserved word, so you can't declare an input type with this name. "+
					"Pick a different name for the input type.", inputType.Name))
		}
	}

	return errs
}

func customQueryNameValidation(schema *ast.SchemaDocument) gqlerror.List {
	var errs []*gqlerror.Error
	forbiddenNames := map[string]bool{}
	definedQueries := make([]*ast.FieldDefinition, 0)

	for _, defn := range schema.Definitions {
		defName := defn.Name
		if defName == "Query" {
			definedQueries = append(definedQueries, defn.Fields...)
			continue
		}
		if defn.Kind != ast.Object && defn.Kind != ast.Interface {
			continue
		}

		// forbid query names that are generated by us
		forbiddenNames["get"+defName] = true
		forbiddenNames["check"+defName+"Password"] = true
		forbiddenNames["query"+defName] = true
	}

	for _, qry := range definedQueries {
		if forbiddenNames[qry.Name] {
			errs = append(errs, gqlerror.ErrorPosf(qry.Position,
				"%s is a reserved word, so you can't declare a query with this name. "+
					"Pick a different name for the query.", qry.Name))
		}
	}

	return errs
}

func customMutationNameValidation(schema *ast.SchemaDocument) gqlerror.List {
	var errs []*gqlerror.Error
	forbiddenNames := map[string]bool{}
	definedMutations := make([]*ast.FieldDefinition, 0)

	for _, defn := range schema.Definitions {
		defName := defn.Name
		if defName == "Mutation" {
			definedMutations = append(definedMutations, defn.Fields...)
			continue
		}
		if defn.Kind != ast.Object && defn.Kind != ast.Interface {
			continue
		}

		// forbid mutation names that are generated by us
		switch defn.Kind {
		case ast.Interface:
			forbiddenNames["update"+defName] = true
			forbiddenNames["delete"+defName] = true
		case ast.Object:
			forbiddenNames["add"+defName] = true
			forbiddenNames["update"+defName] = true
			forbiddenNames["delete"+defName] = true
		}
	}

	for _, mut := range definedMutations {
		if forbiddenNames[mut.Name] {
			errs = append(errs, gqlerror.ErrorPosf(mut.Position,
				"%s is a reserved word, so you can't declare a mutation with this name. "+
					"Pick a different name for the mutation.", mut.Name))
		}
	}

	return errs
}

func dataTypeCheck(schema *ast.Schema, defn *ast.Definition) gqlerror.List {
	if defn.Kind == ast.Object || defn.Kind == ast.Enum || defn.Kind == ast.Interface || defn.
		Kind == ast.InputObject {
		return nil
	}
	return []*gqlerror.Error{gqlerror.ErrorPosf(
		defn.Position,
		"You can't add %s definitions. "+
			"Only type, interface, input and enums are allowed in initial schema.",
		strings.ToLower(string(defn.Kind)))}
}

func nameCheck(schema *ast.Schema, defn *ast.Definition) gqlerror.List {
	if (defn.Kind == ast.Object || defn.Kind == ast.Enum) && isReservedKeyWord(defn.Name) {
		var errMesg string

		if isQueryOrMutationType(defn) {
			for _, fld := range defn.Fields {
				// If we find any query or mutation field defined without a @custom directive, that
				// is an error for us.
				custom := fld.Directives.ForName(customDirective)
				if custom == nil {
					errMesg = "GraphQL Query and Mutation types are only allowed to have fields " +
						"with @custom directive. Other fields are built automatically for you. " +
						"Found " + defn.Name + " " + fld.Name + " without @custom."
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

		return []*gqlerror.Error{gqlerror.ErrorPosf(defn.Position, errMesg)}
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

func conflictingDirectiveValidation(schema *ast.Schema, typ *ast.Definition) gqlerror.List {
	var hasAuth, hasRemote bool
	for _, dir := range typ.Directives {
		if dir.Name == authDirective {
			hasAuth = true
		}
		if dir.Name == remoteDirective {
			hasRemote = true
		}
	}
	if hasAuth && hasRemote {
		return []*gqlerror.Error{gqlerror.ErrorPosf(typ.Position, `Type %s; cannot have both @%s and @%s directive`,
			typ.Name, authDirective, remoteDirective)}
	}
	return nil
}

func passwordDirectiveValidation(schema *ast.Schema, typ *ast.Definition) gqlerror.List {
	dirs := make([]string, 0)
	var errs []*gqlerror.Error

	for _, dir := range typ.Directives {
		if dir.Name != secretDirective {
			continue
		}
		val := dir.Arguments.ForName("field").Value.Raw
		if val == "" {
			errs = append(errs, gqlerror.ErrorPosf(typ.Position,
				`Type %s; Argument "field" of secret directive is empty`, typ.Name))
			return errs
		}
		dirs = append(dirs, val)
	}

	if len(dirs) > 1 {
		val := strings.Join(dirs, ",")
		errs = append(errs, gqlerror.ErrorPosf(typ.Position,
			"Type %s; has more than one secret fields %s", typ.Name, val))
		return errs
	}

	if len(dirs) == 0 {
		return nil
	}

	val := dirs[0]
	for _, f := range typ.Fields {
		if f.Name == val {
			errs = append(errs, gqlerror.ErrorPosf(typ.Position,
				"Type %s; has a secret directive and field of the same name %s",
				typ.Name, val))
			return errs
		}
	}

	return nil
}

func dgraphDirectiveTypeValidation(schema *ast.Schema, typ *ast.Definition) gqlerror.List {
	dir := typ.Directives.ForName(dgraphDirective)
	if dir == nil {
		return nil
	}

	typeArg := dir.Arguments.ForName(dgraphTypeArg)
	if typeArg == nil || typeArg.Value.Raw == "" {
		return []*gqlerror.Error{gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; type argument for @dgraph directive should not be empty.", typ.Name)}
	}
	if typeArg.Value.Kind != ast.StringValue {
		return []*gqlerror.Error{gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; type argument for @dgraph directive should of type String.", typ.Name)}
	}
	return nil
}

// A type should have other fields apart from fields of
// 1. Type ID!
// 2. Fields with @custom directive.
// to be a valid type. Otherwise its not possible to add objects of that type.
func nonIdFieldsCheck(schema *ast.Schema, typ *ast.Definition) gqlerror.List {
	if isQueryOrMutation(typ.Name) || typ.Kind == ast.Enum || typ.Kind == ast.Interface ||
		typ.Kind == ast.InputObject {
		return nil
	}

	// We don't generate mutations for remote types, so we skip this check for them.
	remote := typ.Directives.ForName(remoteDirective)
	if remote != nil {
		return nil
	}

	hasNonIdField := false
	for _, field := range typ.Fields {
		custom := field.Directives.ForName(customDirective)
		if isIDField(typ, field) || custom != nil {
			continue
		}
		hasNonIdField = true
		break
	}

	if !hasNonIdField {
		return []*gqlerror.Error{gqlerror.ErrorPosf(typ.Position, "Type %s; is invalid, a type must have atleast "+
			"one field that is not of ID! type and doesn't have @custom directive.", typ.Name)}
	}
	return nil
}

func remoteTypeValidation(schema *ast.Schema, typ *ast.Definition) gqlerror.List {
	if isQueryOrMutation(typ.Name) {
		return nil
	}
	remote := typ.Directives.ForName(remoteDirective)
	if remote == nil {
		for _, field := range typ.Fields {
			// If the field is being resolved through a custom directive, then we don't care if
			// the type for the field is a remote or a non-remote type.
			custom := field.Directives.ForName(customDirective)
			if custom != nil {
				continue
			}
			t := field.Type.Name()
			origTyp := schema.Types[t]
			remoteDir := origTyp.Directives.ForName(remoteDirective)
			if remoteDir != nil {
				return []*gqlerror.Error{gqlerror.ErrorPosf(field.Position, "Type %s; "+
					"field %s; is of a type that has @remote directive. Those would need to be "+
					"resolved by a @custom directive.", typ.Name, field.Name)}
			}
		}

		for _, implements := range typ.Interfaces {
			origTyp := schema.Types[implements]
			remoteDir := origTyp.Directives.ForName(remoteDirective)
			if remoteDir != nil {
				return []*gqlerror.Error{gqlerror.ErrorPosf(typ.Position, "Type %s; "+
					"without @remote directive can't implement an interface %s; with have "+
					"@remote directive.", typ.Name, implements)}
			}
		}
		return nil
	}

	// This means that the type was a remote type.
	for _, field := range typ.Fields {
		custom := field.Directives.ForName(customDirective)
		if custom != nil {
			return []*gqlerror.Error{gqlerror.ErrorPosf(field.Position, "Type %s; "+
				"field %s; can't have @custom directive as a @remote type can't have fields with"+
				" @custom directive.", typ.Name, field.Name)}
		}

	}

	for _, implements := range typ.Interfaces {
		origTyp := schema.Types[implements]
		remoteDir := origTyp.Directives.ForName(remoteDirective)
		if remoteDir == nil {
			return []*gqlerror.Error{gqlerror.ErrorPosf(typ.Position, "Type %s; "+
				"with @remote directive implements interface %s; which doesn't have @remote "+
				"directive.", typ.Name, implements)}
		}
	}

	return nil
}

func idCountCheck(schema *ast.Schema, typ *ast.Definition) gqlerror.List {
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

	var errs []*gqlerror.Error
	if len(idFields) > 1 {
		fieldNamesString, errLocations := collectFieldNames(idFields)
		errMessage := fmt.Sprintf(
			"Fields %s are listed as IDs for type %s, "+
				"but a type can have only one ID field. "+
				"Pick a single field as the ID for type %s.",
			fieldNamesString, typ.Name, typ.Name,
		)

		errs = append(errs, &gqlerror.Error{
			Message:   errMessage,
			Locations: errLocations,
		})
	}

	if len(idDirectiveFields) > 1 {
		fieldNamesString, errLocations := collectFieldNames(idDirectiveFields)
		errMessage := fmt.Sprintf(
			"Type %s: fields %s have the @id directive, "+
				"but a type can have only one field with @id. "+
				"Pick a single field with @id for type %s.",
			typ.Name, fieldNamesString, typ.Name,
		)

		errs = append(errs, &gqlerror.Error{
			Message:   errMessage,
			Locations: errLocations,
		})
	}

	return errs
}

func hasAuthDirective(typ *ast.Definition, field *ast.FieldDefinition) gqlerror.List {
	for _, directive := range field.Directives {
		if directive.Name != authDirective {
			continue
		}
		return []*gqlerror.Error{gqlerror.ErrorPosf(field.Position,
			"Type %s; Field %s: @%s directive is not allowed on fields",
			typ.Name, field.Name, authDirective)}
	}
	return nil
}

func isValidFieldForList(typ *ast.Definition, field *ast.FieldDefinition) gqlerror.List {
	if field.Type.Elem == nil && field.Type.NamedType != "" {
		return nil
	}

	// ID and Boolean list are not allowed.
	// [Boolean] is not allowed as dgraph schema doesn't support [bool] yet.
	switch field.Type.Elem.Name() {
	case
		"ID",
		"Boolean":
		return []*gqlerror.Error{gqlerror.ErrorPosf(
			field.Position, "Type %s; Field %s: %s lists are invalid.",
			typ.Name, field.Name, field.Type.Elem.Name())}
	}
	return nil
}

func fieldArgumentCheck(typ *ast.Definition, field *ast.FieldDefinition) gqlerror.List {
	if isQueryOrMutationType(typ) {
		return nil
	}

	// We don't need to verify the argument names for fields which are part of a remote type as
	// we don't add any of our own arguments to them.
	remote := typ.Directives.ForName(remoteDirective)
	if remote != nil {
		return nil
	}
	for _, arg := range field.Arguments {
		if isReservedArgument(arg.Name) {
			return []*gqlerror.Error{gqlerror.ErrorPosf(field.Position, "Type %s; Field %s:"+
				" can't have %s as an argument because it is a reserved argument.",
				typ.Name, field.Name, arg.Name)}
		}
	}
	return nil
}

func fieldNameCheck(typ *ast.Definition, field *ast.FieldDefinition) gqlerror.List {
	// field name cannot be a reserved word
	if isReservedKeyWord(field.Name) {
		return []*gqlerror.Error{gqlerror.ErrorPosf(
			field.Position, "Type %s; Field %s: %s is a reserved keyword and "+
				"you cannot declare a field with this name.",
			typ.Name, field.Name, field.Name)}
	}

	return nil
}

func listValidityCheck(typ *ast.Definition, field *ast.FieldDefinition) gqlerror.List {
	if field.Type.Elem == nil && field.Type.NamedType != "" {
		return nil
	}

	// Nested lists are not allowed.
	if field.Type.Elem.Elem != nil {
		return []*gqlerror.Error{gqlerror.ErrorPosf(field.Position,
			"Type %s; Field %s: Nested lists are invalid.",
			typ.Name, field.Name)}
	}

	return nil
}

func hasInverseValidation(sch *ast.Schema, typ *ast.Definition,
	field *ast.FieldDefinition, dir *ast.Directive,
	secrets map[string]x.SensitiveByteSlice) gqlerror.List {
	var errs []*gqlerror.Error

	invTypeName := field.Type.Name()
	if sch.Types[invTypeName].Kind != ast.Object && sch.Types[invTypeName].Kind != ast.Interface {
		errs = append(errs,
			gqlerror.ErrorPosf(
				field.Position,
				"Type %s; Field %s: Field %[2]s is of type %s, but @hasInverse directive only applies"+
					" to fields with object types.", typ.Name, field.Name, invTypeName))
		return errs
	}

	invFieldArg := dir.Arguments.ForName("field")
	if invFieldArg == nil {
		// This check can be removed once gqlparser bug
		// #107(https://github.com/vektah/gqlparser/issues/107) is fixed.
		errs = append(errs,
			gqlerror.ErrorPosf(
				dir.Position,
				"Type %s; Field %s: @hasInverse directive doesn't have field argument.",
				typ.Name, field.Name))
		return errs
	}

	invFieldName := invFieldArg.Value.Raw
	invType := sch.Types[invTypeName]
	invField := invType.Fields.ForName(invFieldName)
	if invField == nil {
		errs = append(errs,
			gqlerror.ErrorPosf(
				dir.Position,
				"Type %s; Field %s: inverse field %s doesn't exist for type %s.",
				typ.Name, field.Name, invFieldName, invTypeName))
		return errs
	}

	if errMsg := isInverse(sch, typ.Name, field.Name, invTypeName, invField); errMsg != "" {
		errs = append(errs, gqlerror.ErrorPosf(dir.Position, errMsg))
		return errs
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

func implements(typ, intfc *ast.Definition) bool {
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
	dir *ast.Directive,
	secrets map[string]x.SensitiveByteSlice) gqlerror.List {
	var errs []*gqlerror.Error

	arg := dir.Arguments.ForName(searchArgs)
	if arg == nil {
		// If there's no arg, then it can be an enum or has to be a scalar that's
		// not ID. The schema generation will add the default search
		// for that type.
		if sch.Types[field.Type.Name()].Kind == ast.Enum ||
			(sch.Types[field.Type.Name()].Kind == ast.Scalar && !isIDField(typ, field)) {
			return nil
		}

		errs = append(errs, gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; Field %s: has the @search directive but fields of type %s "+
				"can't have the @search directive.",
			typ.Name, field.Name, field.Type.Name()))
		return errs
	}

	// This check can be removed once gqlparser bug
	// #107(https://github.com/vektah/gqlparser/issues/107) is fixed.
	if arg.Value.Kind != ast.ListValue {
		errs = append(errs, gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; Field %s: the @search directive requires a list argument, like @search(by: [hash])",
			typ.Name, field.Name))
		return errs
	}

	searchArgs := getSearchArgs(field)
	searchIndexes := make(map[string]string)
	for _, searchArg := range searchArgs {
		if err := validateSearchArg(searchArg, sch, typ, field, dir); err != nil {
			errs = append(errs, err)
			return errs
		}

		// Checks that the filter indexes aren't repeated and they
		// don't clash with each other.
		searchIndex := builtInFilters[searchArg]
		if val, ok := searchIndexes[searchIndex]; ok {
			if field.Type.Name() == "String" || sch.Types[field.Type.Name()].Kind == ast.Enum {
				errs = append(errs, gqlerror.ErrorPosf(
					dir.Position,
					"Type %s; Field %s: the argument to @search '%s' is the same "+
						"as the index '%s' provided before and shouldn't "+
						"be used together",
					typ.Name, field.Name, searchArg, val))
				return errs
			}

			errs = append(errs, gqlerror.ErrorPosf(
				dir.Position,
				"Type %s; Field %s: has the search directive on %s. %s "+
					"allows only one argument for @search.",
				typ.Name, field.Name, field.Type.Name(), field.Type.Name()))
			return errs
		}

		for _, index := range filtersCollisions[searchIndex] {
			if val, ok := searchIndexes[index]; ok {
				errs = append(errs, gqlerror.ErrorPosf(
					dir.Position,
					"Type %s; Field %s: the arguments '%s' and '%s' can't "+
						"be used together as arguments to @search.",
					typ.Name, field.Name, searchArg, val))
				return errs
			}
		}

		searchIndexes[searchIndex] = searchArg
	}

	return errs
}

func dgraphDirectiveValidation(sch *ast.Schema, typ *ast.Definition, field *ast.FieldDefinition,
	dir *ast.Directive, secrets map[string]x.SensitiveByteSlice) gqlerror.List {
	var errs []*gqlerror.Error

	if isID(field) {
		errs = append(errs, gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; Field %s: has the @dgraph directive but fields of type ID "+
				"can't have the @dgraph directive.", typ.Name, field.Name))
		return errs
	}

	predArg := dir.Arguments.ForName(dgraphPredArg)
	if predArg == nil || predArg.Value.Raw == "" {
		errs = append(errs, gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; Field %s: pred argument for @dgraph directive should not be empty.",
			typ.Name, field.Name))
		return errs
	}
	if predArg.Value.Kind != ast.StringValue {
		errs = append(errs, gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; Field %s: pred argument for @dgraph directive should of type String.",
			typ.Name, field.Name))
		return errs
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
			errs = append(errs, gqlerror.ErrorPosf(
				dir.Position,
				"Type %s; Field %s: @hasInverse directive is not allowed when pred argument in "+
					"@dgraph directive starts with a ~.",
				typ.Name, field.Name))
			return errs
		}

		forwardEdgePred := strings.Trim(predArg.Value.Raw, "<~>")
		invTypeName := field.Type.Name()
		if sch.Types[invTypeName].Kind != ast.Object &&
			sch.Types[invTypeName].Kind != ast.Interface {
			errs = append(errs, gqlerror.ErrorPosf(
				field.Position,
				"Type %s; Field %s is of type %s, but reverse predicate in @dgraph"+
					" directive only applies to fields with object types.", typ.Name, field.Name,
				invTypeName))
			return errs
		}

		if field.Type.NamedType != "" {
			errs = append(errs, gqlerror.ErrorPosf(dir.Position,
				"Type %s; Field %s: with a dgraph directive that starts with ~ should be of type "+
					"list.", typ.Name, field.Name))
			return errs
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
					errs = append(errs, gqlerror.ErrorPosf(dir.Position, "Type %s; Field %s: should be of"+
						" type %s to be compatible with @dgraph reverse directive but is of"+
						" type %s.", invTypeName, fld.Name, typ.Name, fld.Type.Name()))
					return errs
				}
				invDirective := fld.Directives.ForName(inverseDirective)
				if invDirective != nil {
					errs = append(errs, gqlerror.ErrorPosf(
						dir.Position,
						"Type %s; Field %s: @hasInverse directive is not allowed is not allowed "+
							"because field is forward edge of another field with reverse directive.",
						invType.Name, fld.Name))
					return errs
				}
				forwardFound = true
				break
			}
		}
		if !forwardFound {
			errs = append(errs, gqlerror.ErrorPosf(
				dir.Position,
				"Type %s; Field %s: pred argument: %s is not supported as forward edge doesn't "+
					"exist for type %s.", typ.Name, field.Name, predArg.Value.Raw, invTypeName))
			return errs
		}
	}
	return nil
}

func passwordValidation(sch *ast.Schema,
	typ *ast.Definition,
	field *ast.FieldDefinition,
	dir *ast.Directive,
	secrets map[string]x.SensitiveByteSlice) gqlerror.List {

	return passwordDirectiveValidation(sch, typ)
}

func customDirectiveValidation(sch *ast.Schema,
	typ *ast.Definition,
	field *ast.FieldDefinition,
	dir *ast.Directive,
	secrets map[string]x.SensitiveByteSlice) gqlerror.List {
	var errs []*gqlerror.Error

	// 1. Validating custom directive itself
	search := field.Directives.ForName(searchDirective)
	if search != nil {
		errs = append(errs, gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; Field %s; custom directive not allowed along with @search directive.",
			typ.Name, field.Name))
	}

	dgraph := field.Directives.ForName(dgraphDirective)
	if dgraph != nil {
		errs = append(errs, gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; Field %s; custom directive not allowed along with @dgraph directive.",
			typ.Name, field.Name))
	}

	defn := sch.Types[typ.Name]
	id := getIDField(defn)
	xid := getXIDField(defn)
	if !isQueryOrMutationType(typ) {
		if len(id) == 0 && len(xid) == 0 {
			errs = append(errs, gqlerror.ErrorPosf(
				dir.Position,
				"Type %s; Field %s; @custom directive is only allowed on fields where the type"+
					" definition has a field with type ID! or a field with @id directive.",
				typ.Name, field.Name))
		}
	}

	// 2. Validating arguments to custom directive
	l := len(dir.Arguments)
	if l == 0 || l > 1 {
		errs = append(errs, gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; Field %s: has %d arguments for @custom directive, "+
				"it should contain exactly 1 argument.",
			typ.Name, field.Name, l))
	}

	// 3. Validating http argument
	httpArg := dir.Arguments.ForName("http")
	if httpArg == nil || httpArg.Value.String() == "" {
		errs = append(errs, gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; Field %s: http argument for @custom directive should not be empty.",
			typ.Name, field.Name))
		return errs
	}
	if httpArg.Value.Kind != ast.ObjectValue {
		errs = append(errs, gqlerror.ErrorPosf(
			httpArg.Position,
			"Type %s; Field %s: http argument for @custom directive should be of type Object.",
			typ.Name, field.Name))
	}

	// Start validating children of http argument

	// 4. Validating url
	httpUrl := httpArg.Value.Children.ForName("url")
	if httpUrl == nil {
		errs = append(errs, gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; Field %s; url field inside @custom directive is mandatory.", typ.Name,
			field.Name))
		return errs
	}
	parsedURL, err := url.ParseRequestURI(httpUrl.Raw)
	if err != nil {
		errs = append(errs, gqlerror.ErrorPosf(
			httpUrl.Position,
			"Type %s; Field %s; url field inside @custom directive is invalid.", typ.Name,
			field.Name))
		return errs
	}

	// collect all the url variables
	type urlVar struct {
		varName  string
		location string // path or query
	}
	elems := strings.Split(parsedURL.Path, "/")
	urlVars := make([]urlVar, 0)
	for _, elem := range elems {
		if strings.HasPrefix(elem, "$") {
			urlVars = append(urlVars, urlVar{varName: elem[1:], location: "path"})
		}
	}
	for _, valList := range parsedURL.Query() {
		for _, val := range valList {
			if strings.HasPrefix(val, "$") {
				urlVars = append(urlVars, urlVar{varName: val[1:], location: "query"})
			}
		}
	}
	// will be used later while validating graphql field for @custom
	urlHasParams := len(urlVars) > 0
	// check errors for url variables
	for _, v := range urlVars {
		if !isQueryOrMutationType(typ) {
			// For fields url variables come from the fields defined within the type. So we
			// check that they should be a valid field in the type definition.
			fd := defn.Fields.ForName(v.varName)
			if fd == nil {
				errs = append(errs, gqlerror.ErrorPosf(
					httpUrl.Position,
					"Type %s; Field %s; url %s inside @custom directive uses a field %s that is "+
						"not defined.", typ.Name, field.Name, v.location, v.varName))
				continue
			}
			if v.location == "path" && !fd.Type.NonNull {
				errs = append(errs, gqlerror.ErrorPosf(
					httpUrl.Position,
					"Type %s; Field %s; url %s inside @custom directive uses a field %s that "+
						"can be null.", typ.Name, field.Name, v.location, v.varName))
			}
		} else {
			arg := field.Arguments.ForName(v.varName)
			if arg == nil {
				errs = append(errs, gqlerror.ErrorPosf(
					httpUrl.Position,
					"Type %s; Field %s; url %s inside @custom directive uses an argument %s that "+
						"is not defined.", typ.Name, field.Name, v.location, v.varName))
				continue
			}
			if v.location == "path" && !arg.Type.NonNull {
				errs = append(errs, gqlerror.ErrorPosf(
					httpUrl.Position,
					"Type %s; Field %s; url %s inside @custom directive uses an argument %s"+
						" that can be null.", typ.Name, field.Name, v.location, v.varName))
			}
		}
	}

	// 5. Validating method
	method := httpArg.Value.Children.ForName("method")
	if method == nil {
		errs = append(errs, gqlerror.ErrorPosf(
			dir.Position,
			"Type %s; Field %s; method field inside @custom directive is mandatory.", typ.Name,
			field.Name))
	} else if !(method.Raw == "GET" || method.Raw == "POST" || method.Raw == "PUT" || method.
		Raw == "PATCH" || method.Raw == "DELETE") {
		errs = append(errs, gqlerror.ErrorPosf(
			method.Position,
			"Type %s; Field %s; method field inside @custom directive can only be GET/POST/PUT"+
				"/PATCH/DELETE.",
			typ.Name, field.Name))
	}

	// 6. Validating mode
	mode := httpArg.Value.Children.ForName(mode)
	var isBatchMode bool
	if mode != nil {
		if isQueryOrMutationType(typ) {
			errs = append(errs, gqlerror.ErrorPosf(
				mode.Position,
				"Type %s; Field %s; mode field inside @custom directive can't be "+
					"present on Query/Mutation.", typ.Name, field.Name))
		}

		op := mode.Raw
		if op != SINGLE && op != BATCH {
			errs = append(errs, gqlerror.ErrorPosf(
				mode.Position,
				"Type %s; Field %s; mode field inside @custom directive can only be "+
					"SINGLE/BATCH.", typ.Name, field.Name))
		}

		isBatchMode = op == BATCH
		if isBatchMode && urlHasParams {
			errs = append(errs, gqlerror.ErrorPosf(
				httpUrl.Position,
				"Type %s; Field %s; has parameters in url inside @custom directive while"+
					" mode is BATCH, url can't contain parameters if mode is BATCH.",
				typ.Name, field.Name))
		}
	}

	// 7. Validating graphql combination with url params, method and body
	body := httpArg.Value.Children.ForName("body")
	graphql := httpArg.Value.Children.ForName("graphql")
	if graphql != nil {
		if urlHasParams {
			errs = append(errs, gqlerror.ErrorPosf(dir.Position,
				"Type %s; Field %s; has parameters in url along with graphql field inside"+
					" @custom directive, url can't contain parameters if graphql field is present.",
				typ.Name, field.Name))
		}
		if method.Raw != "POST" {
			errs = append(errs, gqlerror.ErrorPosf(dir.Position,
				"Type %s; Field %s; has method %s while graphql field is also present inside"+
					" @custom directive, method can only be POST if graphql field is present.",
				typ.Name, field.Name, method.Raw))
		}
		if !isBatchMode {
			if body != nil {
				errs = append(errs, gqlerror.ErrorPosf(dir.Position,
					"Type %s; Field %s; has both body and graphql field inside @custom directive, "+
						"they can't be present together.",
					typ.Name, field.Name))
			}
		} else {
			if body == nil {
				errs = append(errs, gqlerror.ErrorPosf(dir.Position,
					"Type %s; Field %s; both body and graphql field inside @custom directive "+
						"are required if mode is BATCH.",
					typ.Name, field.Name))
			}
		}
	}

	// 8. Validating body
	var requiredFields map[string]bool
	if body != nil {
		_, requiredFields, err = parseBodyTemplate(body.Raw)
		if err != nil {
			errs = append(errs, gqlerror.ErrorPosf(body.Position,
				"Type %s; Field %s; body template inside @custom directive could not be parsed.",
				typ.Name, field.Name))
		}
		// Validating params to body template for Query/Mutation types. For other types the
		// validation is performed later along with graphql.
		if isQueryOrMutationType(typ) {
			for fname := range requiredFields {
				fd := field.Arguments.ForName(fname)
				if fd == nil {
					errs = append(errs, gqlerror.ErrorPosf(body.Position,
						"Type %s; Field %s; body template inside @custom directive uses an"+
							" argument %s that is not defined.", typ.Name, field.Name, fname))
				}
			}
		}
	}

	// 9. Validating graphql
	var graphqlOpDef *ast.OperationDefinition
	if graphql != nil {
		// TODO: we should actually construct *ast.Schema from remote introspection response, and
		// first validate that schema and then validate this graphql query against that schema
		// using:
		//		validator.Validate(schema *Schema, doc *QueryDocument)
		// This will help in keeping the custom validation code at a minimum. Lot of cases like:
		//		*	undefined variables being used in query,
		//		*	multiple args with same name at the same level in query, etc.
		// will get checked with the default validation itself.
		// Added an issue in gqlparser to allow building ast.Schema from Introspection response
		// similar to graphql-js utilities: https://github.com/vektah/gqlparser/issues/125
		// Once that is closed, we should be able to do this.
		queryDoc, gqlErr := parser.ParseQuery(&ast.Source{Input: graphql.Raw})
		if gqlErr != nil {
			errs = append(errs, gqlerror.ErrorPosf(graphql.Position,
				"Type %s; Field %s: unable to parse graphql in @custom directive because: %s",
				typ.Name, field.Name, gqlErr.Message))
			return errs
		}
		opCount := len(queryDoc.Operations)
		if opCount == 0 || opCount > 1 {
			errs = append(errs, gqlerror.ErrorPosf(graphql.Position,
				"Type %s; Field %s: inside graphql in @custom directive, found %d operations, "+
					"it can have exactly one operation.", typ.Name, field.Name, opCount))
			return errs
		}
		graphqlOpDef = queryDoc.Operations[0]
		if graphqlOpDef.Operation != "query" && graphqlOpDef.Operation != "mutation" {
			errs = append(errs, gqlerror.ErrorPosf(graphql.Position,
				"Type %s; Field %s: inside graphql in @custom directive, found `%s` operation, "+
					"it can only have query/mutation.", typ.Name, field.Name,
				graphqlOpDef.Operation))
		}
		if graphqlOpDef.Name != "" {
			errs = append(errs, gqlerror.ErrorPosf(graphql.Position,
				"Type %s; Field %s: inside graphql in @custom directive, found operation with "+
					"name `%s`, it can't have a name.", typ.Name, field.Name, graphqlOpDef.Name))
		}
		if graphqlOpDef.VariableDefinitions != nil {
			if isQueryOrMutationType(typ) {
				for _, vd := range graphqlOpDef.VariableDefinitions {
					ad := field.Arguments.ForName(vd.Variable)
					if ad == nil {
						errs = append(errs, gqlerror.ErrorPosf(graphql.Position,
							"Type %s; Field %s; @custom directive, graphql variables must use "+
								"fields defined within the type, found `%s`.", typ.Name,
							field.Name, vd.Variable))
					}
				}
			} else if !isBatchMode {
				// For BATCH mode we already verify that body should use fields defined inside the
				// parent type.
				requiredFields = make(map[string]bool)
				for _, vd := range graphqlOpDef.VariableDefinitions {
					requiredFields[vd.Variable] = true
				}
			}
		}
		if graphqlOpDef.Directives != nil {
			errs = append(errs, gqlerror.ErrorPosf(graphql.Position,
				"Type %s; Field %s: inside graphql in @custom directive, found operation with "+
					"directives, it can't have any directives.", typ.Name, field.Name))
		}
		opSelSetCount := len(graphqlOpDef.SelectionSet)
		if opSelSetCount == 0 || opSelSetCount > 1 {
			errs = append(errs, gqlerror.ErrorPosf(graphql.Position,
				"Type %s; Field %s: inside graphql in @custom directive, found %d fields inside "+
					"operation `%s`, it can have exactly one field.", typ.Name, field.Name,
				opSelSetCount, graphqlOpDef.Operation))
		}
		query := graphqlOpDef.SelectionSet[0].(*ast.Field)
		if query.Alias != query.Name {
			errs = append(errs, gqlerror.ErrorPosf(graphql.Position,
				"Type %s; Field %s: inside graphql in @custom directive, found %s `%s` with alias"+
					" `%s`, it can't have any alias.",
				typ.Name, field.Name, graphqlOpDef.Operation, query.Name, query.Alias))
		}
		// There can't be any ObjectDefinition as it is a query document; if there were, parser
		// would have given error. So not checking that query.ObjectDefinition is nil
		if query.Directives != nil {
			errs = append(errs, gqlerror.ErrorPosf(graphql.Position,
				"Type %s; Field %s: inside graphql in @custom directive, found %s `%s` with "+
					"directives, it can't have any directives.",
				typ.Name, field.Name, graphqlOpDef.Operation, query.Name))
		}
		if len(query.SelectionSet) != 0 {
			errs = append(errs, gqlerror.ErrorPosf(graphql.Position,
				"Type %s; Field %s: inside graphql in @custom directive, found %s `%s` with a "+
					"selection set, it can't have any selection set.",
				typ.Name, field.Name, graphqlOpDef.Operation, query.Name))
		}
		// Validate that argument values used within remote query are from variable definitions.
		if len(query.Arguments) > 0 {
			// validate the specific input requirements for BATCH mode
			if isBatchMode {
				if len(query.Arguments) != 1 || query.Arguments[0].Value.Kind != ast.Variable {
					errs = append(errs, gqlerror.ErrorPosf(graphql.Position,
						"Type %s; Field %s: inside graphql in @custom directive, for BATCH "+
							"mode, %s `%s` can have only one argument whose value should "+
							"be a variable.",
						typ.Name, field.Name, graphqlOpDef.Operation, query.Name))
					return errs
				}
				argVal := query.Arguments[0].Value.Raw
				vd := graphqlOpDef.VariableDefinitions.ForName(argVal)
				if vd == nil {
					errs = append(errs, gqlerror.ErrorPosf(graphql.Position,
						"Type %s; Field %s; @custom directive, graphql must use fields with "+
							"a variable definition, found `%s`.", typ.Name, field.Name, argVal))
				}
			} else {
				var bodyBuilder strings.Builder
				comma := ","
				bodyBuilder.WriteString("{")
				for i, arg := range query.Arguments {
					if i == len(query.Arguments)-1 {
						comma = ""
					}
					bodyBuilder.WriteString(arg.Name)
					bodyBuilder.WriteString(":")
					bodyBuilder.WriteString(arg.Value.String())
					bodyBuilder.WriteString(comma)
				}
				bodyBuilder.WriteString("}")
				_, requiredVars, err := parseBodyTemplate(bodyBuilder.String())
				if err != nil {
					errs = append(errs, gqlerror.ErrorPosf(graphql.Position,
						"Type %s; Field %s: inside graphql in @custom directive, "+
							"error in parsing arguments for %s `%s`: %s.", typ.Name, field.Name,
						graphqlOpDef.Operation, query.Name, err.Error()))
				}
				for varName := range requiredVars {
					vd := graphqlOpDef.VariableDefinitions.ForName(varName)
					if vd == nil {
						errs = append(errs, gqlerror.ErrorPosf(graphql.Position,
							"Type %s; Field %s; @custom directive, graphql must use fields with "+
								"a variable definition, found `%s`.", typ.Name, field.Name, varName))
					}
				}
			}
		}
	}

	// 10. Validating params to body/graphql template for fields in types other than Query/Mutation
	if !isQueryOrMutationType(typ) {
		var idField, xidField string
		if len(id) > 0 {
			idField = id[0].Name
		}
		if len(xid) > 0 {
			xidField = xid[0].Name
		}

		if field.Name == idField || field.Name == xidField {
			errs = append(errs, gqlerror.ErrorPosf(dir.Position,
				"Type %s; Field %s; custom directive not allowed on field of type ID! or field "+
					"with @id directive.", typ.Name, field.Name))
		}

		// TODO - We also need to have point no. 2 validation for custom queries/mutation.
		// Add that later.

		// 1. The required fields within the body/graphql template should contain an ID! field
		// or a field with @id directive as we use that to do de-duplication before resolving
		// these entities from the remote endpoint.
		// 2. All the required fields should be defined within this type.
		// 3. The required fields for a given field can't contain this field itself.
		// 4. All required fields should be of scalar type
		if body != nil || graphql != nil {
			var errPos *ast.Position
			var errIn string
			switch {
			case body != nil:
				errPos = body.Position
				errIn = "body template"
			case graphql != nil:
				errPos = graphql.Position
				errIn = "graphql"
			default:
				// this case is not possible, as requiredFields will have non-0 length only if there was
				// some body or graphql. Written only to satisfy logic flow, so that errPos is always
				// non-nil.
				errPos = dir.Position
				errIn = "@custom"
			}

			requiresID := false
			for fname := range requiredFields {
				if fname == field.Name {
					errs = append(errs, gqlerror.ErrorPosf(errPos,
						"Type %s; Field %s; @custom directive, %s can't require itself.",
						typ.Name, field.Name, errIn))
				}

				fd := typ.Fields.ForName(fname)
				if fd == nil {
					errs = append(errs, gqlerror.ErrorPosf(errPos,
						"Type %s; Field %s; @custom directive, %s must use fields defined "+
							"within the type, found `%s`.", typ.Name, field.Name, errIn, fname))
					continue
				}

				typName := fd.Type.Name()
				if !isScalar(typName) {
					errs = append(errs, gqlerror.ErrorPosf(errPos,
						"Type %s; Field %s; @custom directive, %s must use scalar fields, "+
							"found field `%s` of type `%s`.", typ.Name, field.Name, errIn,
						fname, typName))
				}

				if fd.Directives.ForName(customDirective) != nil {
					errs = append(errs, gqlerror.ErrorPosf(errPos,
						"Type %s; Field %s; @custom directive, %s can't use another field with "+
							"@custom directive, found field `%s` with @custom.", typ.Name,
						field.Name, errIn, fname))
				}

				if fname == idField || fname == xidField {
					requiresID = true
				}
			}
			if !requiresID {
				errs = append(errs, gqlerror.ErrorPosf(errPos,
					"Type %s; Field %s: @custom directive, %s must use a field with type "+
						"ID! or a field with @id directive.", typ.Name, field.Name, errIn))
			}
		}
	}

	// 12. Finally validate the given graphql operation on remote server, when all locally doable
	// validations have finished
	si := httpArg.Value.Children.ForName("skipIntrospection")
	var skip bool
	if si != nil {
		skip, err = strconv.ParseBool(si.Raw)
		if err != nil {
			errs = append(errs, gqlerror.ErrorPosf(graphql.Position,
				"Type %s; Field %s; skipIntrospection in @custom directive can only be "+
					"true/false, found: `%s`.",
				typ.Name, field.Name, si.Raw))
		}
	}

	forwardHeaders := httpArg.Value.Children.ForName("forwardHeaders")
	if forwardHeaders != nil {
		for _, h := range forwardHeaders.Children {
			key := strings.Split(h.Value.Raw, ":")
			if len(key) > 2 {
				return append(errs, gqlerror.ErrorPosf(graphql.Position,
					"Type %s; Field %s; forwardHeaders in @custom directive should be of the form 'remote_headername:local_headername' or just 'headername'"+
						", found: `%s`.",
					typ.Name, field.Name, h.Value.Raw))
			}
		}
	}

	secretHeaders := httpArg.Value.Children.ForName("secretHeaders")
	if secretHeaders != nil {
		for _, h := range secretHeaders.Children {
			key := strings.Split(h.Value.Raw, ":")
			if len(key) > 2 {
				return append(errs, gqlerror.ErrorPosf(graphql.Position,
					"Type %s; Field %s; secretHeaders in @custom directive should be of the form 'remote_headername:local_headername' or just 'headername'"+
						", found: `%s`.",
					typ.Name, field.Name, h.Value.Raw))
			}
		}
	}

	if errs != nil {
		return errs
	}

	if graphql != nil && !skip && graphqlOpDef != nil {
		secretHeaders := httpArg.Value.Children.ForName("secretHeaders")
		headers := http.Header{}
		if secretHeaders != nil {
			for _, h := range secretHeaders.Children {
				key := strings.Split(h.Value.Raw, ":")
				if len(key) == 1 {
					key = []string{h.Value.Raw, h.Value.Raw}
				}
				// We try and fetch the value from the stored secrets.
				val := secrets[key[1]]
				headers.Add(key[0], string(val))
			}
		}
		if err := validateRemoteGraphql(&remoteGraphqlMetadata{
			parentType:   typ,
			parentField:  field,
			graphqlOpDef: graphqlOpDef,
			isBatch:      isBatchMode,
			url:          httpUrl.Raw,
			headers:      headers,
			schema:       sch,
		}); err != nil {
			errs = append(errs, gqlerror.ErrorPosf(graphql.Position,
				"Type %s; Field %s: inside graphql in @custom directive, %s",
				typ.Name, field.Name, err.Error()))
		}
	}

	return errs
}

func idValidation(sch *ast.Schema,
	typ *ast.Definition,
	field *ast.FieldDefinition,
	dir *ast.Directive,
	secrets map[string]x.SensitiveByteSlice) gqlerror.List {
	if field.Type.String() == "String!" {
		return nil
	}
	return []*gqlerror.Error{gqlerror.ErrorPosf(
		dir.Position,
		"Type %s; Field %s: with @id directive must be of type String!, not %s",
		typ.Name, field.Name, field.Type.String())}
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

func isReservedArgument(name string) bool {
	switch name {
	case "first", "offset", "filter", "order":
		return true
	}
	return false
}

func isReservedKeyWord(name string) bool {
	if isScalar(name) || isQueryOrMutation(name) || name == "uid" {
		return true
	}

	return false
}

func isQueryOrMutationType(typ *ast.Definition) bool {
	return isQueryOrMutation(typ.Name)
}

func isQueryOrMutation(name string) bool {
	return name == "Query" || name == "Mutation"
}
