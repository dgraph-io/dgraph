package schema

import (
	. "github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"

	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/gqlerror"
)

func init() {
	AddSchRule("OneIDPerType", func(sch *ast.SchemaDocument) *gqlerror.Error {
		var flag bool
		for _, typeVal := range sch.Definitions {
			flag = false
			for _, fields := range typeVal.Fields {
				if fields.Type.NamedType == "ID" {
					if flag {
						return &gqlerror.Error{
							Message: "More than one ID field for type " + typeVal.Name,
						}
					}

					flag = true
				}
			}
		}
		return nil
	})
}
