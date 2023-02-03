/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
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
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/pkg/errors"

	"github.com/dgraph-io/gqlparser/v2/ast"
)

func validateUrl(rawURL string) error {
	u, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return err
	}

	if u.RawQuery != "" {
		return fmt.Errorf("POST method cannot have query parameters in url: %s", rawURL)
	}
	return nil
}

type IntrospectionRequest struct {
	Query string `json:"query"`
}

// introspectRemoteSchema introspectes remote schema
func introspectRemoteSchema(url string, headers http.Header) (*introspectedSchema, error) {
	if err := validateUrl(url); err != nil {
		return nil, err
	}
	param := &IntrospectionRequest{
		Query: introspectionQuery,
	}

	body, err := json.Marshal(param)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k := range headers {
		req.Header.Set(k, headers.Get(k))
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	result := &introspectedSchema{}
	return result, errors.Wrapf(json.Unmarshal(body, result),
		"while json unmarshaling result from remote introspection query")
}

const (
	list        = "LIST"
	nonNull     = "NON_NULL"
	inputObject = string(ast.InputObject)
)

const introspectionQuery = `
	query {
	  __schema {
		queryType { name }
		mutationType { name }
		subscriptionType { name }
		types {
		  ...FullType
		}
		directives {
		  name
		  locations
		  args {
			...InputValue
		  }
		}
	  }
	}
	fragment FullType on __Type {
	  kind
	  name
	  fields(includeDeprecated: true) {
		name
		args {
		  ...InputValue
		}
		type {
		  ...TypeRef
		}
		isDeprecated
		deprecationReason
	  }
	  inputFields {
		...InputValue
	  }
	  interfaces {
		...TypeRef
	  }
	  enumValues(includeDeprecated: true) {
		name
		isDeprecated
		deprecationReason
	  }
	  possibleTypes {
		...TypeRef
	  }
	}
	fragment InputValue on __InputValue {
	  name
	  type { ...TypeRef }
	  defaultValue
	}
	fragment TypeRef on __Type {
	  kind
	  name
	  ofType {
		kind
		name
		ofType {
		  kind
		  name
		  ofType {
			kind
			name
			ofType {
			  kind
			  name
			  ofType {
				kind
				name
				ofType {
				  kind
				  name
				  ofType {
					kind
					name
				  }
				}
			  }
			}
		  }
		}
	  }
	}
  `

// remoteGraphqlMetadata represents the minimal set of data that is required to validate the graphql
// given in @custom->http->graphql with the remote server
type remoteGraphqlMetadata struct {
	// parentType is the type which contains the field on which @custom is applied
	parentType *ast.Definition
	// parentField refers to the field on which @custom is applied
	parentField *ast.FieldDefinition
	// graphqlOpDef is the Operation Definition for the operation given in @custom->http->graphql
	// The operation can only be a query or mutation
	graphqlOpDef *ast.OperationDefinition
	// isBatch tells whether it is SINGLE/BATCH mode for resolving custom fields
	isBatch bool
	// url is the url of remote graphql endpoint
	url string
	// headers sent to the remote graphql endpoint for introspection
	headers http.Header
	// schema given by the user.
	schema *ast.Schema
}

// argMatchingMetadata represents all the info needed for the purpose of matching the argument
// signature of given @custom graphql query with remote query.
type argMatchingMetadata struct {
	// givenArgVals is the mapping of argName -> argValue (*ast.Value), for given query.
	// The value is allowed to be a Variable, Object or a List of Objects at the moment.
	givenArgVals map[string]*ast.Value
	// givenVarTypes is the mapping of varName -> type (*ast.Type), for the variables used in given
	// query. For @custom fields, these are fetched from the parent type of the field.
	// For @custom query/mutation these are fetched from the args of that query/mutation.
	givenVarTypes map[string]*ast.Type
	// remoteArgMd represents the arguments of the remote query.
	remoteArgMd *remoteArgMetadata
	// remoteTypes is the mapping of typename -> typeDefinition for all the types present in
	// introspection response for remote query.
	remoteTypes map[string]*types
	// givenQryName points to the name of the given query. Used while reporting errors.
	givenQryName *string
	// operationType points to the name of the given operation (i.e., query or mutation).
	// Used while reporting errors.
	operationType *string
	// local GraphQL schema supplied by the user
	schema *ast.Schema
}

// remoteArgMetadata represents all the arguments in the remote query.
// It is used for the purpose of matching the argument signature for remote query.
type remoteArgMetadata struct {
	// typMap is the mapping of arg typename -> typeDefinition obtained from introspection
	typMap map[string]*gqlType
	// requiredArgs is the list of NON_NULL args in remote query
	requiredArgs []string
}

// validates the graphql given in @custom->http->graphql by introspecting remote schema.
// It assumes that the graphql syntax is correct, only remote validation is needed.
func validateRemoteGraphql(metadata *remoteGraphqlMetadata) error {
	remoteIntrospection, err := introspectRemoteSchema(metadata.url, metadata.headers)
	if err != nil {
		return err
	}

	var remoteQueryTypename string
	operationType := string(metadata.graphqlOpDef.Operation)
	switch operationType {
	case "query":
		if remoteIntrospection.Data.Schema.QueryType == nil {
			return errors.Errorf("remote schema doesn't have any queries.")
		}
		remoteQueryTypename = remoteIntrospection.Data.Schema.QueryType.Name
	case "mutation":
		if remoteIntrospection.Data.Schema.MutationType == nil {
			return errors.Errorf("remote schema doesn't have any mutations.")
		}
		remoteQueryTypename = remoteIntrospection.Data.Schema.MutationType.Name
	default:
		// this case is not possible as we are validating the operation to be query/mutation in
		// @custom directive validation
		return errors.Errorf("found `%s` operation, it can only have query/mutation.", operationType)
	}

	remoteTypes := make(map[string]*types)
	for _, typ := range remoteIntrospection.Data.Schema.Types {
		remoteTypes[typ.Name] = typ
	}

	remoteQryType, ok := remoteTypes[remoteQueryTypename]
	if !ok {
		return missingRemoteTypeError(remoteQueryTypename)
	}

	// check whether given query/mutation is present in remote schema
	var introspectedRemoteQuery *gqlField
	givenQuery := metadata.graphqlOpDef.SelectionSet[0].(*ast.Field)
	for _, remoteQuery := range remoteQryType.Fields {
		if remoteQuery.Name == givenQuery.Name {
			introspectedRemoteQuery = remoteQuery
			break
		}
	}
	if introspectedRemoteQuery == nil {
		return errors.Errorf("%s `%s` is not present in remote schema.",
			operationType, givenQuery.Name)
	}

	// check whether the return type of remote query is same as the required return type
	expectedReturnType := metadata.parentField.Type.String()
	gotReturnType := introspectedRemoteQuery.Type.String()
	if metadata.isBatch {
		expectedReturnType = fmt.Sprintf("[%s]", expectedReturnType)
	}
	if expectedReturnType != gotReturnType {
		return errors.Errorf("found return type mismatch for %s `%s`, expected `%s`, got `%s`.",
			operationType, givenQuery.Name, expectedReturnType, gotReturnType)
	}
	// Deep check the remote return type.
	if err := matchDeepTypes(introspectedRemoteQuery.Type, remoteTypes,
		metadata.schema); err != nil {
		return err
	}

	givenQryArgVals := getGivenQueryArgValsAsMap(givenQuery)
	givenQryVarTypes := getVarTypesAsMap(metadata.parentField, metadata.parentType)
	remoteQryArgMetadata := getRemoteQueryArgMetadata(introspectedRemoteQuery)

	// verify remote query arg format for BATCH mode
	if metadata.isBatch {
		if len(remoteQryArgMetadata.typMap) != 1 {
			return errors.Errorf("remote %s `%s` accepts %d arguments, It must have only one "+
				"argument of the form `[{param1: $var1, param2: $var2, ...}]` for BATCH mode.",
				operationType, givenQuery.Name, len(remoteQryArgMetadata.typMap))
		}
		for argName, inputTyp := range remoteQryArgMetadata.typMap {
			if !((inputTyp.Kind == list && inputTyp.OfType != nil && inputTyp.OfType.
				Kind == inputObject) ||
				(inputTyp.Kind == list && inputTyp.OfType != nil && inputTyp.OfType.
					Kind == nonNull && inputTyp.OfType.OfType != nil && inputTyp.OfType.OfType.
					Kind == inputObject) ||
				(inputTyp.Kind == nonNull && inputTyp.OfType != nil && inputTyp.OfType.
					Kind == list && inputTyp.OfType.OfType != nil && inputTyp.OfType.OfType.
					Kind == inputObject) ||
				(inputTyp.Kind == nonNull && inputTyp.OfType != nil && inputTyp.OfType.
					Kind == list && inputTyp.OfType.OfType != nil && inputTyp.OfType.OfType.
					Kind == nonNull && inputTyp.OfType.OfType.OfType != nil && inputTyp.
					OfType.OfType.OfType.Kind == inputObject)) {
				return errors.Errorf("argument `%s` for given %s `%s` must be of the form `[{param1"+
					": $var1, param2: $var2, ...}]` for BATCH mode in remote %s.", argName,
					operationType, givenQuery.Name, operationType)
			}
			inputTypName := inputTyp.NamedType()
			typ, ok := remoteTypes[inputTypName]
			if !ok {
				return missingRemoteTypeError(inputTypName)
			}
			if typ.Kind != inputObject {
				return errors.Errorf("type %s in remote schema is not an INPUT_OBJECT.", inputTypName)
			}
		}
	}

	if !metadata.isBatch {
		// check whether args of given query/mutation match the args of remote query/mutation
		err = matchArgSignature(&argMatchingMetadata{
			givenArgVals:  givenQryArgVals,
			givenVarTypes: givenQryVarTypes,
			remoteArgMd:   remoteQryArgMetadata,
			remoteTypes:   remoteTypes,
			givenQryName:  &givenQuery.Name,
			operationType: &operationType,
			schema:        metadata.schema,
		})
	}

	return err
}

func missingRemoteTypeError(typName string) error {
	return errors.Errorf("remote schema doesn't have any type named %s.", typName)
}

func matchDeepTypes(remoteType *gqlType, remoteTypes map[string]*types,
	localSchema *ast.Schema) error {
	_, err := expandType(remoteType, remoteTypes)
	if err != nil {
		return err
	}
	return matchRemoteTypes(localSchema, remoteTypes)
}

func matchRemoteTypes(schema *ast.Schema, remoteTypes map[string]*types) error {
	for typeName, def := range schema.Types {
		origTyp := schema.Types[typeName]
		remoteDir := origTyp.Directives.ForName(remoteDirective)
		if remoteDir != nil {
			{
				remoteType, ok := remoteTypes[def.Name]
				fields := def.Fields
				if !ok {
					return errors.Errorf(
						"Unable to find local type %s in the remote schema",
						typeName,
					)
				}
				remoteFields := remoteType.Fields
				if remoteFields == nil {
					// Get fields for INPUT_OBJECT
					remoteFields = remoteType.InputFields
				}
				for _, field := range fields {
					var remoteField *gqlField = nil
					for _, rf := range remoteFields {
						if rf.Name == field.Name {
							remoteField = rf
						}
					}
					if remoteField == nil {
						return errors.Errorf(
							"%s field for the local type %s is not present in the remote type %s",
							field.Name, typeName, remoteType.Name,
						)
					}
					if remoteField.Type.String() != field.Type.String() {
						return errors.Errorf(
							"expected type for the field %s is %s but got %s in type %s",
							remoteField.Name,
							remoteField.Type.String(),
							field.Type.String(),
							typeName,
						)
					}
				}
			}
		}
	}
	return nil
}

// matchArgSignature matches the type signature for arguments supplied in metadata with
// corresponding remote arguments
func matchArgSignature(md *argMatchingMetadata) error {
	// TODO: maybe add path information in error messages,
	// so they are more informative. Like if query was `query { userNames(car: {age: $var}) }`,
	// then for errors on `age`, we shouldn't just be putting `age` in error messages,
	// instead we should put `car.age`
	for givenArgName, givenArgVal := range md.givenArgVals {
		remoteArgTyp, ok := md.remoteArgMd.typMap[givenArgName]
		if !ok {
			return errors.Errorf("argument `%s` is not present in remote %s `%s`.",
				givenArgName, *md.operationType, *md.givenQryName)
		}

		switch givenArgVal.Kind {
		case ast.Variable:
			givenArgTyp, ok := md.givenVarTypes[givenArgVal.Raw]
			if !ok {
				return errors.Errorf("variable $%s is missing in given context.", givenArgVal.Raw)
			}
			// TODO: we will consider ID as String for the purpose of type matching
			//rootType := givenArgTyp
			//for rootType.NamedType == "" {
			//	rootType = rootType.Elem
			//}
			//if rootType.NamedType == "ID" {
			//	rootType.NamedType = "String"
			//}
			expectedArgType := givenArgTyp.String()
			gotArgType := remoteArgTyp.String()
			if expectedArgType != gotArgType {
				return errors.Errorf("found type mismatch for variable `$%s` in %s `%s`, expected"+
					" `%s`, got `%s`.", givenArgVal.Raw, *md.operationType, *md.givenQryName,
					expectedArgType, gotArgType)
			}
			// deep check the remote type and verify it with the local schema.
			if err := matchDeepTypes(remoteArgTyp, md.remoteTypes, md.schema); err != nil {
				return err
			}
		case ast.ObjectValue:
			if !(remoteArgTyp.Kind == inputObject || (remoteArgTyp.
				Kind == nonNull && remoteArgTyp.OfType != nil && remoteArgTyp.OfType.
				Kind == inputObject)) {
				return errors.Errorf("object value supplied for argument `%s` in %s `%s`, "+
					"but remote argument doesn't accept INPUT_OBJECT.", givenArgName,
					*md.operationType, *md.givenQryName)
			}
			remoteObjTypname := remoteArgTyp.NamedType()
			remoteObjTyp, ok := md.remoteTypes[remoteObjTypname]
			if !ok {
				return missingRemoteTypeError(remoteObjTypname)
			}
			if err := matchArgSignature(&argMatchingMetadata{
				givenArgVals:  getObjChildrenValsAsMap(givenArgVal),
				givenVarTypes: md.givenVarTypes,
				remoteArgMd:   getRemoteTypeFieldsMetadata(remoteObjTyp),
				remoteTypes:   md.remoteTypes,
				givenQryName:  md.givenQryName,
				operationType: md.operationType,
				schema:        md.schema,
			}); err != nil {
				return err
			}
		case ast.ListValue:
			if !((remoteArgTyp.Kind == list && remoteArgTyp.OfType != nil) || (remoteArgTyp.
				Kind == nonNull && remoteArgTyp.OfType != nil && remoteArgTyp.OfType.
				Kind == list && remoteArgTyp.OfType.OfType != nil)) {
				return errors.Errorf("LIST value supplied for argument `%s` in %s `%s`, "+
					"but remote argument doesn't accept LIST.", givenArgName, *md.operationType,
					*md.givenQryName)
			}
			remoteListElemTypname := remoteArgTyp.NamedType()
			remoteObjTyp, ok := md.remoteTypes[remoteListElemTypname]
			if !ok {
				return missingRemoteTypeError(remoteListElemTypname)
			}
			if remoteObjTyp.Kind != inputObject {
				return errors.Errorf("argument `%s` in %s `%s` of List kind has non-object"+
					" elements in remote %s, Lists can have only INPUT_OBJECT as element.",
					givenArgName, *md.operationType, *md.givenQryName, *md.operationType)
			}
			remoteObjChildMap := getRemoteTypeFieldsMetadata(remoteObjTyp)
			for _, givenElem := range givenArgVal.Children {
				if givenElem.Value.Kind != ast.ObjectValue {
					return errors.Errorf("argument `%s` in %s `%s` of List kind has non-object"+
						" elements, Lists can have only objects as element.", givenArgName,
						*md.operationType, *md.givenQryName)
				}
				if err := matchArgSignature(&argMatchingMetadata{
					givenArgVals:  getObjChildrenValsAsMap(givenElem.Value),
					givenVarTypes: md.givenVarTypes,
					remoteArgMd:   remoteObjChildMap,
					remoteTypes:   md.remoteTypes,
					givenQryName:  md.givenQryName,
					operationType: md.operationType,
					schema:        md.schema,
				}); err != nil {
					return err
				}
			}
		default:
			return errors.Errorf("scalar value supplied for argument `%s` in %s `%s`, "+
				"only Variable, Object, or List values are allowed.", givenArgName,
				*md.operationType, *md.givenQryName)

		}
	}

	// check all non-null args required by remote query/mutation are present in given query/mutation
	for _, remoteArgName := range md.remoteArgMd.requiredArgs {
		_, ok := md.givenArgVals[remoteArgName]
		if !ok {
			return errors.Errorf("argument `%s` in %s `%s` is missing, it is required by remote"+
				" %s.", remoteArgName, *md.operationType, *md.givenQryName, *md.operationType)
		}
	}

	return nil
}

type expandTypeParams struct {
	// expandedTypes tells whether a type has already been expanded or not.
	// If a key with typename is present in this map, it means that type has been expanded.
	expandedTypes map[string]struct{}
	// remoteTypes is the mapping of typename -> typeDefinition for all the types present in
	// introspection response for remote query.
	remoteTypes map[string]*types
	// typesToFields is the mapping of typename -> fieldDefinitions for the types present in
	// introspection response for remote query.
	typesToFields map[string][]*gqlField
}

func expandTypeRecursively(typenameToExpand string, param *expandTypeParams) error {
	_, alreadyExpanded := param.expandedTypes[typenameToExpand]
	if alreadyExpanded {
		return nil
	}
	// We're marking this to avoid recursive expansion.
	param.expandedTypes[typenameToExpand] = struct{}{}
	typeFound := false
	for _, typ := range param.remoteTypes {
		if typ.Name == typenameToExpand {
			typeFound = true
			param.typesToFields[typ.Name] = make([]*gqlField, 0,
				len(typ.Fields)+len(typ.InputFields))
			param.typesToFields[typ.Name] = append(param.typesToFields[typ.Name],
				typ.Fields...)
			param.typesToFields[typ.Name] = append(param.typesToFields[typ.Name],
				typ.InputFields...)
			// Expand the non scalar types.
			for _, field := range param.typesToFields[typ.Name] {
				if !isGraphqlSpecScalar(field.Type.Name) {
					// expand this field.
					err := expandTypeRecursively(field.Type.NamedType(), param)
					if err != nil {
						return err
					}
				}
			}
		}
	}
	if !typeFound {
		return errors.Errorf("Unable to find the type %s on the remote schema", typenameToExpand)
	}
	return nil

}

// expandType will expand the nested type into flat structure. For eg. Country having a filed called
// states of type State is expanded as Country and State. Scalar fields won't be expanded.
// It also expands deep nested types.
func expandType(typeToBeExpanded *gqlType,
	remoteTypes map[string]*types) (map[string][]*gqlField, error) {
	if isGraphqlSpecScalar(typeToBeExpanded.NamedType()) {
		return nil, nil
	}

	param := &expandTypeParams{
		expandedTypes: make(map[string]struct{}),
		typesToFields: make(map[string][]*gqlField),
		remoteTypes:   remoteTypes,
	}
	// Expand the types that are required to do a query.
	err := expandTypeRecursively(typeToBeExpanded.NamedType(), param)
	if err != nil {
		return nil, err
	}
	return param.typesToFields, nil
}

func getObjChildrenValsAsMap(object *ast.Value) map[string]*ast.Value {
	childValMap := make(map[string]*ast.Value)

	for _, val := range object.Children {
		childValMap[val.Name] = val.Value
	}
	return childValMap
}

func getRemoteTypeFieldsMetadata(remoteTyp *types) *remoteArgMetadata {
	md := &remoteArgMetadata{
		typMap:       make(map[string]*gqlType),
		requiredArgs: make([]string, 0),
	}
	fields := make([]*gqlField, 0, len(remoteTyp.Fields)+len(remoteTyp.InputFields))
	fields = append(fields, remoteTyp.Fields...)
	fields = append(fields, remoteTyp.InputFields...)

	for _, field := range fields {
		md.typMap[field.Name] = field.Type
		if field.Type.Kind == nonNull {
			md.requiredArgs = append(md.requiredArgs, field.Name)
		}
	}
	return md
}

func getVarTypesAsMap(parentField *ast.FieldDefinition,
	parentType *ast.Definition) map[string]*ast.Type {
	typMap := make(map[string]*ast.Type)
	if isQueryOrMutationType(parentType) {
		// this is the case of @custom on some Query or Mutation
		for _, v := range parentField.Arguments {
			typMap[v.Name] = v.Type
		}
	} else {
		// this is the case of @custom on fields inside some user defined type
		for _, v := range parentType.Fields {
			typMap[v.Name] = v.Type
		}
	}

	return typMap
}

func getGivenQueryArgValsAsMap(givenQuery *ast.Field) map[string]*ast.Value {
	argValMap := make(map[string]*ast.Value)

	for _, arg := range givenQuery.Arguments {
		argValMap[arg.Name] = arg.Value
	}
	return argValMap
}

// getRemoteQueryArgMetadata returns the argument metadata for given remote query
func getRemoteQueryArgMetadata(remoteQuery *gqlField) *remoteArgMetadata {
	md := &remoteArgMetadata{
		typMap:       make(map[string]*gqlType),
		requiredArgs: make([]string, 0),
	}

	for _, arg := range remoteQuery.Args {
		md.typMap[arg.Name] = arg.Type
		if arg.Type.Kind == nonNull {
			md.requiredArgs = append(md.requiredArgs, arg.Name)
		}
	}
	return md
}

type introspectedSchema struct {
	Data data `json:"data"`
}
type data struct {
	Schema introspectionSchema `json:"__schema"`
}
type introspectionSchema struct {
	QueryType        *introspectionQueryType `json:"queryType"`
	MutationType     *introspectionQueryType `json:"mutationType"`
	SubscriptionType *introspectionQueryType `json:"subscriptionType"`
	Types            []*types                `json:"types"`
	Directives       []*directive            `json:"directives"`
}
type introspectionQueryType struct {
	Name string `json:"name"`
}
type types struct {
	Kind          string        `json:"kind"`
	Name          string        `json:"name"`
	Fields        []*gqlField   `json:"fields"`
	InputFields   []*gqlField   `json:"inputFields"`
	Interfaces    []interface{} `json:"interfaces"`
	EnumValues    interface{}   `json:"enumValues"`
	PossibleTypes interface{}   `json:"possibleTypes"`
}
type directive struct {
	Name      string   `json:"name"`
	Locations []string `json:"locations"`
	Args      []*arg   `json:"args"`
}
type gqlField struct {
	Name              string      `json:"name"`
	Args              []*arg      `json:"args"`
	Type              *gqlType    `json:"type"`
	IsDeprecated      bool        `json:"isDeprecated"`
	DeprecationReason interface{} `json:"deprecationReason"`
}
type arg struct {
	Name         string      `json:"name"`
	Type         *gqlType    `json:"type"`
	DefaultValue interface{} `json:"defaultValue"`
}
type gqlType struct {
	Kind   string   `json:"kind"`
	Name   string   `json:"name"`
	OfType *gqlType `json:"ofType"`
}

func (t *gqlType) String() string {
	if t == nil {
		return ""
	}
	// refer http://spec.graphql.org/June2018/#sec-Type-Kinds
	// it confirms, if type kind is LIST or NON_NULL all other fields except ofType will be
	// null, so there won't be any name at that level. For other kinds, there will always be a name.
	switch t.Kind {
	case list:
		return fmt.Sprintf("[%s]", t.OfType.String())
	case nonNull:
		return fmt.Sprintf("%s!", t.OfType.String())
	// TODO: we will consider ID as String for the purpose of type matching
	//case "SCALAR":
	//	if t.Name == "ID" {
	//		return "String"
	//	}
	//	return t.Name
	default:
		return t.Name
	}
}

func (t *gqlType) NamedType() string {
	if t.Name != "" {
		return t.Name
	}
	return t.OfType.NamedType()
}
