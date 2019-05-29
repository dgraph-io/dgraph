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
func GenerateCompleteSchema(schema *ast.Schema) string {
	fullSchema := ""

	fullSchema += generateInitialSchema(schema)

	fullSchema += generateInputSchema(schema)

	fullSchema += generateRefSchema(schema)

	fullSchema += generateUpdateSchema(schema)

	fullSchema += generateFilterSchema(schema)

	fullSchema += generateTypeSchema(schema)

	fullSchema += generateQuerySchema(schema)

	fullSchema += generateMutationSchema(schema)

	return fullSchema
}

// generateInitialSchema will generate the schema that was initially provided by the user
func generateInitialSchema(schema *ast.Schema) string {
	sch := ""

	for name, defs := range schema.PossibleTypes {
		defn := defs[0] // Again I don't have idea why this is an array. I am just using first val of array. It has only one element in it as far as I have seen.
		sch += "type " + name + " {\n"
		for _, fld := range defn.Fields {
			sch += "\t" + fld.Name + ": " + fld.Type.NamedType
			if fld.Type.NonNull {
				sch += "!"
			}
			sch += "\n"
		}
		sch += "}\n"
	}
	return sch
}

// generateInputSchema will generate schema for all the input types
// It won't have any type of ID because ID will be assigned internally by dgraph
func generateInputSchema(schema *ast.Schema) string {
	sch := ""

	for name, defs := range schema.PossibleTypes {
		defn := defs[0]
		sch += "type " + name + "Input {\n"
		for _, fld := range defn.Fields {
			if fld.Type.NamedType == "id" {
				continue
			}
			sch += "\t" + fld.Name + ": " + fld.Type.NamedType
			if fld.Type.NonNull {
				sch += "!"
			}
			sch += "\n"
		}
		sch += "}\n"
	}
	return sch
}

// generateRefSchema will generate reference schema i.e. the fucntion
// which will be able to refer to any object based on id
func generateRefSchema(schema *ast.Schema) string {
	sch := ""

	for name, defs := range schema.PossibleTypes {
		defn := defs[0]
		sch += "type " + name + "Ref {\n"
		for _, fld := range defn.Fields {
			if fld.Type.NamedType != "id" {
				continue
			}
			sch += "\t" + fld.Name + ": ID!\n"
		}
		sch += "}\n"
	}
	return sch
}

// generateUpdateSchema will generate schema for all functions which will be
// used to update a particular type. It shouldn't have ID type as ID can't be udpated.
// Also none of the fields should have '!' because nothing should be compulsory for update.
func generateUpdateSchema(schema *ast.Schema) string {
	sch := ""

	for name, defs := range schema.PossibleTypes {
		defn := defs[0]
		sch += "type " + name + "Update {\n"
		for _, fld := range defn.Fields {
			if fld.Type.NamedType == "id" {
				continue
			}
			sch += "\t" + fld.Name + ": " + fld.Type.NamedType + "\n"
		}
		sch += "}\n"
	}
	return sch
}

// generateFilterSchema will generate schema for filter functions which can be used to
// search based on filter given.
func generateFilterSchema(schema *ast.Schema) string {
	sch := ""

	for name, defs := range schema.PossibleTypes {
		defn := defs[0]
		sch += "type " + name + "Filter {\n"
		for _, fld := range defn.Fields {
			if fld.Type.NamedType != "string" {
				continue
			}
			sch += "\t" + fld.Name + ": " + fld.Type.NamedType + "\n"
		}
		sch += "}\n"
	}
	return sch
}

// generateTypeSchema will generate shema for return types of mutations
func generateTypeSchema(schema *ast.Schema) string {
	sch := ""

	for name := range schema.PossibleTypes {
		sch += "type Add" + name + "Payload {\n"
		sch += "\t" + strings.ToLower(name) + ": " + name + "!\n}\n"
	}
	return sch
}

// generateQuerySchema will generate schema for the functions which will be used
// to query data.
func generateQuerySchema(schema *ast.Schema) string {
	sch := "type Query {\n"

	for name, defs := range schema.PossibleTypes {
		defn := defs[0] // Again I don't have idea why this is an array. I am just using first val of array. It has only one element in it as far as I have seen.
		sch += "\tget" + name + "("
		for _, fld := range defn.Fields {
			if fld.Type.NamedType == "id" {
				sch += fld.Name + ": ID!): "
				break
			}
		}
		sch += name + "!\n" // Why this can even be empty in case of invalid id
	}
	sch += "}\n"
	return sch
}

// generateMutationSchema will generate schema for mutation on objects specified in schema
func generateMutationSchema(schema *ast.Schema) string {
	return ""
}
