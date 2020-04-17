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
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/dgraph-io/dgraph/x"
	"github.com/mitchellh/mapstructure"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"github.com/vektah/gqlparser/v2/parser"
)

// returnType is const key to represent graphql return. This key can't be used by user because,
// TypeKey uses restricted delimiter to form the key.
var returnType = string(x.TypeKey("graphql-return"))

// introspectRemoteSchema introspectes remote schema
func introspectRemoteSchema(url string) (*IntrospectedSchema, error) {
	param := &Request{
		Query: introspectionQuery,
	}

	body, err := json.Marshal(param)

	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
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

type remoteGraphqlEndpoint struct {
	graphqlArg *ast.Argument
	rootQuery  *ast.Definition
	schema     *ast.Schema
	field      *ast.FieldDefinition
	directive  *ast.Directive
	url        string
}

func validateRemoteGraphqlCall(endpoint *remoteGraphqlEndpoint) *gqlerror.Error {
	query := endpoint.graphqlArg.Value.Children.ForName("query")
	if query == nil {
		return gqlerror.ErrorPosf(
			endpoint.directive.Position,
			"Type %s; Field %s; query field inside @custom directive is mandatory.",
			endpoint.rootQuery.Name,
			endpoint.field.Name)
	}
	parsedQuery, gqlErr := parser.ParseQuery(&ast.Source{Input: fmt.Sprintf(`query {%s}`,
		query.Raw)})
	if gqlErr != nil {
		return gqlErr
	}

	if len(parsedQuery.Operations[0].SelectionSet) > 1 {
		return gqlerror.ErrorPosf(
			endpoint.directive.Position,
			"Type %s; Field %s; only one query is possible inside query argument. For eg:"+
				" valid input: @custom(..., graphql:{ query: \"getUser(id: $id)\"})"+"invalid input:"+
				"@custom(..., graphql:{query: \"getUser(id: $id) getAuthor(id: $id)\"})",
			endpoint.rootQuery.Name,
			endpoint.field.Name)
	}

	// Validate given remote query is present in the remote schema or not.
	remoteQuery := parsedQuery.Operations[0].SelectionSet[0].(*ast.Field)

	// remoteQuery should not contain any selection set.
	// for eg: bla(..){
	// 	..
	// }
	if len(remoteQuery.SelectionSet) != 0 {
		return gqlerror.ErrorPosf(
			endpoint.directive.Position, "Type %s; Field %s;Remote query %s should not contain"+
				" any selection statement. eg: Remote query should be like this %s(...) and not"+
				" like %s(..){...}", endpoint.rootQuery.Name,
			endpoint.field.Name,
			remoteQuery.Name,
			remoteQuery.Name,
			remoteQuery.Name)
	}

	// Check whether the given argument and query is present in the remote schema by introspecting.
	remoteIntrospection, err := introspectRemoteSchema(endpoint.url)
	if err != nil {
		return gqlerror.ErrorPosf(
			endpoint.directive.Position, "Type %s; Field %s; unable to introspect remote schema"+
				" for the url %s", endpoint.rootQuery.Name,
			endpoint.field.Name,
			endpoint.url)
	}

	var introspectedRemoteQuery GqlField
	queryExist := false
	for _, types := range remoteIntrospection.Data.Schema.Types {
		if types.Name != "Query" {
			continue
		}
		for _, queryType := range types.Fields {
			if queryType.Name == remoteQuery.Name {
				queryExist = true
				introspectedRemoteQuery = queryType
			}
		}
	}

	if !queryExist {
		return gqlerror.ErrorPosf(
			endpoint.directive.Position, "Type %s; Field %s; %s is not present in remote schema",
			endpoint.rootQuery.Name,
			endpoint.field.Name,
			remoteQuery.Name)
	}

	// check whether given arguments are present in the remote query.
	remoteArguments := collectArgumentsFromQuery(remoteQuery)
	argValToType := make(map[string]string)
	argValToGqlType := make(map[string]*GqlType)

	remoteQueryArguments := collectArgsFromIntrospection(introspectedRemoteQuery)

	for remoteArg, remoteArgVal := range remoteArguments {

		argType, ok := remoteQueryArguments.arguments[remoteArg]
		if !ok {
			return gqlerror.ErrorPosf(
				endpoint.directive.Position, "Type %s; Field %s; %s arg not present in the remote"+
					" query %s",
				endpoint.rootQuery.Name,
				endpoint.field.Name,
				remoteArg,
				remoteQuery.Name)
		}

		argValToType[remoteArgVal] = argType
		argValToGqlType[remoteArgVal] = remoteQueryArguments.argumentsToGqlType[remoteArg]
	}

	// We are only checking whether the required variable is exist in the
	// local remote call.
	for requiredArg := range remoteQueryArguments.notNullArgs {
		if _, ok := remoteArguments[requiredArg]; !ok {
			return gqlerror.ErrorPosf(
				endpoint.directive.Position, "Type %s; Field %s;%s is a required argument in the "+
					"remote query %s. But, the %s is not present in the custom logic call for %s. "+
					"Please provide the required arg in the remote query %s",
				endpoint.rootQuery.Name,
				endpoint.field.Name,
				requiredArg,
				remoteQuery.Name,
				requiredArg,
				remoteQuery.Name,
				remoteQuery.Name)
		}
	}

	// Add return type as well to expand. because, we need to type check the return type as well.
	argValToType[returnType] = endpoint.field.Type.String()

	// Now we have to expand the remote types and check with the local types.
	expandedTypes := expandArgs(argValToType, remoteIntrospection)

	for typeName, fields := range expandedTypes {
		localType, ok := endpoint.schema.Types[typeName]
		if !ok {
			return gqlerror.ErrorPosf(
				endpoint.directive.Position, "Unable to find remote type %s in the local schema",
				typeName,
			)
		}
		for _, field := range fields {
			localField := localType.Fields.ForName(field.Name)
			if localField == nil {
				return gqlerror.ErrorPosf(
					endpoint.directive.Position,
					"%s field for the remote type %s is not present in the local type %s",
					field.Name, localType.Name, localType.Name,
				)
			}
			if localField.Type.NamedType != recursivelyFindName(&field.Type) {
				return gqlerror.ErrorPosf(
					endpoint.field.Position,
					"expected type for the field %s is %s but got %s",
					field.Name,
					recursivelyFindName(&field.Type),
					localField.Type.NamedType,
				)
			}
		}
	}

	// Type check for remote argument type with local query argument.
	for variable := range argValToType {

		if variable == returnType {
			introspectedReturnType := buildTypeStringFromGqlType(&introspectedRemoteQuery.Type)
			if endpoint.field.Type.String() != introspectedReturnType {
				return gqlerror.ErrorPosf(
					endpoint.directive.Position, "Type %s; Field %s; expected return type  "+
						"is %s. But got %s",
					endpoint.rootQuery.Name,
					endpoint.field.Name,
					introspectedReturnType,
					endpoint.field.Type.String())
			}
			continue
		}
		localRemoteCallArg := endpoint.field.Arguments.ForName(variable[1:])
		if localRemoteCallArg == nil {
			return gqlerror.ErrorPosf(
				endpoint.directive.Position, `Type %s; Field %s; unable to find the variable %s in 
				  %s`,
				endpoint.rootQuery.Name,
				endpoint.field.Name,
				variable,
				endpoint.field.Name)
		}

		if localRemoteCallArg.Type.String() != buildTypeStringFromGqlType(argValToGqlType[variable]) {
			return gqlerror.ErrorPosf(
				endpoint.directive.Position, "Type %s; Field %s; expected type for variable  "+
					"%s is %s. But got %s",
				endpoint.rootQuery.Name,
				endpoint.field.Name,
				variable,
				buildTypeStringFromGqlType(argValToGqlType[variable]),
				localRemoteCallArg.Type)
		}
	}
	return nil
}

type expandArgParams struct {
	expandedTypes      map[string]struct{}
	introspectedSchema *IntrospectedSchema
	typesToFields      map[string][]GqlField
}

func expandArgRecursively(arg string, param *expandArgParams) {
	_, alreadyExpanded := param.expandedTypes[arg]
	if alreadyExpanded {
		return
	}
	// We're marking this to avoid recursive expansion.
	param.expandedTypes[arg] = struct{}{}
	for _, inputType := range param.introspectedSchema.Data.Schema.Types {
		if inputType.Name == arg {
			param.typesToFields[inputType.Name] = inputType.Fields
			// Expand the non scalar types.
			for _, field := range inputType.Fields {
				_, ok := graphqlScalarType[recursivelyFindName(&field.Type)]
				if !ok {
					// expand this field.
					expandArgRecursively(recursivelyFindName(&field.Type), param)
				}
			}
		}
	}
}

func expandArgs(argToVal map[string]string,
	introspectedSchema *IntrospectedSchema) map[string][]GqlField {

	param := &expandArgParams{
		expandedTypes:      make(map[string]struct{}, 0),
		typesToFields:      make(map[string][]GqlField),
		introspectedSchema: introspectedSchema,
	}
	// Expand the types that are required to do a query.
	for _, typeTobeExpanded := range argToVal {
		_, ok := graphqlScalarType[typeTobeExpanded]
		if ok {
			continue
		}
		expandArgRecursively(typeTobeExpanded, param)
	}
	return param.typesToFields
}

// collectArgumentsFromQuery will collect all the arguments and values from the query.
func collectArgumentsFromQuery(query *ast.Field) map[string]string {
	arguments := make(map[string]string)
	for _, arg := range query.Arguments {
		val := arg.Value.String()
		arguments[arg.Name] = val
	}
	return arguments
}

type remoteArgParams struct {
	notNullArgs        map[string]int
	arguments          map[string]string
	argumentsToGqlType map[string]*GqlType
}

// collectArgsFromIntrospection will collect all the arguments with it's type and required argument
func collectArgsFromIntrospection(query GqlField) *remoteArgParams {
	notNullArgs := make(map[string]int)
	arguments := make(map[string]string)
	argumentsToGqlType := make(map[string]*GqlType)
	for _, introspectedArg := range query.Args {

		// Collect all the required variable to validate against provided variable.
		if introspectedArg.Type.Kind == "NON_NULL" {
			notNullArgs[introspectedArg.Name] = 0
		}

		arguments[introspectedArg.Name] = recursivelyFindName(&introspectedArg.Type)
		argumentsToGqlType[introspectedArg.Name] = &introspectedArg.Type
	}
	return &remoteArgParams{
		notNullArgs:        notNullArgs,
		arguments:          arguments,
		argumentsToGqlType: argumentsToGqlType,
	}
}

func buildTypeStringFromGqlType(in *GqlType) string {
	if in.Kind == "LIST" {
		tmp := &GqlType{}
		mappedField, ok := in.OfType.(map[string]interface{})
		if !ok {
			// List of type can't be nil. Need a proper error message.
			x.Panic(errors.New("Of type should have map value"))
		}
		mapstructure.Decode(mappedField, tmp)
		return "[" + buildTypeStringFromGqlType(tmp) + "]"
	} else if in.Kind == "NON_NULL" {
		tmp := &GqlType{}
		mappedField, ok := in.OfType.(map[string]interface{})
		if !ok {
			// List of type can't be nil. Need a proper error message.
			x.Panic(errors.New("Of type should have map value"))
		}
		mapstructure.Decode(mappedField, tmp)
		return buildTypeStringFromGqlType(tmp) + "!"
	}
	return in.Name
}

func recursivelyFindName(in *GqlType) string {
	if in.Name != "" {
		return in.Name
	}
	if in.OfType != nil {
		tmp := &GqlType{}
		mappedField, ok := in.OfType.(map[string]interface{})
		if !ok {
			x.Panic(errors.New("Of type should have map value"))
		}
		mapstructure.Decode(mappedField, tmp)
		return recursivelyFindName(tmp)
	}
	return ""
}

// findFieldFromType will go deep to find the type
func recursivelyBuildTypeString(interface{}) string {
	return ""
}

type IntrospectedSchema struct {
	Data Data `json:"data"`
}
type IntrospectionQueryType struct {
	Name string `json:"name"`
}
type GqlType struct {
	Kind   string      `json:"kind"`
	Name   string      `json:"name"`
	OfType interface{} `json:"ofType"`
}

type GqlField struct {
	Name              string      `json:"name"`
	Args              []Args      `json:"args"`
	Type              GqlType     `json:"type"`
	IsDeprecated      bool        `json:"isDeprecated"`
	DeprecationReason interface{} `json:"deprecationReason"`
}
type Types struct {
	Kind          string        `json:"kind"`
	Name          string        `json:"name"`
	Fields        []GqlField    `json:"fields"`
	InputFields   []GqlField    `json:"inputFields"`
	Interfaces    []interface{} `json:"interfaces"`
	EnumValues    interface{}   `json:"enumValues"`
	PossibleTypes interface{}   `json:"possibleTypes"`
}
type Args struct {
	Name         string      `json:"name"`
	Type         GqlType     `json:"type"`
	DefaultValue interface{} `json:"defaultValue"`
}
type Directives struct {
	Name      string   `json:"name"`
	Locations []string `json:"locations"`
	Args      []Args   `json:"args"`
}
type IntrospectionSchema struct {
	QueryType        IntrospectionQueryType `json:"queryType"`
	MutationType     interface{}            `json:"mutationType"`
	SubscriptionType interface{}            `json:"subscriptionType"`
	Types            []Types                `json:"types"`
	Directives       []Directives           `json:"directives"`
}
type Data struct {
	Schema IntrospectionSchema `json:"__schema"`
}
