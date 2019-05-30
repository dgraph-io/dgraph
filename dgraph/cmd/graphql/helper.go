package graphql

import (
	"github.com/vektah/gqlparser/ast"
)

// SupportedScalars will be the list of scalar types that we will support.
type SupportedScalars string

const (
	INT      SupportedScalars = "Int"
	FLOAT    SupportedScalars = "Float"
	STRING   SupportedScalars = "String"
	DATETIME SupportedScalars = "DateTime"
	ID       SupportedScalars = "ID"
	BOOLEAN  SupportedScalars = "Boolean"
)

// addScalars adds simply adds all the supported scalars in the schema
func addScalars(doc *ast.SchemaDocument) {
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

func StringifySchema(sch *ast.Schema) string {

}
