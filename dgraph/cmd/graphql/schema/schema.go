package schema

import (
	"strings"

	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql"
	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/gqlerror"
)

const (
	IntMask      = 1 << 0
	FloatMask    = 1 << 1
	StringMask   = 1 << 2
	DatetimeMask = 1 << 3
	IDMask       = 1 << 4
	BooleanMask  = 1 << 5
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
			extenderMap[defn.Name+"Input"] = genInputType(defn)
			extenderMap[defn.Name+"Ref"] = genRefType(defn)
			extenderMap[defn.Name+"Update"] = genUpdateType(defn)
			extenderMap[def.Name+"Filter"] = genFilterType(defn)
			extenderMap["Add"+def.Name+"Payload"] = genResultType(defn)
			addQueryType(defn, schema.Query, extenderMap[def.Name+"Filter"])
		}
	}
}

func genInputType(defn *ast.Definition) *ast.Definition {
	inputDefn := &ast.Definition{
		Kind: ast.InputObject,
		Name: defn.Name + "Input",
	}
	inputDefn.Fields =
		getFieldList(StringMask|FloatMask|BooleanMask|DatetimeMask|IntMask, defn)

	return inputDefn
}

func genRefType(defn *ast.Definition) *ast.Definition {
	refDefn := &ast.Definition{
		Kind: ast.InputObject,
		Name: defn.Name + "Ref",
	}
	refDefn.Fields = getFieldList(IDMask, defn)

	return refDefn
}

func genUpdateType(defn *ast.Definition) *ast.Definition {
	updDefn := &ast.Definition{
		Kind: ast.InputObject,
		Name: defn.Name + "Update",
	}
	updDefn.Fields =
		getFieldList(StringMask|FloatMask|BooleanMask|DatetimeMask|IntMask, defn)

	for _, fld := range updDefn.Fields {
		fld.Type.NonNull = false
	}

	return updDefn
}

func genFilterType(defn *ast.Definition) *ast.Definition {
	fltrDefn := &ast.Definition{
		Kind: ast.InputObject,
		Name: defn.Name + "Filter",
	}
	fltrDefn.Fields =
		getFieldList(StringMask, defn)
}

func genResultType(defn *ast.Definition) *ast.Definition {
	// @TODO: Need a bit of discussion around it. How exactly we
	// decide name of the field that will be reutrned. For e.g.
	// In refernce to what Michael has suggested
	// type AddPostPayload {
	//    post: Post! (Here what is significance of post?)
	// }
}

func addQuery(defn *ast.Definition, qry *ast.Query, filterDefn *ast.Definition) {
	getDefn := &ast.FieldDefinition {
		Description: "query function for " + defn.Name,
		Name: "query" + defn.Name,
	}
	getDefn.Type = &ast.Type{
		NamedType: defn.Name
		NonNull: true // Idk why but it was in the ast.Schema object if I parse full schema
	}
	qry.Fields = append(qry.Fields, getDefn)

	qryDefn := &ast.FieldDefinition {
		Description: ""
	}
}

// getFieldList returns list of fields based on flag
func getFieldList(flag int8, defn *ast.Definition) ast.FieldList {
	fldList := make([]*ast.FieldDefinition, 0)
	for _, fld := range defn.Fields {
		if fld.Type.Name() == graphql.ID && (flag & IDMask) {
			fldList = append(fldList, fld)
		} else if fld.Type.Name() == graphql.STRING && (flag & StringMask) {
			fldList = append(fldList, fld)
		} else if fld.Type.Name() == graphql.FLOAT && (flag & FloatMask) {
			fldList = append(fldList, fld)
		} else if fld.Type.Name() == graphql.BOOLEAN && (flag & BooleanMask) {
			fldList = append(flag, fld)
		} else if fld.Type.Name() == graphql.DATETIME && (flag & DatetimeMask) {
			fldList = append(flag, fld)
		} else if fld.Type.Name() == graphql.INT && (flag & IntMask) {
			fldList = append(flag, fld)
		}
	}
	return fldList
}

// generateInitialSchema will generate the schema that was initially provided by the user
func generateInitialSchema(schema *ast.Schema) string {
	var sch strings.Builder

	for name, defs := range schema.PossibleTypes {
		defn := defs[0] // Again I don't have idea why this is an array. I am just using first val of array. It has only one element in it as far as I have seen.
		sch.WriteString("type " + name + " {\n")
		for _, fld := range defn.Fields {
			sch.WriteString("\t" + fld.Name + ": " + fld.Type.String() + "\n") // Anonymus strings and other strings concatenation. How will this perform ?
		}
		sch.WriteString("}\n")
	}
	return sch.String()
}

// generateInputSchema will generate schema for all the input types
// It won't have any type of ID because ID will be assigned internally by dgraph
func generateInputSchema(schema *ast.Schema) string {
	var sch strings.Builder

	for name, defs := range schema.PossibleTypes {
		defn := defs[0]
		sch.WriteString("type " + name + "Input {\n")
		for _, fld := range defn.Fields {
			if fld.Type.NamedType == "id" {
				continue
			}
			sch.WriteString("\t" + fld.Name + ": " + fld.Type.String() + "\n")
		}
		sch.WriteString("}\n")
	}
	return sch.String()
}

// generateRefSchema will generate reference schema i.e. the fucntion
// which will be able to refer to any object based on id
func generateRefSchema(schema *ast.Schema) string {
	var sch strings.Builder

	for name, defs := range schema.PossibleTypes {
		defn := defs[0]
		sch.WriteString("type " + name + "Ref {\n")
		for _, fld := range defn.Fields {
			if fld.Type.NamedType != "id" {
				continue
			}
			sch.WriteString("\t" + fld.Name + ": ID!\n")
		}
		sch.WriteString("}\n")
	}
	return sch.String()
}

// generateUpdateSchema will generate schema for all functions which will be
// used to update a particular type. It shouldn't have ID type as ID can't be udpated.
// Also none of the fields should have '!' because nothing should be compulsory for update.
func generateUpdateSchema(schema *ast.Schema) string {
	var sch strings.Builder

	for name, defs := range schema.PossibleTypes {
		defn := defs[0]
		sch.WriteString("type " + name + "Update {\n")
		for _, fld := range defn.Fields {
			if fld.Type.NamedType == "id" {
				continue
			}
			sch.WriteString("\t" + fld.Name + ": " + fld.Type.Name() + "\n")
		}
		sch.WriteString("}\n")
	}
	return sch.String()
}

// generateFilterSchema will generate schema for filter functions which can be used to
// search based on filter given.
func generateFilterSchema(schema *ast.Schema) string {
	var sch strings.Builder

	for name, defs := range schema.PossibleTypes {
		defn := defs[0]
		sch.WriteString("type " + name + "Filter {\n")
		for _, fld := range defn.Fields {
			if fld.Type.NamedType != "string" {
				continue
			}
			sch.WriteString("\t" + fld.Name + ": " + fld.Type.Name() + "\n")
		}
		sch.WriteString("}\n")
	}
	return sch.String()
}

// generateTypeSchema will generate shema for return types of mutations
func generateTypeSchema(schema *ast.Schema) string {
	var sch strings.Builder

	for name := range schema.PossibleTypes {
		sch.WriteString("type Add" + name + "Payload {\n")
		sch.WriteString("\t" + strings.ToLower(name) + ": " + name + "!\n}\n")
	}
	return sch.String()
}

// generateQuerySchema will generate schema for the functions which will be used
// to query data.
func generateQuerySchema(schema *ast.Schema) string {
	var sch strings.Builder

	for name, defs := range schema.PossibleTypes {
		defn := defs[0] // Again I don't have idea why this is an array. I am just using first val of array. It has only one element in it as far as I have seen.
		sch.WriteString("\tget" + name + "(")
		for _, fld := range defn.Fields {
			if fld.Type.NamedType == "id" {
				sch.WriteString(fld.Name + ": ID!): ")
				break
			}
		}
		sch.WriteString(name + "!\n") // Why this can even be empty in case of invalid id
	}
	sch.WriteString("}\n")
	return sch.String()
}

// generateMutationSchema will generate schema for mutation on objects specified in schema
func generateMutationSchema(schema *ast.Schema) string {
	return ""
}
