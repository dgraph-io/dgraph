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
	AddPreRule("OnlyTypeEnumInInitialSchema", dataTypeCheck)
	AddPreRule("TypeNameCantBeReservedKeyWords", nameCheck)

	AddPostRule("OneIDPerType", idCountCheck)
	AddPostRule("ValidListType", listValidityCheck)
	AddPostRule("DirectiveCheck", directivesCheck)
}

func directivesCheck(sch *ast.Schema) *gqlerror.Error {

	for _, typ := range sch.Types {
		for _, fld := range typ.Fields {
			for _, direc := range fld.Directives {

				if err := validateDirectiveArguments(typ, fld, direc, sch); err != nil {
					return err
				}

			}
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

func idCountCheck(sch *ast.Schema) *gqlerror.Error {
	var found bool
	for _, typeVal := range sch.Types {
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
func listValidityCheck(sch *ast.Schema) *gqlerror.Error {
	for _, typ := range sch.Types {
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

func checkHasInverseArgs(typ *ast.Definition, fld *ast.FieldDefinition,
	directive *ast.Directive, sch *ast.Schema) *gqlerror.Error {
	args := directive.Arguments

	// @hasInverse should have only one argument i.e "field"
	if len(args) != 1 {
		return gqlerror.Errorf("Invalid number of arguments to hasInverse\n" +
			genDirectiveArgumentsString(args))
	}

	fldArg := args.ForName(string(FIELD))
	if fldArg == nil {
		return gqlerror.ErrorPosf(directive.Position, "Invalid argument to hasInverse")
	}

	// Value of field argument should be of the form "typ.fld". typ should be
	// one of the types in schema. fld should be one of the fields of typ.
	// e.g. @hasInverse(field: "Posts.author")
	splitVal := strings.Split(fldArg.Value.Raw, ".")
	if len(splitVal) < 2 {
		return gqlerror.ErrorPosf(fldArg.Position, "Invalid value of field argument")
	}

	invrseTypeName := splitVal[0]
	invrseFldName := splitVal[1]

	if invType, ok := sch.Types[invrseTypeName]; ok {
		if invFld := invType.Fields.ForName(invrseFldName); invFld != nil {

			// Type of the field to which a directive is pointing should be same as the
			// type contains the field containing directive. e.g.
			//
			// type Post { author: Author @hasInverse(field: "Author.posts") }
			// type Author { posts: [Posts!]! @hasInverse(field: "Post.author") }
			if invFld.Type.Name() != typ.Name {
				return gqlerror.ErrorPosf(
					fld.Position, "Inverse type doesn't match %s",
					typ.Name,
				)
			}

			if invFld.Directives == nil {
				return gqlerror.ErrorPosf(
					fld.Position, "Inverse of %s: %s, doesn't have inverse directive pointing back",
					fld.Name, fldArg.Value.Raw,
				)
			}

			if invDirective := invFld.Directives.ForName(string(HASINVERSE)); invDirective != nil {
				if invFldArg := invDirective.Arguments.ForName(string(FIELD)); invFldArg != nil {
					invSplitVal := strings.Split(invFldArg.Value.Raw, ".")
					if len(invSplitVal) == 2 &&
						!(invSplitVal[0] == typ.Name && invSplitVal[1] == fld.Name) {
						return gqlerror.ErrorPosf(
							fld.Position,
							"Inverse of %s: %s, doesn't have inverse directive pointing back",
							fld.Name, fldArg.Value.Raw,
						)
					}
				}
			} else {
				return gqlerror.ErrorPosf(
					fld.Position, "Inverse of %s: %s, doesn't have inverse directive pointing back",
					fld.Name, fldArg.Value.Raw,
				)
			}

		} else {
			return gqlerror.ErrorPosf(
				fld.Position, "@hasInverse: Specified field %s doesn't exist",
				invrseFldName,
			)
		}
	} else {
		return gqlerror.ErrorPosf(
			fld.Position, "@hasInverse: Specified type %s doesn't exist",
			invrseTypeName,
		)
	}

	return nil
}

func validateDirectiveArguments(typ *ast.Definition,
	fld *ast.FieldDefinition, direc *ast.Directive, sch *ast.Schema) *gqlerror.Error {

	if direc.Name == string(HASINVERSE) {
		return checkHasInverseArgs(typ, fld, direc, sch)
	}

	return nil
}
