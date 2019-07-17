package schema

import (
	"strings"

	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/gqlerror"
)

func init() {
	AddSchRule("OnlyTypeEnumInInitialSchema", dataTypeCheck)
	AddSchRule("OneIDPerType", idCountCheck)
	AddSchRule("TypeNameCantBeReservedKeyWords", nameCheck)
	AddSchRule("ValidListType", listValidityCheck)
	AddSchRule("DirectiveCheck", directivesCheck)
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
	var flag bool

	for _, typeVal := range sch.Definitions {
		flag = false
		for _, fields := range typeVal.Fields {
			if fields.Type.NamedType == "ID" {
				if flag {
					return gqlerror.Errorf("More than one ID field for type " + typeVal.Name)
				}

				flag = true
			}
		}
	}

	return nil
}

func nameCheck(sch *ast.SchemaDocument) *gqlerror.Error {

	for _, defn := range sch.Definitions {
		if isReservedKeyWord(defn.Name) {
			return gqlerror.ErrorPosf(defn.Position,
				defn.Name+" is reserved keyword. You can't declare type with this name")
		}
	}

	return nil
}

func listValidityCheck(sch *ast.SchemaDocument) *gqlerror.Error {

	for _, typ := range sch.Definitions {
		for _, fld := range typ.Fields {
			if fld.Type.Elem != nil && fld.Type.NonNull && !fld.Type.Elem.NonNull {
				return gqlerror.ErrorPosf(fld.Type.Position,
					"["+fld.Type.Name()+"]! type of lists are invalid")
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
			genArgumentsString(args))
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
