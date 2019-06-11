package schemarules

import (
	. "github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/gqlerror"
	. "github.com/vektah/gqlparser/validator"
)

func init() {
	AddSchRule("OnlyTypeEnumInInitialSchema", func(schema *Schema) *gqlerror.Error {
		for _, typ := range schema.Types {
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
