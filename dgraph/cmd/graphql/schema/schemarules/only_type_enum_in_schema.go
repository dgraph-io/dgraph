package schemarules

import (
	"github.com/vektah/gqlparser/ast"
	. "github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/gqlerror"
	. "github.com/vektah/gqlparser/validator"
)

func init() {
	AddSchRule("OnlyTypeEnumInInitialSchema", func(schema *Schema) *gqlerror.Error {
		for _, typ := range schema.Types {
			if typ.Kind != ast.Object && typ.Kind != ast.Enum {
				return &gqlerror.Error{
					Message: "Only type and enums are allowed in initial schema.",
				}
			}

		}

		return nil
	})
}
