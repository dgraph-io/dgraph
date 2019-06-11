package schemarules

import (
	. "github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/gqlerror"
	. "github.com/vektah/gqlparser/validator"

	sch "github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
)

func init() {
	AddSchRule("TypeNameCheck", func(schema *Schema) *gqlerror.Error {
		for name := range schema.Types {
			if name == string(sch.INT) || name == string(sch.BOOLEAN) ||
				name == string(sch.FLOAT) || name == string(sch.STRING) ||
				name == string(sch.DATETIME) || name == string(sch.ID) {
				return &gqlerror.Error{
					Message: name + " is reserved keyword. You can't declare" +
						"type with this name",
				}
			}
		}

		return nil
	})
}
