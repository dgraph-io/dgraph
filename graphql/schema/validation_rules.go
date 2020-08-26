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
	"errors"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/validator"
	"math"
	"strconv"
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

func intRangeCheck(observers *validator.Events, addError validator.AddErrFunc) {
	observers.OnValue(func(walker *validator.Walker, value *ast.Value) {
		if value.Definition == nil || value.ExpectedType == nil {
			return
		}

		if value.Kind != ast.IntValue {
			return
		}

		var err error
		var val int64
		switch value.Definition.Name {
		case "Int":
			_, err = strconv.ParseInt(value.Raw, 10, 32)
		case "Int64":
			val, err = strconv.ParseInt(value.Raw, 10, 54)
		default:
			return

		}
		//Range of Json numbers is [-(2**53)+1, (2**53)-1] while of 54 bit integers is [-(2**53), (2**53)-1]
		if err != nil {
			if float64(val) == (-1)*math.Pow(2, 53) || errors.Is(err, strconv.ErrRange) {
				addError(validator.Message("Out of range value '%s', for type `%s`",
					value.Raw, value.Definition.Name), validator.At(value.Position))
				return
			}
			addError(validator.Message("%s", err), validator.At(value.Position))
			return
		}

	})
}
