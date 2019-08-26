package validator

import (
	"github.com/vektah/gqlparser/ast"
	. "github.com/vektah/gqlparser/validator"
)

func init() {
	AddRule("UniqueVariableNames", func(observers *Events, addError AddErrFunc) {
		observers.OnOperation(func(walker *Walker, operation *ast.OperationDefinition) {
			seen := map[string]bool{}
			for _, def := range operation.VariableDefinitions {
				if seen[def.Variable] {
					addError(
						Message(`There can be only one variable named "%s".`, def.Variable),
						At(def.Position),
					)
				}
				seen[def.Variable] = true
			}
		})
	})
}
