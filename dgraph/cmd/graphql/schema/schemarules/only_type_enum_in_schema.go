package schema

import (
	. "github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"

	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/gqlerror"
)

func init() {
	AddSchRule("OnlyTypeEnumInInitialSchema", func(sch *ast.SchemaDocument) *gqlerror.Error {
		for _, typ := range sch.Definitions {
			if typ.Kind != ast.Object && typ.Kind != ast.Enum {
				return &gqlerror.Error{
					Message: "Only type and enums are allowed in initial schema.",
				}
			}
		}

		return nil
	})
}
