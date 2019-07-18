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
	"sort"
	"strings"

	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/gqlerror"
)

// SupportedScalars will be the list of scalar types that we will support.
type SupportedScalars string

type schRuleFunc func(schema *ast.SchemaDocument) *gqlerror.Error

type schRule struct {
	name        string
	schRuleFunc schRuleFunc
}

var schRules []schRule

const (
	INT      SupportedScalars = "Int"
	FLOAT    SupportedScalars = "Float"
	STRING   SupportedScalars = "String"
	DATETIME SupportedScalars = "DateTime"
	ID       SupportedScalars = "ID"
	BOOLEAN  SupportedScalars = "Boolean"
)

// AddScalars adds simply adds all the supported scalars in the schema.
func AddScalars(doc *ast.SchemaDocument) {
	addScalarInSchema(INT, doc)
	addScalarInSchema(FLOAT, doc)
	addScalarInSchema(ID, doc)
	addScalarInSchema(DATETIME, doc)
	addScalarInSchema(STRING, doc)
	addScalarInSchema(BOOLEAN, doc)
}

func addScalarInSchema(sType SupportedScalars, doc *ast.SchemaDocument) {
	for _, def := range doc.Definitions {
		if def.Kind == "SCALAR" && def.Name == string(sType) { // Just to check if it is already added
			return
		}
	}

	doc.Definitions = append(doc.Definitions, &ast.Definition{Kind: ast.Scalar, Name: string(sType)})
}

// AddRule function adds a new schema rule to the global array schRules.
func AddRule(name string, f schRuleFunc) {
	schRules = append(schRules, schRule{
		name:        name,
		schRuleFunc: f,
	})
}

// ValidateSchema function validates the schema against dgraph's rules of schema.
func ValidateSchema(schema *ast.SchemaDocument) gqlerror.List {
	var errs []*gqlerror.Error

	for i := range schRules {
		gqlErr := schRules[i].schRuleFunc(schema)
		if gqlErr != nil {
			errs = append(errs, gqlErr)
		}
	}

	return errs
}

// GenerateCompleteSchema will generate all the required query/mutation/update functions
// for all the types mentioned the the schema.
func GenerateCompleteSchema(schema *ast.Schema) {
	extenderMap := make(map[string]*ast.Definition)

	schema.Query = &ast.Definition{
		Kind:        ast.Object,
		Description: "Query object contains all the query functions",
		Name:        "Query",
		Fields:      make([]*ast.FieldDefinition, 0),
	}

	schema.Mutation = &ast.Definition{
		Kind:        ast.Object,
		Description: "Mutation object contains all the mutation functions",
		Name:        "Mutation",
		Fields:      make([]*ast.FieldDefinition, 0),
	}

	for _, defn := range schema.Types {
		if defn.Kind == "OBJECT" {
			extenderMap[defn.Name+"Input"] = genInputType(schema, defn)
			extenderMap[defn.Name+"Ref"] = genRefType(defn)
			extenderMap[defn.Name+"Update"] = genUpdateType(schema, defn)
			extenderMap[defn.Name+"Filter"] = genFilterType(defn)
			extenderMap["Add"+defn.Name+"Payload"] = genAddResultType(defn)
			extenderMap["Update"+defn.Name+"Payload"] = genUpdResultType(defn)
			extenderMap["Delete"+defn.Name+"Payload"] = genDelResultType(defn)

			schema.Query.Fields = append(schema.Query.Fields, addQueryType(defn)...)
			schema.Mutation.Fields = append(schema.Mutation.Fields, addMutationType(defn)...)
		}
	}

	for name, extType := range extenderMap {
		schema.Types[name] = extType
	}
}

// AreEqualSchema checks if sch1 and sch2 are the same schema.
func AreEqualSchema(sch1, sch2 *ast.Schema) bool {
	return AreEqualQuery(sch1.Query, sch2.Query) &&
		AreEqualMutation(sch1.Mutation, sch2.Mutation) &&
		AreEqualTypes(sch1.Types, sch2.Types)
}

// AreEqualQuery checks if query blocks qry1, qry2 are same.
func AreEqualQuery(qry1, qry2 *ast.Definition) bool {
	return AreEqualFields(qry1.Fields, qry2.Fields)
}

// AreEqualMutation checks if mutation blocks mut1, mut2 are same.
func AreEqualMutation(mut1, mut2 *ast.Definition) bool {
	return AreEqualFields(mut1.Fields, mut2.Fields)
}

// AreEqualTypes checks if types typ1, typ2 are same.
func AreEqualTypes(typ1, typ2 map[string]*ast.Definition) bool {
	for name, def := range typ1 {
		val, ok := typ2[name]

		if !ok || def.Kind != val.Kind {
			return false
		}

		if !AreEqualFields(def.Fields, val.Fields) {
			return false
		}
	}

	return true
}

// AreEqualFields checks if fieldlist flds1, flds2 are same.
func AreEqualFields(flds1, flds2 ast.FieldList) bool {
	fldDict := make(map[string]*ast.FieldDefinition)

	for _, fld := range flds1 {
		fldDict[fld.Name] = fld
	}

	for _, fld := range flds2 {

		if strings.HasPrefix(fld.Name, "__") {
			continue
		}
		val, ok := fldDict[fld.Name]

		if !ok {
			return false
		}

		var s1, s2 strings.Builder

		s1.WriteString("\t" + fld.Name)
		s1.WriteString(genArgumentsString(fld.Arguments) + ": ")
		s1.WriteString(fld.Type.String() + "\n")

		s2.WriteString("\t" + val.Name)
		s2.WriteString(genArgumentsString(val.Arguments) + ": ")
		s2.WriteString(val.Type.String() + "\n")

		if s1.String() != s2.String() {
			return false
		}
	}

	return true
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
	updDefn := &ast.Definition{
		Kind:   ast.InputObject,
		Name:   defn.Name + "Update",
		Fields: getNonIDFields(schema, defn),
	}

	for _, fld := range updDefn.Fields {
		fld.Type.NonNull = false
	}

	return updDefn
}

func genFilterType(defn *ast.Definition) *ast.Definition {
	return &ast.Definition{
		Kind:   ast.InputObject,
		Name:   defn.Name + "Filter",
		Fields: getFilterField(),
	}
}

func genAddResultType(defn *ast.Definition) *ast.Definition {
	addFldList := make([]*ast.FieldDefinition, 0)
	parentFld := &ast.FieldDefinition{ // Field type is same as the parent object type
		Name: strings.ToLower(defn.Name),
		Type: &ast.Type{
			NamedType: defn.Name,
			NonNull:   true,
		},
	}
	addFldList = append(addFldList, parentFld)

	return &ast.Definition{
		Kind:   ast.Object,
		Name:   "Add" + defn.Name + "Payload",
		Fields: addFldList,
	}
}

func genUpdResultType(defn *ast.Definition) *ast.Definition {
	updFldList := make([]*ast.FieldDefinition, 0)
	parentFld := &ast.FieldDefinition{ // Field type is same as the parent object type
		Name: strings.ToLower(defn.Name),
		Type: &ast.Type{
			NamedType: defn.Name,
			NonNull:   true,
		},
	}
	updFldList = append(updFldList, parentFld)

	return &ast.Definition{
		Kind:   ast.Object,
		Name:   "Update" + defn.Name + "Payload",
		Fields: updFldList,
	}
}

func genDelResultType(defn *ast.Definition) *ast.Definition {
	delFldList := make([]*ast.FieldDefinition, 0)
	delFld := &ast.FieldDefinition{
		Name: "msg",
		Type: &ast.Type{
			NamedType: "String",
			NonNull:   true,
		},
	}
	delFldList = append(delFldList, delFld)

	return &ast.Definition{
		Kind:   ast.Object,
		Name:   "Delete" + defn.Name + "Payload",
		Fields: delFldList,
	}
}

func createGetFld(defn *ast.Definition) *ast.FieldDefinition {
	getArgs := make([]*ast.ArgumentDefinition, 0)
	getArg := &ast.ArgumentDefinition{
		Name: "id",
		Type: &ast.Type{
			NamedType: "ID",
			NonNull:   true,
		},
	}
	getArgs = append(getArgs, getArg)

	return &ast.FieldDefinition{
		Description: "Query " + defn.Name + " by ID",
		Name:        "get" + defn.Name,
		Type: &ast.Type{
			NamedType: defn.Name,
			NonNull:   true,
		},
		Arguments: getArgs,
	}
}

func createQryFld(defn *ast.Definition) *ast.FieldDefinition {
	qryArgs := make([]*ast.ArgumentDefinition, 0)
	qryArg := &ast.ArgumentDefinition{
		Name: "filter",
		Type: &ast.Type{
			NamedType: defn.Name + "Filter",
			NonNull:   true,
		},
	}
	qryArgs = append(qryArgs, qryArg)

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
		Arguments: qryArgs,
	}
}

func addQueryType(defn *ast.Definition) (flds []*ast.FieldDefinition) {
	flds = append(flds, createGetFld(defn))
	flds = append(flds, createQryFld(defn))

	return
}

func createAddFld(defn *ast.Definition) *ast.FieldDefinition {
	addArgs := make([]*ast.ArgumentDefinition, 0)
	addArg := &ast.ArgumentDefinition{
		Name: "input",
		Type: &ast.Type{
			NamedType: defn.Name + "Input",
			NonNull:   true,
		},
	}
	addArgs = append(addArgs, addArg)

	return &ast.FieldDefinition{
		Description: "Function for adding " + defn.Name,
		Name:        "add" + defn.Name,
		Type: &ast.Type{
			NamedType: "Add" + defn.Name + "Payload",
			NonNull:   true,
		},
		Arguments: addArgs,
	}
}

func createUpdFld(defn *ast.Definition) *ast.FieldDefinition {
	updArgs := make([]*ast.ArgumentDefinition, 0)
	updArg := &ast.ArgumentDefinition{
		Name: "id",
		Type: &ast.Type{
			NamedType: "ID",
			NonNull:   true,
		},
	}
	updArgs = append(updArgs, updArg)
	updArg = &ast.ArgumentDefinition{
		Name: "input",
		Type: &ast.Type{
			NamedType: defn.Name + "Update",
			NonNull:   false,
		},
	}
	updArgs = append(updArgs, updArg)

	return &ast.FieldDefinition{
		Description: "Function for updating " + defn.Name,
		Name:        "update" + defn.Name,
		Type: &ast.Type{
			NamedType: "Update" + defn.Name + "Payload",
			NonNull:   true,
		},
		Arguments: updArgs,
	}
}

func createDelFld(defn *ast.Definition) *ast.FieldDefinition {
	delArgs := make([]*ast.ArgumentDefinition, 0)
	delArg := &ast.ArgumentDefinition{
		Name: "id",
		Type: &ast.Type{
			NamedType: "ID",
			NonNull:   true,
		},
	}
	delArgs = append(delArgs, delArg)

	return &ast.FieldDefinition{
		Description: "Function for deleting " + defn.Name,
		Name:        "delete" + defn.Name,
		Type: &ast.Type{
			NamedType: "Delete" + defn.Name + "Payload",
			NonNull:   true,
		},
		Arguments: delArgs,
	}
}

func addMutationType(defn *ast.Definition) (flds []*ast.FieldDefinition) {
	flds = append(flds, createAddFld(defn))
	flds = append(flds, createUpdFld(defn))
	flds = append(flds, createDelFld(defn))

	return
}

func getFilterField() ast.FieldList {
	fldList := make([]*ast.FieldDefinition, 0)

	newDefn := &ast.FieldDefinition{
		Name: "dgraph",
		Type: &ast.Type{
			NamedType: string(STRING),
		},
	}

	fldList = append(fldList, newDefn)
	return fldList
}

func getNonIDFields(schema *ast.Schema, defn *ast.Definition) ast.FieldList {
	fldList := make([]*ast.FieldDefinition, 0)
	for _, fld := range defn.Fields {
		if fld.Type.Name() == "ID" {
			continue
		}
		if schema.Types[fld.Type.Name()].Kind == "OBJECT" {
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
		if fld.Type.Name() == "ID" {
			// Deepcopy is not required because we will never modify values other than nonull
			newFld := *fld
			fldList = append(fldList, &newFld)
			break
		}
	}
	return fldList
}

func genArgumentsString(args ast.ArgumentDefinitionList) string {
	if args == nil {
		return ""
	}

	var sch strings.Builder
	var argsStrs []string

	sch.WriteString("(")
	for _, arg := range args {
		argsStrs = append(argsStrs, arg.Name+": "+arg.Type.String())
	}

	sort.Slice(argsStrs, func(i, j int) bool { return argsStrs[i] < argsStrs[j] })
	sch.WriteString(strings.Join(argsStrs, ",") + ")")

	return sch.String()
}

func genFieldsString(flds ast.FieldList) string {
	if flds == nil {
		return ""
	}

	var sch strings.Builder

	for _, fld := range flds {
		// Some extra types are generated by gqlparser for internal purpose.
		if !strings.HasPrefix(fld.Name, "__") {
			sch.WriteString("\t" + fld.Name)
			sch.WriteString(genArgumentsString(fld.Arguments) + ": ")
			sch.WriteString(fld.Type.String() + "\n")
		}
	}

	return sch.String()
}

func generateInputString(typ *ast.Definition) string {
	var sch strings.Builder

	sch.WriteString("input " + typ.Name + " {\n")
	sch.WriteString(genFieldsString(typ.Fields) + "}\n")
	return sch.String()
}

func generateEnumString(typ *ast.Definition) string {
	var sch strings.Builder

	sch.WriteString("enum " + typ.Name + " {\n")
	for _, val := range typ.EnumValues {
		if !strings.HasPrefix(val.Name, "__") {
			sch.WriteString("\t" + val.Name + "\n")
		}
	}
	sch.WriteString("}\n")

	return sch.String()
}

func generateObjectString(typ *ast.Definition) string {
	var sch strings.Builder

	sch.WriteString("type " + typ.Name + " {\n")
	sch.WriteString(genFieldsString(typ.Fields) + "}\n")

	return sch.String()
}

func generateScalarString(typ *ast.Definition) string {
	var sch strings.Builder

	sch.WriteString("scalar " + typ.Name + "\n")
	return sch.String()
}

func generateQMString(flag bool, def *ast.Definition) string {
	var sch strings.Builder
	var opType string
	if flag {
		opType = "Query"
	} else {
		opType = "Mutation"
	}

	sch.WriteString("type " + opType + " {\n")
	sch.WriteString(genFieldsString(def.Fields) + "}\n")

	return sch.String()
}

// Stringify will return entire schema in string format
func Stringify(schema *ast.Schema) string {
	var sch, object, scalar, input, ref, filter, payload, query, mutation strings.Builder

	if schema.Types == nil {
		return ""
	}

	for name, typ := range schema.Types {
		if typ.Kind == ast.Object {
			object.WriteString(generateObjectString(typ) + "\n")
		} else if typ.Kind == ast.Scalar {
			scalar.WriteString(generateScalarString(typ))
		} else if typ.Kind == ast.InputObject {
			input.WriteString(generateInputString(typ) + "\n")
		} else if typ.Kind == ast.Enum {
			input.WriteString(generateEnumString(typ) + "\n")
		} else if len(name) >= 7 && name[len(name)-7:len(name)] == "Payload" {
			payload.WriteString(generateObjectString(typ) + "\n")
		} else if len(name) >= 3 && name[len(name)-3:len(name)] == "Ref" {
			ref.WriteString(generateInputString(typ) + "\n")
		}
	}

	if schema.Query != nil {
		query.WriteString(generateQMString(true, schema.Query))
	}

	if schema.Mutation != nil {
		mutation.WriteString(generateQMString(false, schema.Mutation))
	}

	sch.WriteString(object.String())
	sch.WriteString(scalar.String() + "\n")
	sch.WriteString(input.String())
	sch.WriteString(ref.String())
	sch.WriteString(filter.String())
	sch.WriteString(payload.String())
	sch.WriteString(query.String())
	sch.WriteString(mutation.String())

	return sch.String()
}
