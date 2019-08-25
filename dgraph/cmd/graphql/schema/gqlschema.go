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

var defnValidations, typeValidations []func(defn *ast.Definition) *gqlerror.Error
var fieldValidations []func(field *ast.FieldDefinition) *gqlerror.Error

type scalar struct {
	name       string
	dgraphType string
}

type directive struct {
	directiveDefn  *ast.DirectiveDefinition
	validationFunc func(sch *ast.Schema, typ *ast.Definition,
		field *ast.FieldDefinition, dir *ast.Directive) *gqlerror.Error
}

var supportedScalars = map[string]scalar{
	"ID":       scalar{name: "ID", dgraphType: "uid"},
	"Boolean":  scalar{name: "Boolean", dgraphType: "bool"},
	"Int":      scalar{name: "Int", dgraphType: "int"},
	"Float":    scalar{name: "Float", dgraphType: "float"},
	"String":   scalar{name: "String", dgraphType: "string"},
	"DateTime": scalar{name: "DateTime", dgraphType: "dateTime"},
}

var supportedDirectives = map[string]directive{
	"hasInverse": directive{
		directiveDefn: &ast.DirectiveDefinition{
			Name:      "hasInverse",
			Locations: []ast.DirectiveLocation{ast.LocationFieldDefinition},
			Arguments: []*ast.ArgumentDefinition{&ast.ArgumentDefinition{
				Name: "field",
				Type: &ast.Type{NamedType: "String", NonNull: true},
			}},
		},
		validationFunc: hasInverseValidation,
	},
}

// addScalars adds all the supported scalars in the schema.
func addScalars(doc *ast.SchemaDocument) {
	for _, s := range supportedScalars {
		doc.Definitions = append(
			doc.Definitions,
			// Empty Position because it is being inserted by the engine.
			&ast.Definition{Kind: ast.Scalar, Name: s.name, Position: &ast.Position{}},
		)
	}
}

func addDirectives(doc *ast.SchemaDocument) {
	for _, d := range supportedDirectives {
		doc.Directives = append(doc.Directives, d.directiveDefn)
	}
}

// preGQLValidation validates schema before gql validation
func preGQLValidation(schema *ast.SchemaDocument) gqlerror.List {
	var errs []*gqlerror.Error

	for _, defn := range schema.Definitions {
		errs = append(errs, applyDefnValidations(defn, defnValidations)...)
	}

	return errs
}

// postGQLValidation validates schema after gql validation.
func postGQLValidation(schema *ast.Schema, definitions []string) gqlerror.List {
	var errs []*gqlerror.Error

	for _, defn := range definitions {
		typ := schema.Types[defn]

		errs = append(errs, applyDefnValidations(typ, typeValidations)...)

		for _, field := range typ.Fields {
			errs = append(errs, applyFieldValidations(field)...)

			for _, dir := range field.Directives {
				errs = appendIfNotNull(errs,
					supportedDirectives[dir.Name].validationFunc(schema, typ, field, dir))
			}
		}
	}

	return errs
}

func applyDefnValidations(defn *ast.Definition,
	rules []func(defn *ast.Definition) *gqlerror.Error) gqlerror.List {
	var errs []*gqlerror.Error

	for _, rule := range rules {
		errs = appendIfNotNull(errs, rule(defn))
	}

	return errs
}

func applyFieldValidations(field *ast.FieldDefinition) gqlerror.List {
	var errs []*gqlerror.Error

	for _, rule := range fieldValidations {
		errs = appendIfNotNull(errs, rule(field))
	}

	return errs
}

// generateCompleteSchema generates all the required query/mutation/update functions
// for all the types mentioned the the schema.
func generateCompleteSchema(sch *ast.Schema, definitions []string) {

	sch.Query = &ast.Definition{
		Kind:        ast.Object,
		Description: "Query object contains all the query functions",
		Name:        "Query",
		Fields:      make([]*ast.FieldDefinition, 0),
	}

	sch.Mutation = &ast.Definition{
		Kind:        ast.Object,
		Description: "Mutation object contains all the mutation functions",
		Name:        "Mutation",
		Fields:      make([]*ast.FieldDefinition, 0),
	}

	for _, key := range definitions {
		defn := sch.Types[key]

		if defn.Kind == ast.Object {
			// types and inputs needed for mutations
			addInputType(sch, defn)
			addReferenceType(sch, defn)
			addPatchType(sch, defn)
			addUpdateType(sch, defn)
			addAddPayloadType(sch, defn)
			addUpdatePayloadType(sch, defn)
			addDeletePayloadType(sch, defn)
			addMutations(sch, defn)

			// types and inputs needed for query and search
			addFilterType(sch, defn)
			addQueries(sch, defn)
		}
	}
}

func addInputType(schema *ast.Schema, defn *ast.Definition) {
	schema.Types[defn.Name+"Input"] = &ast.Definition{
		Kind:   ast.InputObject,
		Name:   defn.Name + "Input",
		Fields: getNonIDFields(schema, defn),
	}
}

func addReferenceType(schema *ast.Schema, defn *ast.Definition) {
	schema.Types[defn.Name+"Ref"] = &ast.Definition{
		Kind:   ast.InputObject,
		Name:   defn.Name + "Ref",
		Fields: getIDField(defn),
	}
}

func addUpdateType(schema *ast.Schema, defn *ast.Definition) {
	updType := &ast.Definition{
		Kind: ast.InputObject,
		Name: "Update" + defn.Name + "Input",
		Fields: append(
			getIDField(defn),
			&ast.FieldDefinition{
				Name: "patch",
				Type: &ast.Type{
					NamedType: "Patch" + defn.Name,
					NonNull:   true,
				},
			}),
	}
	schema.Types["Update"+defn.Name+"Input"] = updType
}

func addPatchType(schema *ast.Schema, defn *ast.Definition) {
	patchDefn := &ast.Definition{
		Kind:   ast.InputObject,
		Name:   "Patch" + defn.Name,
		Fields: getNonIDFields(schema, defn),
	}
	schema.Types["Patch"+defn.Name] = patchDefn

	for _, fld := range patchDefn.Fields {
		fld.Type.NonNull = false
	}
}

func addFilterType(schema *ast.Schema, defn *ast.Definition) {
	schema.Types[defn.Name+"Filter"] = &ast.Definition{
		Kind:   ast.InputObject,
		Name:   defn.Name + "Filter",
		Fields: getFilterField(),
	}
}

func addAddPayloadType(schema *ast.Schema, defn *ast.Definition) {
	schema.Types["Add"+defn.Name+"Payload"] = &ast.Definition{
		Kind: ast.Object,
		Name: "Add" + defn.Name + "Payload",
		Fields: []*ast.FieldDefinition{
			&ast.FieldDefinition{
				Name: strings.ToLower(defn.Name),
				Type: &ast.Type{
					NamedType: defn.Name,
				},
			},
		},
	}
}

func addUpdatePayloadType(schema *ast.Schema, defn *ast.Definition) {
	schema.Types["Update"+defn.Name+"Payload"] = &ast.Definition{
		Kind: ast.Object,
		Name: "Update" + defn.Name + "Payload",
		Fields: []*ast.FieldDefinition{
			&ast.FieldDefinition{
				Name: strings.ToLower(defn.Name),
				Type: &ast.Type{
					NamedType: defn.Name,
				},
			},
		},
	}
}

func addDeletePayloadType(schema *ast.Schema, defn *ast.Definition) {
	schema.Types["Delete"+defn.Name+"Payload"] = &ast.Definition{
		Kind: ast.Object,
		Name: "Delete" + defn.Name + "Payload",
		Fields: []*ast.FieldDefinition{
			&ast.FieldDefinition{
				Name: "msg",
				Type: &ast.Type{
					NamedType: "String",
				},
			},
		},
	}
}

func addGetQuery(schema *ast.Schema, defn *ast.Definition) {
	schema.Query.Fields = append(schema.Query.Fields,
		&ast.FieldDefinition{
			Description: "Get " + defn.Name + " by ID",
			Name:        "get" + defn.Name,
			Type: &ast.Type{
				NamedType: defn.Name,
			},
			Arguments: []*ast.ArgumentDefinition{
				&ast.ArgumentDefinition{
					Name: "id",
					Type: &ast.Type{
						NamedType: idTypeFor(defn),
						NonNull:   true,
					},
				},
			},
		})
}

func addFilterQuery(schema *ast.Schema, defn *ast.Definition) {
	qry := &ast.FieldDefinition{
		Description: "Query " + defn.Name,
		Name:        "query" + defn.Name,
		Type: &ast.Type{
			Elem: &ast.Type{
				NamedType: defn.Name,
				NonNull:   true,
			},
		},
		Arguments: []*ast.ArgumentDefinition{
			&ast.ArgumentDefinition{
				Name: "filter",
				Type: &ast.Type{
					NamedType: defn.Name + "Filter",
					NonNull:   true,
				},
			},
		},
	}
	schema.Query.Fields = append(schema.Query.Fields, qry)
}

func addQueries(schema *ast.Schema, defn *ast.Definition) {
	addGetQuery(schema, defn)
	addFilterQuery(schema, defn)
}

func addAddMutation(schema *ast.Schema, defn *ast.Definition) {
	add := &ast.FieldDefinition{
		Description: "Add a " + defn.Name,
		Name:        "add" + defn.Name,
		Type: &ast.Type{
			NamedType: "Add" + defn.Name + "Payload",
		},
		Arguments: []*ast.ArgumentDefinition{
			&ast.ArgumentDefinition{
				Name: "input",
				Type: &ast.Type{
					NamedType: defn.Name + "Input",
					NonNull:   true,
				},
			},
		},
	}
	schema.Mutation.Fields = append(schema.Mutation.Fields, add)
}

func addUpdateMutation(schema *ast.Schema, defn *ast.Definition) {
	schema.Mutation.Fields = append(schema.Mutation.Fields,
		&ast.FieldDefinition{
			Description: "Update a " + defn.Name,
			Name:        "update" + defn.Name,
			Type: &ast.Type{
				NamedType: "Update" + defn.Name + "Payload",
			},
			Arguments: []*ast.ArgumentDefinition{
				&ast.ArgumentDefinition{
					Name: "input",
					Type: &ast.Type{
						NamedType: "Update" + defn.Name + "Input",
						NonNull:   true,
					},
				},
			},
		})
}

func addDeleteMutation(schema *ast.Schema, defn *ast.Definition) {
	del := &ast.FieldDefinition{
		Description: "Delete a " + defn.Name,
		Name:        "delete" + defn.Name,
		Type: &ast.Type{
			NamedType: "Delete" + defn.Name + "Payload",
		},
		Arguments: []*ast.ArgumentDefinition{
			&ast.ArgumentDefinition{
				Name: "id",
				Type: &ast.Type{
					NamedType: idTypeFor(defn),
					NonNull:   true,
				},
			},
		},
	}
	schema.Mutation.Fields = append(schema.Mutation.Fields, del)
}

func addMutations(schema *ast.Schema, defn *ast.Definition) {
	addAddMutation(schema, defn)
	addUpdateMutation(schema, defn)
	addDeleteMutation(schema, defn)
}

func getFilterField() ast.FieldList {
	return []*ast.FieldDefinition{
		&ast.FieldDefinition{
			Name: "dgraph",
			Type: &ast.Type{
				NamedType: "String",
			},
		},
	}
}

func getNonIDFields(schema *ast.Schema, defn *ast.Definition) ast.FieldList {
	fldList := make([]*ast.FieldDefinition, 0)
	for _, fld := range defn.Fields {
		if isIDField(defn, fld) {
			continue
		}
		if schema.Types[fld.Type.Name()].Kind == ast.Object {
			newDefn := &ast.FieldDefinition{
				Name: fld.Name,
			}

			newDefn.Type = &ast.Type{}
			newDefn.Type.NonNull = fld.Type.NonNull
			if fld.Type.NamedType != "" {
				newDefn.Type.NamedType = fld.Type.Name() + "Ref"
			} else {
				newDefn.Type.Elem = &ast.Type{
					NamedType: fld.Type.Name() + "Ref",
					NonNull:   fld.Type.Elem.NonNull,
				}
			}

			fldList = append(fldList, newDefn)
		} else {
			newFld := *fld
			newFldType := *fld.Type
			newFld.Type = &newFldType
			fldList = append(fldList, &newFld)
		}
	}
	return fldList
}

func getIDField(defn *ast.Definition) ast.FieldList {
	fldList := make([]*ast.FieldDefinition, 0)
	for _, fld := range defn.Fields {
		if isIDField(defn, fld) {
			// Deepcopy is not required because we don't modify values other than nonull
			newFld := *fld
			fldList = append(fldList, &newFld)
			break
		}
	}
	return fldList
}

func genArgumentsDefnString(args ast.ArgumentDefinitionList) string {
	if len(args) == 0 {
		return ""
	}

	argStrs := make([]string, len(args))
	for i, arg := range args {
		argStrs[i] = genArgumentDefnString(arg)
	}

	return fmt.Sprintf("(%s)", strings.Join(argStrs, ", "))
}

func genArgumentsString(args ast.ArgumentList) string {
	if len(args) == 0 {
		return ""
	}

	argStrs := make([]string, len(args))
	for i, arg := range args {
		argStrs[i] = genArgumentString(arg)
	}

	return fmt.Sprintf("(%s)", strings.Join(argStrs, ", "))
}

func genDirectivesString(direcs ast.DirectiveList) string {
	if len(direcs) == 0 {
		return ""
	}

	direcArgs := make([]string, len(direcs))

	for idx, dir := range direcs {
		direcArgs[idx] = fmt.Sprintf("@%s%s", dir.Name, genArgumentsString(dir.Arguments))
	}

	return " " + strings.Join(direcArgs, " ")
}

func genFieldsString(flds ast.FieldList) string {
	if flds == nil {
		return ""
	}

	var sch strings.Builder

	for _, fld := range flds {
		// Some extra types are generated by gqlparser for internal purpose.
		if !strings.HasPrefix(fld.Name, "__") {
			sch.WriteString(genFieldString(fld))
		}
	}

	return sch.String()
}

func genFieldString(fld *ast.FieldDefinition) string {
	return fmt.Sprintf(
		"\t%s%s: %s%s\n", fld.Name, genArgumentsDefnString(fld.Arguments),
		fld.Type.String(), genDirectivesString(fld.Directives),
	)
}

func genArgumentDefnString(arg *ast.ArgumentDefinition) string {
	return fmt.Sprintf("%s: %s", arg.Name, arg.Type.String())
}

func genArgumentString(arg *ast.Argument) string {
	return fmt.Sprintf("%s: %s", arg.Name, arg.Value.String())
}

func generateInputString(typ *ast.Definition) string {
	return fmt.Sprintf("input %s {\n%s}\n", typ.Name, genFieldsString(typ.Fields))
}

func generateEnumString(typ *ast.Definition) string {
	var sch strings.Builder

	sch.WriteString(fmt.Sprintf("enum %s {\n", typ.Name))
	for _, val := range typ.EnumValues {
		if !strings.HasPrefix(val.Name, "__") {
			sch.WriteString(fmt.Sprintf("\t%s\n", val.Name))
		}
	}
	sch.WriteString("}\n")

	return sch.String()
}

func generateObjectString(typ *ast.Definition) string {
	return fmt.Sprintf("type %s {\n%s}\n", typ.Name, genFieldsString(typ.Fields))
}

func generateScalarString(typ *ast.Definition) string {
	var sch strings.Builder

	sch.WriteString(fmt.Sprintf("scalar %s\n", typ.Name))
	return sch.String()
}

// Stringify returns entire schema in string format
func Stringify(schema *ast.Schema) string {
	var sch, object, scalar, input, query, mutation, enum strings.Builder

	if schema.Types == nil {
		return ""
	}

	keys := make([]string, 0, len(schema.Types))
	for k := range schema.Types {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		typ := schema.Types[key]

		if typ.Kind == ast.Object {
			object.WriteString(generateObjectString(typ) + "\n")
		} else if typ.Kind == ast.Scalar {
			scalar.WriteString(generateScalarString(typ))
		} else if typ.Kind == ast.InputObject {
			input.WriteString(generateInputString(typ) + "\n")
		} else if typ.Kind == ast.Enum {
			enum.WriteString(generateEnumString(typ) + "\n")
		}
	}

	if schema.Query != nil {
		query.WriteString(generateObjectString(schema.Query))
	}

	if schema.Mutation != nil {
		mutation.WriteString(generateObjectString(schema.Mutation))
	}

	sch.WriteString("#######################\n# Generated Types\n#######################\n")
	sch.WriteString(object.String())
	sch.WriteString("#######################\n# Scalar Definitions\n#######################\n")
	sch.WriteString(scalar.String())
	sch.WriteString("#######################\n# Enum Definitions\n#######################\n")
	sch.WriteString(enum.String())
	sch.WriteString("#######################\n# Input Definitions\n#######################\n")
	sch.WriteString(input.String())
	sch.WriteString("#######################\n# Generated Query\n#######################\n")
	sch.WriteString(query.String())
	sch.WriteString("#######################\n# Generated Mutations\n#######################\n")
	sch.WriteString(mutation.String())

	return sch.String()
}

func isIDField(defn *ast.Definition, fld *ast.FieldDefinition) bool {
	return fld.Type.Name() == idTypeFor(defn)
}

func idTypeFor(defn *ast.Definition) string {
	// Placeholder till more ID types are introduced.
	return "ID"
}

func appendIfNotNull(errs []*gqlerror.Error, err *gqlerror.Error) gqlerror.List {
	if err != nil {
		errs = append(errs, err)
	}

	return errs
}
