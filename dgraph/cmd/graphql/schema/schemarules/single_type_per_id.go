package schema

import (
	. "github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/gqlerror"
	. "github.com/vektah/gqlparser/validator"
)

func init() {
	AddSchRule("OneIDPerType", func(schema *Schema) *gqlerror.Error {
		var counter int
		for _, typeVal := range schema.PossibleTypes {
			counter = 0
			tmpTypeVal := typeVal[0] // IDK why schema.PossibleTypes[type] is an array. Should be a value acc to me
			for _, fields := range tmpTypeVal.Fields {
				if fields.Type.NamedType == "ID" {
					counter++
				}
			}
			if counter > 1 {
				err := &gqlerror.Error{Message: "More than one ID field for type " + tmpTypeVal.Name}
				return err
			}
		}
		return nil
	})
}
