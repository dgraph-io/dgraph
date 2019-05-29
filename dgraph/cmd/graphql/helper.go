package graphql

import (
	"strings"

	"github.com/vektah/gqlparser/ast"
)

// SupportedScalars will be the list of scalar types that we will support.
type SupportedScalars string

const (
	INT      SupportedScalars = "int"
	FLOAT    SupportedScalars = "float"
	STRING   SupportedScalars = "string"
	DATETIME SupportedScalars = "datetime"
	ID       SupportedScalars = "id"
	BOOLEAN  SupportedScalars = "boolean"
)

// addScalars adds all the scalars that are used in the schema
// provided by user, with condition that those scalars should be
// listed as SupportedScalars
func addScalars(doc *ast.SchemaDocument) {
	for _, def := range doc.Definitions {
		if def.Kind == "OBJECT" {
			for _, fld := range def.Fields {
				switch strings.ToLower(fld.Type.NamedType) {
				case string(INT):
					addScalarInSchema(INT, doc)
				case string(FLOAT):
					addScalarInSchema(FLOAT, doc)
				case string(ID):
					addScalarInSchema(ID, doc)
				case string(DATETIME):
					addScalarInSchema(DATETIME, doc)
				case string(STRING):
					addScalarInSchema(STRING, doc)
				case string(BOOLEAN):
					addScalarInSchema(BOOLEAN, doc)
				}
			}
		}
	}
}

func addScalarInSchema(sType SupportedScalars, doc *ast.SchemaDocument) {
	for _, def := range doc.Definitions {
		if def.Kind == "SCALAR" && strings.ToLower(def.Name) == string(sType) {
			return
		}
	}

	doc.Definitions = append(doc.Definitions, &ast.Definition{Kind: ast.Scalar, Name: string(sType)})
}
