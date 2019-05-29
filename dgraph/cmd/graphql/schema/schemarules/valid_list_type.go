package schema

import (
	. "github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"

	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/gqlerror"
)

func init() {
	AddSchRule("OnlyTypeEnumInInitialSchema", func(sch *ast.SchemaDocument) *gqlerror.Error {
		for _, typ := range sch.Definitions {
			for _, fld := range typ.Fields {
				if fld.Type.Elem != nil && fld.Type.NonNull && !fld.Type.Elem.NonNull {
					return &gqlerror.Error{
						Message: "[" + fld.Type.Name() + "]! type of lists are invalid",
					}
				}
			}
		}

		return nil
	})
}
