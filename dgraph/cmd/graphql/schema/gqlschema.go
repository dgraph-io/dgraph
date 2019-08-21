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

var supportedScalars = map[string]scalar{
	"ID":       scalar{name: "ID", dgraphType: "uid"},
	"Boolean":  scalar{name: "Boolean", dgraphType: "bool"},
	"Int":      scalar{name: "Int", dgraphType: "int"},
	"Float":    scalar{name: "Float", dgraphType: "float"},
	"String":   scalar{name: "String", dgraphType: "string"},
	"DateTime": scalar{name: "DateTime", dgraphType: "dateTime"},
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
			sch.Types[defn.Name+"Input"] = genInputType(sch, defn)
			sch.Types[defn.Name+"Ref"] = genRefType(defn)
			sch.Types["Patch"+defn.Name] = genPatchType(sch, defn)
			sch.Types[defn.Name+"Update"] = genUpdateType(sch, defn)
			sch.Types[defn.Name+"Filter"] = genFilterType(defn)
			sch.Types["Add"+defn.Name+"Payload"] = genAddResultType(defn)
			sch.Types["Update"+defn.Name+"Payload"] = genUpdResultType(defn)
			sch.Types["Delete"+defn.Name+"Payload"] = genDelResultType(defn)

			sch.Query.Fields = append(sch.Query.Fields, addQueryType(defn)...)
			sch.Mutation.Fields = append(sch.Mutation.Fields, addMutationType(defn)...)
		}
	}
}

func genInputType(schema *ast.Schema, defn *ast.Definition) *ast.Definition {
	return &ast.Definition{
		Kind:   ast.InputObject,
		Name:   defn.Name + "Input",
		Fields: getNonIDFields(schema, defn),
	}
}

func genRefType(defn *ast.Definition) *ast.Definition {
	return &ast.Definition{
		Kind:   ast.InputObject,
		Name:   defn.Name + "Ref",
		Fields: getIDField(defn),
	}
}

func genUpdateType(schema *ast.Schema, defn *ast.Definition) *ast.Definition {
	return &ast.Definition{
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
}

func genPatchType(schema *ast.Schema, defn *ast.Definition) *ast.Definition {
	patchDefn := &ast.Definition{
		Kind:   ast.InputObject,
		Name:   "Patch" + defn.Name,
		Fields: getNonIDFields(schema, defn),
	}

	for _, fld := range patchDefn.Fields {
		fld.Type.NonNull = false
	}

	return patchDefn
}

func genFilterType(defn *ast.Definition) *ast.Definition {
	return &ast.Definition{
		Kind:   ast.InputObject,
		Name:   defn.Name + "Filter",
		Fields: getFilterField(),
	}
}

func genAddResultType(defn *ast.Definition) *ast.Definition {
	return &ast.Definition{
		Kind: ast.Object,
		Name: "Add" + defn.Name + "Payload",
		Fields: []*ast.FieldDefinition{
			&ast.FieldDefinition{
				Name: strings.ToLower(defn.Name),
				Type: &ast.Type{
					NamedType: defn.Name,
					NonNull:   true,
				},
			},
		},
	}
}

func genUpdResultType(defn *ast.Definition) *ast.Definition {
	return &ast.Definition{
		Kind: ast.Object,
		Name: "Update" + defn.Name + "Payload",
		Fields: []*ast.FieldDefinition{
			&ast.FieldDefinition{ // Field type is same as the parent object type
				Name: strings.ToLower(defn.Name),
				Type: &ast.Type{
					NamedType: defn.Name,
					NonNull:   true,
				},
			},
		},
	}
}

func genDelResultType(defn *ast.Definition) *ast.Definition {
	return &ast.Definition{
		Kind: ast.Object,
		Name: "Delete" + defn.Name + "Payload",
		Fields: []*ast.FieldDefinition{
			&ast.FieldDefinition{
				Name: "msg",
				Type: &ast.Type{
					NamedType: "String",
					NonNull:   true,
				},
			},
		},
	}
}

func createGetFld(defn *ast.Definition) *ast.FieldDefinition {
	return &ast.FieldDefinition{
		Description: "Query " + defn.Name + " by ID",
		Name:        "get" + defn.Name,
		Type: &ast.Type{
			NamedType: defn.Name,
			NonNull:   true,
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
}

func createQryFld(defn *ast.Definition) *ast.FieldDefinition {
	return &ast.FieldDefinition{
		Description: "Query " + defn.Name,
		Name:        "query" + defn.Name,
		Type: &ast.Type{
			NonNull: true,
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
}

func addQueryType(defn *ast.Definition) (flds []*ast.FieldDefinition) {
	flds = append(flds, createGetFld(defn))
	flds = append(flds, createQryFld(defn))

	return
}

func createAddFld(defn *ast.Definition) *ast.FieldDefinition {
	return &ast.FieldDefinition{
		Description: "Function for adding " + defn.Name,
		Name:        "add" + defn.Name,
		Type: &ast.Type{
			NamedType: "Add" + defn.Name + "Payload",
			NonNull:   true,
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
}

func createUpdFld(defn *ast.Definition) *ast.FieldDefinition {
	return &ast.FieldDefinition{
		Description: "Update a " + defn.Name,
		Name:        "update" + defn.Name,
		Type: &ast.Type{
			NamedType: "Update" + defn.Name + "Payload",
			NonNull:   false,
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
	}
}

func createDelFld(defn *ast.Definition) *ast.FieldDefinition {
	return &ast.FieldDefinition{
		Description: "Function for deleting " + defn.Name,
		Name:        "delete" + defn.Name,
		Type: &ast.Type{
			NamedType: "Delete" + defn.Name + "Payload",
			NonNull:   true,
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
}

func addMutationType(defn *ast.Definition) (flds []*ast.FieldDefinition) {
	flds = append(flds, createAddFld(defn))
	flds = append(flds, createUpdFld(defn))
	flds = append(flds, createDelFld(defn))

	return
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

func genArgumentsString(args ast.ArgumentDefinitionList) string {
	if args == nil || len(args) == 0 {
		return ""
	}

	var argsStrs []string

	for _, arg := range args {
		argsStrs = append(argsStrs, genArgumentString(arg))
	}

	return fmt.Sprintf("(%s)", strings.Join(argsStrs, ","))
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
		"\t%s%s: %s\n", fld.Name, genArgumentsString(fld.Arguments), fld.Type.String(),
	)
}

func genArgumentString(arg *ast.ArgumentDefinition) string {
	return fmt.Sprintf("%s: %s", arg.Name, arg.Type.String())
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
