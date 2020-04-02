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

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"github.com/vektah/gqlparser/v2/parser"
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

	introspectedArgs, notNullArgs := collectArgsFromIntrospection(introspectedRemoteQuery)

	for remoteArg, remoteArgVal := range remoteArguments {

		argType, ok := introspectedArgs[remoteArg]
		if !ok {
			return gqlerror.ErrorPosf(
				endpoint.directive.Position, "Type %s; Field %s;%s arg not present in the remote"+
					" query %s",
				endpoint.rootQuery.Name,
				endpoint.field.Name,
				remoteArg,
				remoteQuery.Name)
		}

		if _, ok = graphqlScalarType[argType]; !ok {
			fmt.Println(argType)
			return gqlerror.ErrorPosf(
				endpoint.directive.Position, "Type %s; Field %s; %s is not scalar. only scalar"+
					" argument is supported in the remote graphql call.",
				endpoint.rootQuery.Name,
				endpoint.field.Name,
				remoteArg)
		}
		argValToType[remoteArgVal] = argType
	}

	// We are only checking whether the required variable is exist in the
	// local remote call.
	for requiredArg := range notNullArgs {
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

	// Validate given argument type is matching with the remote query argument.
	for variable, typeName := range argValToType {
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

		if localRemoteCallArg.Type.Name() != typeName {
			return gqlerror.ErrorPosf(
				endpoint.directive.Position, "Type %s; Field %s; expected type for variable  "+
					"%s is %s. But got %s",
				endpoint.rootQuery.Name,
				endpoint.field.Name,
				variable,
				typeName,
				localRemoteCallArg.Type)
		}
	}
	return nil
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

// collectArgsFromIntrospection will collect all the arguments with it's type and required argument
func collectArgsFromIntrospection(query GqlField) (map[string]string, map[string]int) {
	notNullArgs := make(map[string]int)
	arguments := make(map[string]string)
	for _, introspectedArg := range query.Args {

		// Collect all the required variable to validate against provided variable.
		if introspectedArg.Type.Kind == "NOT_NULL" {
			notNullArgs[introspectedArg.Name] = 0
		}

		arguments[introspectedArg.Name] = introspectedArg.Type.OfType.Name
	}
	return arguments, notNullArgs
}

type IntrospectedSchema struct {
	Data Data `json:"data"`
}
type IntrospectionQueryType struct {
	Name string `json:"name"`
}
type OfType struct {
	Kind   string      `json:"kind"`
	Name   string      `json:"name"`
	OfType interface{} `json:"ofType"`
}
type GqlType struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	OfType OfType `json:"ofType"`
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
