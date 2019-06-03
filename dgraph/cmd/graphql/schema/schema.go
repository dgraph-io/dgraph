package schema

import (
	"strings"

	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/gqlerror"
)

type schRuleFunc func(schema *ast.Schema) *gqlerror.Error

type schRule struct {
	name        string
	schRuleFunc schRuleFunc
}

var schRules []schRule

// AddSchRule function adds a new schema rule to the global array schRules
func AddSchRule(name string, f schRuleFunc) {
	schRules = append(schRules, schRule{
		name:        name,
		schRuleFunc: f,
	})
}

// ValidateSchema function validates the schema against dgraph's rules of schema.
func ValidateSchema(schema *ast.Schema) gqlerror.List {
	errs := make([]*gqlerror.Error, 0)
	for i := range schRules {
		tmpRVal := schRules[i].schRuleFunc(schema)
		if tmpRVal != nil {
			errs = append(errs, tmpRVal)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

// GenerateCompleteSchema will generate all the required query/mutation/update functions
// for all the types mentioned the the schema
func GenerateCompleteSchema(schema *ast.Schema) {
	extenderMap := make(map[string]*ast.Definition)

	for _, defn := range schema.Types {
		if defn.Kind == "OBJECT" {
			extenderMap[defn.Name+"Input"] = genInputType(schema, defn)
			extenderMap[defn.Name+"Ref"] = genRefType(defn)
			extenderMap[defn.Name+"Update"] = genUpdateType(schema, defn)
			extenderMap[defn.Name+"Filter"] = genFilterType(defn)
			extenderMap["Add"+defn.Name+"Payload"] = genAddResultType(defn)
			extenderMap["Update"+defn.Name+"Payload"] = genUpdResultType(defn)
			extenderMap["Delete"+defn.Name+"Payload"] = genDelResultType(defn)
			addQueryType(defn, schema.Query)
			addMutationType(defn, schema.Mutation)
		}
	}

	for name, extType := range extenderMap {
		schema.Types[name] = extType
	}
}

func genInputType(schema *ast.Schema, defn *ast.Definition) *ast.Definition {
	inputDefn := &ast.Definition{
		Kind:   ast.InputObject,
		Name:   defn.Name + "Input",
		Fields: getNonIDFields(schema, defn),
	}
	return inputDefn
}

func genRefType(defn *ast.Definition) *ast.Definition {
	refDefn := &ast.Definition{
		Kind:   ast.InputObject,
		Name:   defn.Name + "Ref",
		Fields: getIDField(defn),
	}
	return refDefn
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
	fltrDefn := &ast.Definition{
		Kind: ast.InputObject,
		Name: defn.Name + "Filter",
		// Fields: getFieldList(StringMask, defn),
		// Do I have to take all the string fields ???
	}
	return fltrDefn
}

func genAddResultType(defn *ast.Definition) *ast.Definition {
	addDefn := &ast.Definition{
		Kind: ast.Object,
		Name: "Add" + defn.Name + "Payload",
	}
	addFldList := make([]*ast.FieldDefinition, 0)
	parentFld := &ast.FieldDefinition{ // Field type is same as the parent object type
		Name: strings.ToLower(defn.Name),
		Type: &ast.Type{
			NamedType: defn.Name,
			NonNull:   true, // Not sure if it should always be true
		},
	}
	addFldList = append(addFldList, parentFld)
	addDefn.Fields = addFldList
	return addDefn
}

func genUpdResultType(defn *ast.Definition) *ast.Definition {
	updDefn := &ast.Definition{
		Kind: ast.Object,
		Name: "Update" + defn.Name + "Payload",
	}
	updFldList := make([]*ast.FieldDefinition, 0)
	parentFld := &ast.FieldDefinition{ // Field type is same as the parent object type
		Name: strings.ToLower(defn.Name),
		Type: &ast.Type{
			NamedType: defn.Name,
			NonNull:   true, // Not sure if it should always be true
		},
	}
	updFldList = append(updFldList, parentFld)
	updDefn.Fields = updFldList
	return updDefn
}

func genDelResultType(defn *ast.Definition) *ast.Definition {
	delDefn := &ast.Definition{
		Kind: ast.Object,
		Name: "Delete" + defn.Name + "Payload",
	}
	delFldList := make([]*ast.FieldDefinition, 0)
	delFld := &ast.FieldDefinition{
		Name: "msg",
		Type: &ast.Type{
			NamedType: "String",
			NonNull:   true,
		},
	}
	delFldList = append(delFldList, delFld)
	delDefn.Fields = delFldList
	return delDefn
}

func addQueryType(defn *ast.Definition, qry *ast.Definition) {
	getDefn := &ast.FieldDefinition{
		Description: "ID based query function for " + defn.Name,
		Name:        "get" + defn.Name,
		Type: &ast.Type{
			NamedType: defn.Name,
			NonNull:   true, // Idk why but it was in the ast.Schema object if I parse full schema
		},
	}
	getArgs := make([]*ast.ArgumentDefinition, 0)
	getArg := &ast.ArgumentDefinition{
		Name: "id",
		Type: &ast.Type{
			NamedType: "ID",
			NonNull:   true,
		},
	}
	getArgs = append(getArgs, getArg)
	getDefn.Arguments = getArgs
	qry.Fields = append(qry.Fields, getDefn)

	qryDefn := &ast.FieldDefinition{
		Description: "Input Filter based query function for" + defn.Name,
		Name:        "query" + defn.Name,
		Type: &ast.Type{
			NonNull: true,
			Elem: &ast.Type{
				NamedType: defn.Name,
				NonNull:   true,
			},
		},
	}
	qryArgs := make([]*ast.ArgumentDefinition, 0)
	qryArg := &ast.ArgumentDefinition{
		Name: "input",
		Type: &ast.Type{
			NamedType: defn.Name + "Filter",
			NonNull:   true, // Again not sure if this can be null
		},
	}
	qryArgs = append(qryArgs, qryArg)
	qry.Fields = append(qry.Fields, qryDefn)
}

func addMutationType(defn *ast.Definition, mutation *ast.Definition) {
	addDefn := &ast.FieldDefinition{
		Description: "Function for adding " + defn.Name,
		Name:        "add" + defn.Name,
		Type: &ast.Type{
			NamedType: "Add" + defn.Name + "Payload",
			NonNull:   true,
		},
	}
	addArgs := make([]*ast.ArgumentDefinition, 0)
	addArg := &ast.ArgumentDefinition{
		Name: "input",
		Type: &ast.Type{
			NamedType: defn.Name + "Input",
			NonNull:   true,
		},
	}
	addArgs = append(addArgs, addArg)
	addDefn.Arguments = addArgs
	mutation.Fields = append(mutation.Fields, addDefn)

	updDefn := &ast.FieldDefinition{
		Description: "Function for updating " + defn.Name,
		Name:        "update" + defn.Name,
		Type: &ast.Type{
			NamedType: "Update" + defn.Name + "Payload",
			NonNull:   true,
		},
	}
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
			NonNull:   false, // Becasue in example there was no nonnull sign
		},
	}
	updArgs = append(updArgs, updArg)
	updDefn.Arguments = updArgs
	mutation.Fields = append(mutation.Fields, updDefn)

	delDefn := &ast.FieldDefinition{
		Description: "Function for deleting " + defn.Name,
		Name:        "delete" + defn.Name,
		Type: &ast.Type{
			NamedType: "Delete" + defn.Name + "Payload",
			NonNull:   true,
		},
	}
	delArgs := make([]*ast.ArgumentDefinition, 0)
	delArg := &ast.ArgumentDefinition{
		Name: "id",
		Type: &ast.Type{
			NamedType: defn.Name + "ID",
			NonNull:   true,
		},
	}
	delArgs = append(delArgs, delArg)
	delDefn.Arguments = delArgs
	mutation.Fields = append(mutation.Fields, delDefn)
}

// getFieldList returns list of fields based on flag
func getNonIDFields(schema *ast.Schema, defn *ast.Definition) ast.FieldList {
	fldList := make([]*ast.FieldDefinition, 0)
	for _, fld := range defn.Fields {
		if fld.Type.Name() == "ID" {
			continue
		}
		if schema.Types[fld.Type.Name()].Kind == "OBJECT" {
			newDefn := &ast.FieldDefinition{Name: fld.Type.Name() + "Ref"}
			fldList = append(fldList, newDefn)
		} else {
			newFld := *fld
			fldList = append(fldList, &newFld)
		}
	}
	return fldList
}

func getIDField(defn *ast.Definition) ast.FieldList {
	fldList := make([]*ast.FieldDefinition, 0)
	for _, fld := range defn.Fields {
		if fld.Type.Name() == "ID" {
			newFld := *fld // Deepcopy is not required because we will never modify values other than nonull
			fldList = append(fldList, &newFld)
		}
	}
	return fldList
}

func generateInputString(typ *ast.Definition) string {
	var sch strings.Builder

	sch.WriteString("input " + typ.Name + " {\n")
	for _, fld := range typ.Fields {
		sch.WriteString("\t" + fld.Name + ": " + fld.Type.String() + "\n")
	}
	sch.WriteString("}\n")
	return sch.String()
}

func generateObjectString(typ *ast.Definition) string {
	var sch strings.Builder

	sch.WriteString("type " + typ.Name + " {\n")
	for _, fld := range typ.Fields {
		sch.WriteString("\t" + fld.Name + ":" + fld.Type.String() + "\n")
	}
	sch.WriteString("}\n")

	return sch.String()
}

func generateScalarString(typ *ast.Definition) string {
	var sch strings.Builder

	sch.WriteString("Scalar " + typ.Name + "\n")
	return sch.String()
}

func generateQMString(flag bool, qry *ast.Definition) string {
	var sch strings.Builder
	var opType string
	if flag {
		opType = "Query"
	} else {
		opType = "Mutation"
	}

	sch.WriteString("type " + opType + " {\n")
	for _, fld := range qry.Fields {
		sch.WriteString("\t" + fld.Name + "(")
		argLen := len(fld.Arguments) // I hope it returns size of array
		for idx, arg := range fld.Arguments {
			sch.WriteString(arg.Name + ":" + arg.Type.String())
			if idx != argLen-1 {
				sch.WriteString(",")
			}
		}
		sch.WriteString("): " + fld.Type.String() + "\n")
	}
	sch.WriteString("}\n")

	return sch.String()
}

// Stringify will return entire schema in string format
func Stringify(schema *ast.Schema) string {
	var sch, object, scalar, input, ref, filter, payload, query, mutation strings.Builder

	for _, typ := range schema.Types {
		if typ.Kind == ast.Object {
			object.WriteString(generateObjectString(typ))
		} else if typ.Kind == ast.Scalar {
			scalar.WriteString(generateScalarString(typ))
		} else if typ.Kind == ast.InputObject {
			input.WriteString(generateInputString(typ))
		} else if string(typ.Kind)[len(string(typ.Kind))-6:len(string(typ.Kind))] == "Filter" {
			filter.WriteString(generateInputString(typ))
		} else if string(typ.Kind)[len(string(typ.Kind))-7:len(string(typ.Kind))] == "Payload" {
			payload.WriteString(generateObjectString(typ))
		} else if string(typ.Kind)[len(string(typ.Kind))-3:len(string(typ.Kind))] == "Ref" {
			ref.WriteString(generateInputString(typ))
		}
	}

	query.WriteString(generateQMString(true, schema.Query))
	mutation.WriteString(generateQMString(false, schema.Mutation))

	sch.WriteString(object.String())
	sch.WriteString(scalar.String())
	sch.WriteString(input.String())
	sch.WriteString(ref.String())
	sch.WriteString(filter.String())
	sch.WriteString(payload.String())
	sch.WriteString(query.String())
	sch.WriteString(mutation.String())

	return sch.String()
}
