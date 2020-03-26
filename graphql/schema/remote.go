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
 
 var graphqlScalarType = map[string]int{
	 "Int":     0,
	 "Float":   0,
	 "String":  0,
	 "Boolean": 0,
	 "ID":      0,
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
			 "Type %s; Field %s; only one query is possible inside query argument.",
			 endpoint.rootQuery.Name,
			 endpoint.field.Name)
	 }
 
	 // Check whether the given argument and query is present in the remote schema by introspecting.
	 remoteIntrospection, err := introspectRemoteSchema(endpoint.url)
	 if err != nil {
		 return gqlerror.ErrorPosf(
			 endpoint.directive.Position, "Type %s; Field %s; %s", endpoint.rootQuery.Name,
			 endpoint.field.Name,
			 err.Error())
	 }
 
	 // Validate given remote query is present in the remote schema or not.
	 remoteQuery := parsedQuery.Operations[0].SelectionSet[0].(*ast.Field)
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
	 NotNullArgs := make(map[string]int)
 
	 for remoteArg, remoteArgVal := range remoteArguments {
		 argFound := false
		 for _, introspectedArg := range introspectedRemoteQuery.Args {
 
			 // Collect all the required variable to validate against provided variable.
			 if introspectedArg.Type.Kind == "NOT_NULL" {
				 NotNullArgs[introspectedArg.Name] = 0
			 }
 
			 if remoteArg == introspectedArg.Name {
				 if _, ok := graphqlScalarType[introspectedArg.Type.Name]; !ok {
					 return gqlerror.ErrorPosf(
						 endpoint.directive.Position, `Type %s; Field %s; only scalar argument is
						 supported in the remote graphql call`,
						 endpoint.rootQuery.Name,
						 endpoint.field.Name)
				 }
				 argFound = true
				 // ASK: do we only support variables or hardcode values?
				 // Now assuming we support only variables.
				 argValToType[remoteArgVal] = introspectedArg.Type.Name
			 }
		 }
		 if !argFound {
			 return gqlerror.ErrorPosf(
				 endpoint.directive.Position, `Type %s; Field %s;%s arg not present in the remote 
				 query %s`,
				 endpoint.rootQuery.Name,
				 endpoint.field.Name,
				 remoteArg,
				 remoteQuery.Name)
		 }
	 }
 
	 for requiredArg := range NotNullArgs {
		 if _, ok := remoteArguments[requiredArg]; !ok {
			 return gqlerror.ErrorPosf(
				 endpoint.directive.Position, `Type %s; Field %s;%s is a required argument in the
				 remote query %s`,
				 endpoint.rootQuery.Name,
				 endpoint.field.Name,
				 requiredArg,
				 remoteQuery.Name)
		 }
	 }
 
	 // We are only checking whether the required variable is exist in the
	 // local remote call.
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
				 endpoint.directive.Position, `Type %s; Field %s; expected type for variable  %s 
				 is %s. But got %s`,
				 endpoint.rootQuery.Name,
				 endpoint.field.Name,
				 variable,
				 typeName,
				 localRemoteCallArg.Name)
		 }
	 }
	 return nil
 }
 
 func collectArgumentsFromQuery(query *ast.Field) map[string]string {
	 arguments := make(map[string]string)
	 for _, arg := range query.Arguments {
		 val := arg.Value.String()
		 arguments[arg.Name] = val
	 }
	 return arguments
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
 