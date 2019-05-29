package schema

import (
	. "github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"

	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/gqlerror"
)

func init() {
	AddSchRule("TypeNameCantBeReservedKeyWords", func(sch *ast.SchemaDocument) *gqlerror.Error {
		for _, defn := range sch.Definitions {
			if isReservedKeyWord(defn.Name) {
				return &gqlerror.Error{
					Message: defn.Name + " is reserved keyword. You can't declare" +
						"type with this name",
				}
			}
		}

		return nil
	})
}

func isReservedKeyWord(name string) bool {
	if name == string(INT) || name == string(BOOLEAN) ||
		name == string(FLOAT) || name == string(STRING) ||
		name == string(DATETIME) || name == string(ID) || name == "Query" || name == "Mutation" {
		return true
	}

	return false
}
