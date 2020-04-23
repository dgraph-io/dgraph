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
func introspectRemoteSchema(url string) (*IntrospectedSchema, error) {
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
	result := &IntrospectedSchema{}

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

// validates the graphql given in @custom->http->graphql by introspecting remote schema.
// It assumes that the graphql syntax is correct, only remote validation is needed.
func validateRemoteGraphql(metadata *remoteGraphqlMetadata) error {
	remoteIntrospection, err := introspectRemoteSchema(metadata.url)
	if err != nil {
		return err
	}

	var remoteQueryTypename string
	operationType := metadata.graphqlOpDef.Operation
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
		return errors.Errorf("found %s operation, it can only have query/mutation.", operationType)
	}

	remoteTypes := make(map[string]*Types)
	for _, typ := range remoteIntrospection.Data.Schema.Types {
		remoteTypes[typ.Name] = typ
	}

	remoteQryType, ok := remoteTypes[remoteQueryTypename]
	if !ok {
		return errors.Errorf("remote schema doesn't have any type named %s.", remoteQueryTypename)
	}

	// check whether given query/mutation is present in remote schema
	var introspectedRemoteQuery *GqlField
	givenQuery := metadata.graphqlOpDef.SelectionSet[0].(*ast.Field)
	for _, remoteQuery := range remoteQryType.Fields {
		if remoteQuery.Name == givenQuery.Name {
			introspectedRemoteQuery = remoteQuery
			break
		}
	}
	if introspectedRemoteQuery == nil {
		return errors.Errorf("given %s: %s is not present in remote schema.",
			operationType, givenQuery.Name)
	}

	// check whether the return type of remote query is same as the required return type
	expectedReturnType := metadata.parentField.Type.String()
	gotReturnType := introspectedRemoteQuery.Type.String()
	if metadata.isBatch {
		expectedReturnType = fmt.Sprintf("[%s]", expectedReturnType)
	}
	if expectedReturnType != gotReturnType {
		return errors.Errorf("given %s: %s: return type mismatch; expected: %s, got: %s.",
			operationType, givenQuery.Name, expectedReturnType, gotReturnType)
	}

	givenQryArgDefs, givenQryArgVals := getGivenQueryArgsAsMap(givenQuery, metadata.isBatch,
		metadata.parentField, metadata.parentType)
	remoteQryArgDefs, remoteQryRequiredArgs := getRemoteQueryArgsAsMap(introspectedRemoteQuery)

	// correctly set remoteQryArgDefs and remoteQryRequiredArgs for batch operations
	if metadata.isBatch {
		input, ok := remoteQryArgDefs["input"]
		if !ok || input.Type.Kind != "LIST" || input.Type.OfType == nil || input.Type.OfType.
			Kind != "OBJECT" {
			return errors.Errorf("given %s: %s: arg `input` is not of the form `[{param1: $var1, "+
				"param2: $var2, ...}]` in remote %s.",
				operationType, givenQuery.Name, operationType)
		}
		inputTypName := input.Type.OfType.Name
		typ, ok := remoteTypes[inputTypName]
		if !ok {
			return errors.Errorf("remote schema doesn't have any type named %s.", inputTypName)
		}
		remoteQryArgDefs = make(map[string]*Args)
		remoteQryRequiredArgs = make([]string, 0)
		for _, field := range typ.Fields {
			remoteQryArgDefs[field.Name] = &Args{Type: field.Type}
			if field.Type.Kind == "NON_NULL" {
				remoteQryRequiredArgs = append(remoteQryRequiredArgs, field.Name)
			}
		}
	}

	// check whether args of given query/mutation match the args of remote query/mutation
	for givenArgName, givenArgDef := range givenQryArgDefs {
		remoteArgDef, ok := remoteQryArgDefs[givenArgName]
		if !ok {
			return errors.Errorf("given %s: %s: arg %s not present in remote %s.", operationType,
				givenQuery.Name, givenArgName, operationType)
		}
		if givenArgDef == nil {
			return errors.Errorf("given %s: %s: variable %s is missing in given context.",
				operationType, givenQuery.Name, givenQryArgVals[givenArgName])
		}
		expectedArgType := givenArgDef.Type.String()
		gotArgType := remoteArgDef.Type.String()
		if expectedArgType != gotArgType {
			return errors.Errorf("given %s: %s: type mismatch for variable %s; expected: %s, "+
				"got: %s.", operationType, givenQuery.Name, givenQryArgVals[givenArgName],
				expectedArgType, gotArgType)
		}
	}

	// check all non-null args required by remote query/mutation are present in given query/mutation
	for _, remoteArgName := range remoteQryRequiredArgs {
		_, ok := givenQryArgVals[remoteArgName]
		if !ok {
			return errors.Errorf("given %s: %s: required arg %s is missing.", operationType,
				givenQuery.Name, remoteArgName)
		}
	}

	return nil
}

// getGivenQueryArgsAsMap returns following maps:
// 1. arg name -> *ast.ArgumentDefinition (Currently, we only need to use the Type field from this)
// 2. arg name -> argument value (i.e., variable like $id)
// It takes care of the special format for batch mode operations
func getGivenQueryArgsAsMap(givenQuery *ast.Field, isBatch bool, parentField *ast.FieldDefinition,
	parentType *ast.Definition) (map[string]*ast.ArgumentDefinition, map[string]string) {
	argDefMap := make(map[string]*ast.ArgumentDefinition)
	argValMap := make(map[string]string)
	givenQueryArgs := givenQuery.Arguments

	var inputArgDefsMap map[string]*ast.ArgumentDefinition
	if isQueryOrMutationType(parentType) {
		// this is the case of @custom on some Query or Mutation
		inputArgDefsMap = getFieldArgDefsAsMap(parentField)
	} else {
		// this is the case of @custom on fields inside some user defined type
		inputArgDefsMap = getTypeFieldsAsArgDefsMap(parentType)
		if isBatch {
			givenQueryArgs = make([]*ast.Argument, 0)
			for _, arg := range givenQuery.Arguments[0].Value.Children[0].Value.Children {
				givenQueryArgs = append(givenQueryArgs, &ast.Argument{
					Name:     arg.Name,
					Value:    arg.Value,
					Position: arg.Position,
				})
			}
		}
	}

	for _, arg := range givenQueryArgs {
		varName := arg.Value.String()
		argDefMap[arg.Name] = inputArgDefsMap[varName[1:]]
		argValMap[arg.Name] = varName
	}
	return argDefMap, argValMap
}

func getFieldArgDefsAsMap(fieldDef *ast.FieldDefinition) map[string]*ast.ArgumentDefinition {
	argMap := make(map[string]*ast.ArgumentDefinition)
	for _, v := range fieldDef.Arguments {
		argMap[v.Name] = v
	}
	return argMap
}

func getTypeFieldsAsArgDefsMap(typ *ast.Definition) map[string]*ast.ArgumentDefinition {
	argMap := make(map[string]*ast.ArgumentDefinition)
	for _, v := range typ.Fields {
		argMap[v.Name] = &ast.ArgumentDefinition{Type: v.Type}
	}
	return argMap
}

// getRemoteQueryArgsAsMap returns following things:
// 1. map of arg name -> Argument Definition in Gql introspection response format
// 2. list of arg name for NON_NULL args
func getRemoteQueryArgsAsMap(remoteQuery *GqlField) (map[string]*Args, []string) {
	argDefMap := make(map[string]*Args)
	requiredArgs := make([]string, 0)

	for _, arg := range remoteQuery.Args {
		argDefMap[arg.Name] = arg
		if arg.Type.Kind == "NON_NULL" {
			requiredArgs = append(requiredArgs, arg.Name)
		}
	}
	return argDefMap, requiredArgs
}

type IntrospectedSchema struct {
	Data Data `json:"data"`
}
type IntrospectionQueryType struct {
	Name string `json:"name"`
}
type GqlType struct {
	Kind   string   `json:"kind"`
	Name   string   `json:"name"`
	OfType *GqlType `json:"ofType"`
}
type GqlField struct {
	Name              string      `json:"name"`
	Args              []*Args     `json:"args"`
	Type              *GqlType    `json:"type"`
	IsDeprecated      bool        `json:"isDeprecated"`
	DeprecationReason interface{} `json:"deprecationReason"`
}
type Types struct {
	Kind          string        `json:"kind"`
	Name          string        `json:"name"`
	Fields        []*GqlField   `json:"fields"`
	InputFields   []*GqlField   `json:"inputFields"`
	Interfaces    []interface{} `json:"interfaces"`
	EnumValues    interface{}   `json:"enumValues"`
	PossibleTypes interface{}   `json:"possibleTypes"`
}
type Args struct {
	Name         string      `json:"name"`
	Type         *GqlType    `json:"type"`
	DefaultValue interface{} `json:"defaultValue"`
}
type Directives struct {
	Name      string   `json:"name"`
	Locations []string `json:"locations"`
	Args      []*Args  `json:"args"`
}
type IntrospectionSchema struct {
	QueryType        *IntrospectionQueryType `json:"queryType"`
	MutationType     *IntrospectionQueryType `json:"mutationType"`
	SubscriptionType *IntrospectionQueryType `json:"subscriptionType"`
	Types            []*Types                `json:"types"`
	Directives       []*Directives           `json:"directives"`
}
type Data struct {
	Schema IntrospectionSchema `json:"__schema"`
}

func (t *GqlType) String() string {
	if t == nil {
		return ""
	}
	// refer http://spec.graphql.org/June2018/#sec-Type-Kinds
	// it confirms, if type kind is LIST or NON_NULL all other fields except ofType will be
	// null, so there won't be any name at that level.
	switch t.Kind {
	case "LIST":
		return fmt.Sprintf("[%s]", t.OfType.String())
	case "NON_NULL":
		return fmt.Sprintf("%s!", t.OfType.String())
	default:
		return t.Name
	}
}
