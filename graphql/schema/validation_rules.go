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
	"fmt"
	"strconv"

	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/gqlparser/v2/ast"
	"github.com/dgraph-io/gqlparser/v2/gqlerror"
	"github.com/dgraph-io/gqlparser/v2/validator"
)

var allowedFilters = []string{"StringHashFilter", "StringExactFilter", "StringFullTextFilter",
	"StringRegExpFilter", "StringTermFilter", "DateTimeFilter", "FloatFilter", "Int64Filter", "IntFilter", "PointGeoFilter",
	"ContainsFilter", "IntersectsFilter", "PolygonGeoFilter"}

func listInputCoercion(observers *validator.Events, addError validator.AddErrFunc) {
	observers.OnValue(func(walker *validator.Walker, value *ast.Value) {
		if value.Definition == nil || value.ExpectedType == nil {
			return
		}

		if value.Kind == ast.Variable {
			return
		}
		if value.ExpectedType.NamedType == IDType {
			value.Kind = ast.StringValue
		}
		// If the expected value is a list (ExpectedType.Elem != nil) && the value is not of list type,
		// then we need to coerce the value to a list, otherwise, we can return here as we do below.
		if !(value.ExpectedType.Elem != nil && value.Kind != ast.ListValue) {
			return
		}
		if value.ExpectedType.Elem.NamedType == IDType {
			value.Kind = ast.StringValue
		}
		val := *value
		child := &ast.ChildValue{Value: &val}
		valueNew := ast.Value{Children: []*ast.ChildValue{child}, Kind: ast.ListValue, Position: val.Position, Definition: val.Definition}
		*value = valueNew
	})
}

func filterCheck(observers *validator.Events, addError validator.AddErrFunc) {
	observers.OnValue(func(walker *validator.Walker, value *ast.Value) {
		if value.Definition == nil {
			return
		}

		if x.HasString(allowedFilters, value.Definition.Name) && len(value.Children) > 1 {
			addError(validator.Message("%s filter expects only one filter function, got: %d", value.Definition.Name, len(value.Children)), validator.At(value.Position))
		}
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
			fieldArg := directive.Arguments.ForName(cascadeArg)
			if fieldArg == nil {
				return
			}
			isVariable := fieldArg.Value.Kind == ast.Variable
			fieldsVal, _ := directive.ArgumentMap(walker.Variables)[cascadeArg].([]interface{})
			if len(fieldsVal) == 0 {
				return
			}
			var validatorPath ast.Path
			if isVariable {
				validatorPath = ast.Path{ast.PathName("variables")}
				validatorPath = append(validatorPath, ast.PathName(fieldArg.Value.Raw))

			}

			typFields := directive.ParentDefinition.Fields
			typName := directive.ParentDefinition.Name

			for _, value := range fieldsVal {
				v, ok := value.(string)
				if !ok {
					continue
				}
				if typFields.ForName(v) == nil {
					err := fmt.Sprintf("Field `%s` is not present in type `%s`."+
						" You can only use fields in cascade which are in type `%s`", value, typName, typName)
					if isVariable {
						validatorPath = append(validatorPath, ast.PathName(v))
						err = gqlerror.ErrorPathf(validatorPath, err).Error()
					}
					addError(validator.Message(err), validator.At(directive.Position))
					return
				}

			}

		}

	})
}

func intRangeCheck(observers *validator.Events, addError validator.AddErrFunc) {
	observers.OnValue(func(walker *validator.Walker, value *ast.Value) {
		if value.Definition == nil || value.ExpectedType == nil || value.Kind == ast.Variable || value.Kind == ast.ListValue {
			return
		}

		switch value.Definition.Name {
		case "Int":
			if value.Kind == ast.NullValue {
				return
			}
			_, err := strconv.ParseInt(value.Raw, 10, 32)
			if err != nil {
				if errors.Is(err, strconv.ErrRange) {
					addError(validator.Message("Out of range value '%s', for type `%s`",
						value.Raw, value.Definition.Name), validator.At(value.Position))
				} else {
					addError(validator.Message("%s", err), validator.At(value.Position))
				}
			}
		case "Int64":
			if value.Kind == ast.IntValue {
				_, err := strconv.ParseInt(value.Raw, 10, 64)
				if err != nil {
					if errors.Is(err, strconv.ErrRange) {
						addError(validator.Message("Out of range value '%s', for type `%s`",
							value.Raw, value.Definition.Name), validator.At(value.Position))
					} else {
						addError(validator.Message("%s", err), validator.At(value.Position))
					}
				}
				value.Kind = ast.StringValue
			} else {
				addError(validator.Message("Type mismatched for Value `%s`, expected: Int64, got: '%s'", value.Raw,
					valueKindToString(value.Kind)), validator.At(value.Position))
			}
		}
	})
}

func valueKindToString(valKind ast.ValueKind) string {
	switch valKind {
	case ast.Variable:
		return "Variable"
	case ast.StringValue:
		return "String"
	case ast.IntValue:
		return "Int"
	case ast.FloatValue:
		return "Float"
	case ast.BlockValue:
		return "Block"
	case ast.BooleanValue:
		return "Boolean"
	case ast.NullValue:
		return "Null"
	case ast.EnumValue:
		return "Enum"
	case ast.ListValue:
		return "List"
	case ast.ObjectValue:
		return "Object"
	}
	return ""
}
