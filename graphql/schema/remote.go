/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
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
	"time"

	"github.com/pkg/errors"
	"github.com/vektah/gqlparser/v2/ast"
)

// introspectRemoteSchema introspectes remote schema
func introspectRemoteSchema(url string) (*introspectedSchema, error) {
	param := &Request{
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

	return result, json.Unmarshal(body, result)
}

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
	// isBatch tells whether it is single/batch operation for resolving custom fields
	isBatch bool
	// url is the url of remote graphql endpoint
	url string
}

type argMatchingMetadata struct {
	givenArgVals  map[string]*ast.Value
	givenVarTypes map[string]*ast.Type
	remoteArgMd   *remoteArgMetadata
	remoteTypes   map[string]*types
	givenQryName  *string
	operationType *string
}

type remoteArgMetadata struct {
	typMap       map[string]*gqlType
	requiredArgs []string
}

// validates the graphql given in @custom->http->graphql by introspecting remote schema.
// It assumes that the graphql syntax is correct, only remote validation is needed.
func validateRemoteGraphql(metadata *remoteGraphqlMetadata) error {
	remoteIntrospection, err := introspectRemoteSchema(metadata.url)
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

	// TODO: still need to do deep type checking for return types
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

	givenQryArgVals := getGivenQueryArgValsAsMap(givenQuery)
	givenQryVarTypes := getVarTypesAsMap(metadata.parentField, metadata.parentType)
	remoteQryArgMetadata := getRemoteQueryArgMetadata(introspectedRemoteQuery)

	// verify remote query arg format for batch operations
	if metadata.isBatch {
		// TODO: make sure if it is [{}] or [{}!] or [{}]! or [{}!]!
		if len(remoteQryArgMetadata.typMap) != 1 {
			return errors.Errorf("remote %s `%s` accepts %d arguments, It must have only one "+
				"argument of the form `[{param1: $var1, param2: $var2, ...}]` for batch operation.",
				operationType, givenQuery.Name, len(remoteQryArgMetadata.typMap))
		}
		for argName, inputTyp := range remoteQryArgMetadata.typMap {
			if inputTyp.Kind != "LIST" || inputTyp.OfType == nil || inputTyp.OfType.
				Kind != "INPUT_OBJECT" {
				return errors.Errorf("argument `%s` for given %s `%s` must be of the form `[{param1"+
					": $var1, param2: $var2, ...}]` for batch operations in remote %s.", argName,
					operationType, givenQuery.Name, operationType)
			}
			inputTypName := inputTyp.OfType.Name
			typ, ok := remoteTypes[inputTypName]
			if !ok {
				return missingRemoteTypeError(inputTypName)
			}
			if typ.Kind != "INPUT_OBJECT" {
				return errors.Errorf("type %s in remote schema is not an INPUT_OBJECT.", inputTypName)
			}
		}
	}

	// check whether args of given query/mutation match the args of remote query/mutation
	if err = matchArgSignature(&argMatchingMetadata{
		givenArgVals:  givenQryArgVals,
		givenVarTypes: givenQryVarTypes,
		remoteArgMd:   remoteQryArgMetadata,
		remoteTypes:   remoteTypes,
		givenQryName:  &givenQuery.Name,
		operationType: &operationType,
	}); err != nil {
		return err
	}

	return nil
}

func missingRemoteTypeError(typName string) error {
	return errors.Errorf("remote schema doesn't have any type named %s.", typName)
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
		case ast.ObjectValue:
			if !(remoteArgTyp.Kind == "INPUT_OBJECT" || (remoteArgTyp.
				Kind == "NON_NULL" && remoteArgTyp.OfType != nil && remoteArgTyp.OfType.
				Kind == "INPUT_OBJECT")) {
				return errors.Errorf("object value supplied for argument `%s` in %s `%s`, "+
					"but remote argument doesn't accept INPUT_OBJECT.", givenArgName,
					*md.operationType, *md.givenQryName)
			}
			remoteObjTypname := remoteArgTyp.Name
			if remoteArgTyp.Kind != "INPUT_OBJECT" {
				remoteObjTypname = remoteArgTyp.OfType.Name
			}
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
			}); err != nil {
				return err
			}
		case ast.ListValue:
			if !((remoteArgTyp.Kind == "LIST" && remoteArgTyp.OfType != nil) || (remoteArgTyp.
				Kind == "NON_NULL" && remoteArgTyp.OfType != nil && remoteArgTyp.OfType.
				Kind == "LIST" && remoteArgTyp.OfType.OfType != nil)) {
				return errors.Errorf("LIST value supplied for argument `%s` in %s `%s`, "+
					"but remote argument doesn't accept LIST.", givenArgName, *md.operationType,
					*md.givenQryName)
			}
			remoteListElemTypname := remoteArgTyp.OfType.Name
			if remoteArgTyp.Kind != "LIST" {
				remoteListElemTypname = remoteArgTyp.OfType.OfType.Name
			}
			remoteObjTyp, ok := md.remoteTypes[remoteListElemTypname]
			if !ok {
				return missingRemoteTypeError(remoteListElemTypname)
			}
			if remoteObjTyp.Kind != "INPUT_OBJECT" {
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

	for _, field := range remoteTyp.Fields {
		md.typMap[field.Name] = field.Type
		if field.Type.Kind == "NON_NULL" {
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
		if arg.Type.Kind == "NON_NULL" {
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
	case "LIST":
		return fmt.Sprintf("[%s]", t.OfType.String())
	case "NON_NULL":
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
