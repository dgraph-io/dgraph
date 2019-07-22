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
	"fmt"
	"strings"

	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/gqlerror"
)

func init() {
	AddRule("OnlyTypeEnumInInitialSchema", dataTypeCheck)
	AddRule("OneIDPerType", idCountCheck)
	AddRule("TypeNameCantBeReservedKeyWords", nameCheck)
	AddRule("ValidListType", listValidityCheck)
	AddRule("DirectiveCheck", directivesCheck)
}

func directivesCheck(sch *ast.SchemaDocument) *gqlerror.Error {
	inverseMap := make(map[string][]string)

	for _, typ := range sch.Definitions {
		for _, fld := range typ.Fields {
			for _, direc := range fld.Directives {
				if !isSupportedFieldDirective(direc) {
					return gqlerror.ErrorPosf(direc.Position, "Unsupported diretive "+direc.Name)
				}

				if err := validateDirectiveArguments(typ, fld, direc, sch); err != nil {
					return err
				}

				if _, ok := inverseMap[typ.Name+"."+fld.Name]; !ok {
					inverseMap[typ.Name+"."+fld.Name] = make([]string, 0)
				}

				for _, arg := range direc.Arguments {
					if arg.Name == "field" {
						inverseMap[typ.Name+"."+fld.Name] =
							append(inverseMap[typ.Name+"."+fld.Name], arg.Value.Raw)
					}
				}
			}
		}
	}

	for key, vals := range inverseMap {
		for _, val := range vals {

			if _, ok := inverseMap[val]; !ok {
				return gqlerror.Errorf("Inverse link doesn't exists for " + key + " and " + val)
			}

			for _, invVal := range inverseMap[val] {
				if invVal == key {
					return nil
				}
			}

			return gqlerror.Errorf("Inverse link doesn't exists for " + key + " and " + val)
		}
	}

	return nil
}

func dataTypeCheck(sch *ast.SchemaDocument) *gqlerror.Error {
	for _, typ := range sch.Definitions {
		if typ.Kind != ast.Object && typ.Kind != ast.Enum {
			return gqlerror.ErrorPosf(typ.Position,
				"Only type and enums are allowed in initial schema.")
		}
	}

	return nil
}

func idCountCheck(sch *ast.SchemaDocument) *gqlerror.Error {
	var found bool
	for _, typeVal := range sch.Definitions {
		found = false
		for _, fld := range typeVal.Fields {
			if isIDField(fld) {
				if found {
					return gqlerror.ErrorPosf(
						fld.Position,
						fmt.Sprintf("More than one ID field found for type %s", typeVal.Name),
					)
				}

				found = true
			}
		}
	}

	return nil
}

func nameCheck(sch *ast.SchemaDocument) *gqlerror.Error {
	for _, defn := range sch.Definitions {
		if isReservedKeyWord(defn.Name) {
			return gqlerror.ErrorPosf(
				defn.Position,
				fmt.Sprintf(
					"%s is reserved keyword. You can't declare type with this name", defn.Name,
				),
			)
		}
	}

	return nil
}

// [Posts]! -> invalid, [Posts!]! -> valid
func listValidityCheck(sch *ast.SchemaDocument) *gqlerror.Error {
	for _, typ := range sch.Definitions {
		for _, fld := range typ.Fields {
			if fld.Type.Elem != nil && fld.Type.NonNull && !fld.Type.Elem.NonNull {
				return gqlerror.ErrorPosf(
					fld.Position,
					fmt.Sprintf("[%s]! type of lists are invalid", fld.Type.Name()),
				)
			}
		}
	}

	return nil
}

func isReservedKeyWord(name string) bool {
	if name == string(INT) || name == string(BOOLEAN) ||
		name == string(FLOAT) || name == string(STRING) ||
		name == string(DATETIME) || name == string(ID) || name == "Query" || name == "Mutation" {
		return true
	}

	return false
}

func isSupportedFieldDirective(direc *ast.Directive) bool {

	if direc.Name == "hasInverse" {
		return true
	}

	return false
}

func checkHasInverseArgs(direcTyp *ast.Definition, fld *ast.FieldDefinition,
	args ast.ArgumentList, sch *ast.SchemaDocument) *gqlerror.Error {
	if len(args) != 1 {
		return gqlerror.Errorf("Invalid number of arguments to hasInverse\n" +
			genDirectiveArgumentsString(args))
	}

	arg := args[0]
	if arg.Name == "field" {
		splitVal := strings.Split(arg.Value.Raw, ".")
		if len(splitVal) < 2 {
			return gqlerror.ErrorPosf(arg.Position, "Invalid inverse field")
		}

		invrseType := splitVal[0]
		invrseFld := splitVal[1]

		for _, defn := range sch.Definitions {
			if defn.Name == invrseType && defn.Kind == ast.Object {
				for _, fld := range defn.Fields {
					if fld.Name == invrseFld {
						if direcTyp.Name == fld.Type.Name() ||
							direcTyp.Name == fld.Type.Elem.Name() {
							return nil
						}

						return gqlerror.ErrorPosf(arg.Position,
							"Inverse type doesn't match "+direcTyp.Name)
					}
				}

				return gqlerror.ErrorPosf(arg.Position, "Undefined field "+invrseFld)
			}
		}

		return gqlerror.ErrorPosf(arg.Position, "Undefined type "+invrseType)
	}

	return gqlerror.ErrorPosf(arg.Position, "Invalid argument to hasInverse")
}

func validateDirectiveArguments(typ *ast.Definition, fld *ast.FieldDefinition, direc *ast.Directive, sch *ast.SchemaDocument) *gqlerror.Error {

	if direc.Name == "hasInverse" {
		return checkHasInverseArgs(typ, fld, direc.Arguments, sch)
	}

	return nil
}
