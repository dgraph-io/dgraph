package schema

import (
	. "github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/gqlerror"
	. "github.com/vektah/gqlparser/validator"
)

func init() {
	AddSchRule("OneIDPerType", func(schema *Schema) *gqlerror.Error {
		var flag bool
		for _, typeVal := range schema.Types {
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
