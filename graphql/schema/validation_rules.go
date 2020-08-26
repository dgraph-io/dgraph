/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package schema

import (
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/validator"
)

func listTypeCheck(observers *validator.Events, addError validator.AddErrFunc) {
	observers.OnValue(func(walker *validator.Walker, value *ast.Value) {
		if value.Definition == nil || value.ExpectedType == nil {
			return
		}

		if value.Kind == ast.Variable {
			return
		}

		// ExpectedType.Elem will be not nil if it is of list type. Otherwise
		// it will be nil. So it's safe to say that value.Kind should be list
		if !(value.ExpectedType.Elem != nil && value.Kind != ast.ListValue) {
			return
		}

		addError(validator.Message("Value provided %s is incompatible with expected type %s",
			value.String(),
			value.ExpectedType.String()), validator.At(value.Position))
	})
}

func variableTypeCheck(observers *validator.Events, addError validator.AddErrFunc) {
	observers.OnValue(func(walker *validator.Walker, value *ast.Value) {
		if value.Definition == nil || value.ExpectedType == nil ||
			value.VariableDefinition == nil {
			return
		}

		if value.Kind != ast.Variable {
			return
		}

		if value.VariableDefinition.Type.IsCompatible(value.ExpectedType) {
			return
		}

		addError(validator.Message("Variable type provided %s is incompatible with expected type %s",
			value.VariableDefinition.Type.String(),
			value.ExpectedType.String()), validator.At(value.Position))
	})
}

func directiveArgumentsCheck(observers *validator.Events, addError validator.AddErrFunc) {
	observers.OnDirective(func(walker *validator.Walker, directive *ast.Directive) {

		if directive.Name == cascadeDirective && len(directive.Arguments) == 1 {
			if directive.ParentDefinition == nil {
				addError(validator.Message("Schema is not set yet. Please try after sometime."))
				return
			}
			if directive.Arguments.ForName(cascadeArg) == nil {
				return
			}
			typFields := directive.ParentDefinition.Fields
			for _, child := range directive.Arguments.ForName(cascadeArg).Value.Children {
				if typFields.ForName(child.Value.Raw) == nil {
					addError(validator.Message("Field `%s` is not present in type `%s`. You can only use fields which are in type `%s`",
						child.Value.Raw, directive.ParentDefinition.Name, directive.ParentDefinition.Name))
					return
				}

			}

		}

	})
}
