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

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os/exec"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v2"
)

type expectedRequest struct {
	method    string
	urlSuffix string
	body      string
	// Send headers as nil to ignore comparing headers.
	// Provide nil value for a key just to ensure that the key exists in request headers.
	// Provide both key and value to ensure that key exists with given value
	headers map[string][]string
}

type GraphqlRequest struct {
	Query         string          `json:"query"`
	OperationName string          `json:"operationName"`
	Variables     json.RawMessage `json:"variables"`
}
type graphqlResponseObject struct {
	Response  string
	Schema    string
	Name      string
	Request   string
	Variables string
}

var graphqlResponses map[string]graphqlResponseObject

func init() {
	b, err := ioutil.ReadFile("graphqlresponse.yaml")
	if err != nil {
		panic(err)
	}
	resps := []graphqlResponseObject{}

	err = yaml.Unmarshal(b, &resps)
	if err != nil {
		log.Fatal(err)
	}

	graphqlResponses = make(map[string]graphqlResponseObject)

	for _, resp := range resps {
		graphqlResponses[resp.Name] = resp
	}
}

func generateIntrospectionResult(schema string) string {
	cmd := exec.Command("node", "index.js", schema)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		log.Fatal(err)
	}
	b, err := ioutil.ReadAll(stdout)
	if err != nil {
		log.Fatal(err)
	}
	return string(b)
}

func commonGraphqlHandler(handlerName string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Fatal(err)
		}

		// return introspection json if it's introspection request.
		if strings.Contains(string(body), "__schema") {
			check2(fmt.Fprint(w,
				generateIntrospectionResult(graphqlResponses[handlerName].Schema)))
			return
		}
		// Parse the given graphql request.
		req := &GraphqlRequest{}
		err = json.Unmarshal(body, req)
		if err != nil {
			log.Fatal(err)
		}
		if req.Query == strings.TrimSpace(graphqlResponses[handlerName].Request) && string(req.Variables) == strings.TrimSpace(graphqlResponses[handlerName].Variables) {
			fmt.Fprintf(w, graphqlResponses[handlerName].Response)
			return
		}
	}
}

type expectedGraphqlRequest struct {
	urlSuffix string
	// Send body as empty string to make sure that only introspection queries are expected
	body string
}

func check2(v interface{}, err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func getError(key, val string) error {
	return fmt.Errorf(`{ "errors": [{"message": "%s: %s"}] }`, key, val)
}

func verifyRequest(r *http.Request, expectedRequest expectedRequest) error {
	if r.Method != expectedRequest.method {
		return getError("Invalid HTTP method", r.Method)
	}

	if !strings.HasSuffix(r.URL.String(), expectedRequest.urlSuffix) {
		return getError("Invalid URL", r.URL.String())
	}

	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return getError("Unable to read request body", err.Error())
	}
	if string(b) != expectedRequest.body {
		return getError("Unexpected value for request body", string(b))
	}

	if expectedRequest.headers != nil {
		actualHeaderLen := len(r.Header)
		expectedHeaderLen := len(expectedRequest.headers)
		if actualHeaderLen != expectedHeaderLen {
			return getError(fmt.Sprintf("Wanted %d headers in request, got", expectedHeaderLen),
				strconv.Itoa(actualHeaderLen))
		}

		for k, v := range expectedRequest.headers {
			rv, ok := r.Header[k]
			if !ok {
				return getError("Required header not found", k)
			}

			if v == nil {
				continue
			}

			sort.Strings(rv)
			sort.Strings(v)

			if !reflect.DeepEqual(rv, v) {
				return getError(fmt.Sprintf("Unexpected value for %s header", k), fmt.Sprint(rv))
			}
		}
	}

	return nil
}

// bool parameter in return signifies whether it is an introspection query or not:
//
// true -> introspection query
//
// false -> not an introspection query
func verifyGraphqlRequest(r *http.Request, expectedRequest expectedGraphqlRequest) (bool, error) {
	if r.Method != http.MethodPost {
		return false, getError("Invalid HTTP method", r.Method)
	}

	if !strings.HasSuffix(r.URL.String(), expectedRequest.urlSuffix) {
		return false, getError("Invalid URL", r.URL.String())
	}

	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return false, getError("Unable to read request body", err.Error())
	}
	actualBody := string(b)
	if strings.Contains(actualBody, "__schema") {
		return true, nil
	}
	if actualBody != expectedRequest.body {
		return false, getError("Unexpected value for request body", actualBody)
	}

	return false, nil
}

func getDefaultResponse() []byte {
	resTemplate := `[
			{
				"id": "0x3",
				"name": "Star Wars",
				"director": [
					{
						"id": "0x4",
						"name": "George Lucas"
					}
				]
			},
			{
				"id": "0x5",
				"name": "Star Trek",
				"director": [
					{
						"id": "0x6",
						"name": "J.J. Abrams"
					}
				]
			}
		]`

	return []byte(resTemplate)
}

func getFavMoviesHandler(w http.ResponseWriter, r *http.Request) {
	err := verifyRequest(r, expectedRequest{
		method:    http.MethodGet,
		urlSuffix: "/0x123?name=Author&num=10",
		body:      "",
		headers:   nil,
	})
	if err != nil {
		check2(w.Write([]byte(err.Error())))
		return
	}
	check2(w.Write(getDefaultResponse()))
}

func postFavMoviesHandler(w http.ResponseWriter, r *http.Request) {
	err := verifyRequest(r, expectedRequest{
		method:    http.MethodPost,
		urlSuffix: "/0x123?name=Author&num=10",
		body:      "",
		headers:   nil,
	})
	if err != nil {
		check2(w.Write([]byte(err.Error())))
		return
	}
	check2(w.Write(getDefaultResponse()))
}

func verifyHeadersHandler(w http.ResponseWriter, r *http.Request) {
	err := verifyRequest(r, expectedRequest{
		method:    http.MethodGet,
		urlSuffix: "/verifyHeaders",
		body:      "",
		headers: map[string][]string{
			"X-App-Token":     {"app-token"},
			"X-User-Id":       {"123"},
			"Accept-Encoding": nil,
			"User-Agent":      nil,
		},
	})
	if err != nil {
		check2(w.Write([]byte(err.Error())))
		return
	}
	check2(w.Write([]byte(`[{"id":"0x3","name":"Star Wars"}]`)))
}

func favMoviesCreateHandler(w http.ResponseWriter, r *http.Request) {
	err := verifyRequest(r, expectedRequest{
		method:    http.MethodPost,
		urlSuffix: "/favMoviesCreate",
		body:      `{"movies":[{"director":[{"name":"Dir1"}],"name":"Mov1"},{"name":"Mov2"}]}`,
		headers:   nil,
	})
	if err != nil {
		check2(w.Write([]byte(err.Error())))
		return
	}

	check2(w.Write([]byte(`[
        {
          "id": "0x1",
          "name": "Mov1",
          "director": [
            {
              "id": "0x2",
              "name": "Dir1"
            }
          ]
        },
        {
          "id": "0x3",
          "name": "Mov2"
        }
    ]`)))
}

func favMoviesUpdateHandler(w http.ResponseWriter, r *http.Request) {
	err := verifyRequest(r, expectedRequest{
		method:    http.MethodPatch,
		urlSuffix: "/favMoviesUpdate/0x1",
		body:      `{"director":[{"name":"Dir1"}],"name":"Mov1"}`,
		headers:   nil,
	})
	if err != nil {
		check2(w.Write([]byte(err.Error())))
		return
	}

	check2(w.Write([]byte(`
	{
        "id": "0x1",
        "name": "Mov1",
        "director": [
          {
            "id": "0x2",
            "name": "Dir1"
          }
        ]
    }`)))
}

func favMoviesDeleteHandler(w http.ResponseWriter, r *http.Request) {
	err := verifyRequest(r, expectedRequest{
		method:    http.MethodDelete,
		urlSuffix: "/favMoviesDelete/0x1",
		body:      "",
		headers: map[string][]string{
			"X-App-Token":     {"app-token"},
			"X-User-Id":       {"123"},
			"Accept-Encoding": nil,
			"User-Agent":      nil,
		},
	})
	if err != nil {
		check2(w.Write([]byte(err.Error())))
		return
	}

	check2(w.Write([]byte(`
	{
        "id": "0x1",
        "name": "Mov1"
    }`)))
}

func emptyQuerySchema(w http.ResponseWriter, r *http.Request) {
	if _, err := verifyGraphqlRequest(r, expectedGraphqlRequest{
		urlSuffix: "/noquery",
		body:      ``,
	}); err != nil {
		check2(w.Write([]byte(err.Error())))
		return
	}
	check2(fmt.Fprintf(w, `
	{
	"data": {
		"__schema": {
		  "queryType": {
			"name": "Query"
		  },
		  "mutationType": null,
		  "subscriptionType": null,
		  "types": [
			{
			  "kind": "OBJECT",
			  "name": "Query",
			  "fields": []
			}]
		  }
	   }
	}
	`))
}

func nullQueryAndMutationType(w http.ResponseWriter, r *http.Request) {
	if _, err := verifyGraphqlRequest(r, expectedGraphqlRequest{
		urlSuffix: "/nullQueryAndMutationType",
		body:      ``,
	}); err != nil {
		check2(w.Write([]byte(err.Error())))
		return
	}
	check2(fmt.Fprintf(w, `
	{
		"data": {
			"__schema": {
				"queryType": null,
				"mutationType": null,
				"subscriptionType": null
			}
		}
	}
	`))
}

func missingQueryAndMutationType(w http.ResponseWriter, r *http.Request) {
	if _, err := verifyGraphqlRequest(r, expectedGraphqlRequest{
		urlSuffix: "/missingQueryAndMutationType",
		body:      ``,
	}); err != nil {
		check2(w.Write([]byte(err.Error())))
		return
	}
	check2(fmt.Fprintf(w, `
	{
		"data": {
			"__schema": {
				"queryType": {
					"name": "Query"
				},
				"mutationType": {
					"name": "Mutation"
				},
				"subscriptionType": null
			}
		}
	}
	`))
}

func invalidInputForBatchedField(w http.ResponseWriter, r *http.Request) {
	if _, err := verifyGraphqlRequest(r, expectedGraphqlRequest{
		urlSuffix: "/invalidInputForBatchedField",
		body:      ``,
	}); err != nil {
		check2(w.Write([]byte(err.Error())))
		return
	}
	check2(fmt.Fprint(w,
		generateIntrospectionResult(graphqlResponses["invalidinputbatchedfield"].Schema)))
}

func missingTypeForBatchedFieldInput(w http.ResponseWriter, r *http.Request) {
	if _, err := verifyGraphqlRequest(r, expectedGraphqlRequest{
		urlSuffix: "/missingTypeForBatchedFieldInput",
		body:      ``,
	}); err != nil {
		check2(w.Write([]byte(err.Error())))
		return
	}
	check2(fmt.Fprintf(w, `
		{
		"data": {
			"__schema": {
			  "queryType": {
				"name": "Query"
			  },
			  "mutationType": null,
			  "subscriptionType": null,
			  "types": [
				{
				  "kind": "OBJECT",
				  "name": "Query",
				  "fields": [
					{
						"name": "getPosts",
						"args": [
						  {
							"name": "input",
							"type": {
							  "kind": "LIST",
							  "name": null,
							  "ofType": {
								"kind": "INPUT_OBJECT",
								"name": "PostFilterInput",
								"ofType": null
							  }
							},
							"defaultValue": null
						  }
						],
						"type": {
						  "kind": "LIST",
						  "name": null,
						  "ofType": {
						 	"kind": "NON_NULL",
						 	"name": null,
							"ofType": {
							  "kind": "OBJECT",
							  "name": "String",
							  "ofType": null
							}
						  }
						},
						"isDeprecated": false,
						"deprecationReason": null
					  }
				  ]
				}]
			  }
		   }
		}`))
}

func getPosts(w http.ResponseWriter, r *http.Request) {
	_, err := verifyGraphqlRequest(r, expectedGraphqlRequest{
		urlSuffix: "/getPosts",
		body:      ``,
	})
	if err != nil {
		check2(w.Write([]byte(err.Error())))
		return
	}

	check2(fmt.Fprint(w, generateIntrospectionResult(graphqlResponses["getPosts"].Schema)))
}

func getPostswithLike(w http.ResponseWriter, r *http.Request) {
	_, err := verifyGraphqlRequest(r, expectedGraphqlRequest{
		urlSuffix: "/getPostswithLike",
		body:      ``,
	})
	if err != nil {
		check2(w.Write([]byte(err.Error())))
		return
	}

	check2(fmt.Fprint(w, generateIntrospectionResult(graphqlResponses["getPostswithLike"].Schema)))
}

type input struct {
	ID string `json:"uid"`
}

func (i input) Name() string {
	return "uname-" + i.ID
}

func getInput(r *http.Request, v interface{}) error {
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Println("while reading body: ", err)
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		fmt.Println("while doing JSON unmarshal: ", err)
		return err
	}
	return nil
}

func userNamesHandler(w http.ResponseWriter, r *http.Request) {
	var inputBody []input
	err := getInput(r, &inputBody)
	if err != nil {
		fmt.Println("while reading input: ", err)
		return
	}

	// append uname to the id and return it.
	res := make([]interface{}, 0, len(inputBody))
	for i := 0; i < len(inputBody); i++ {
		res = append(res, "uname-"+inputBody[i].ID)
	}

	b, err := json.Marshal(res)
	if err != nil {
		fmt.Println("while marshaling result: ", err)
		return
	}
	check2(fmt.Fprint(w, string(b)))
}

type tinput struct {
	ID string `json:"tid"`
}

func (i tinput) Name() string {
	return "tname-" + i.ID
}

func teacherNamesHandler(w http.ResponseWriter, r *http.Request) {
	var inputBody []tinput
	err := getInput(r, &inputBody)
	if err != nil {
		fmt.Println("while reading input: ", err)
		return
	}

	// append tname to the id and return it.
	res := make([]interface{}, 0, len(inputBody))
	for i := 0; i < len(inputBody); i++ {
		res = append(res, "tname-"+inputBody[i].ID)
	}

	b, err := json.Marshal(res)
	if err != nil {
		fmt.Println("while marshaling result: ", err)
		return
	}
	check2(fmt.Fprint(w, string(b)))
}

type sinput struct {
	ID string `json:"sid"`
}

func (i sinput) Name() string {
	return "sname-" + i.ID
}

func schoolNamesHandler(w http.ResponseWriter, r *http.Request) {
	var inputBody []sinput
	err := getInput(r, &inputBody)
	if err != nil {
		fmt.Println("while reading input: ", err)
		return
	}

	// append sname to the id and return it.
	res := make([]interface{}, 0, len(inputBody))
	for i := 0; i < len(inputBody); i++ {
		res = append(res, "sname-"+inputBody[i].ID)
	}

	b, err := json.Marshal(res)
	if err != nil {
		fmt.Println("while marshaling result: ", err)
		return
	}
	check2(fmt.Fprint(w, string(b)))
}

func carsHandler(w http.ResponseWriter, r *http.Request) {
	var inputBody []input
	err := getInput(r, &inputBody)
	if err != nil {
		fmt.Println("while reading input: ", err)
		return
	}

	res := []interface{}{}
	for i := 0; i < len(inputBody); i++ {
		res = append(res, map[string]interface{}{
			"name": "car-" + inputBody[i].ID,
		})
	}

	b, err := json.Marshal(res)
	if err != nil {
		fmt.Println("while marshaling result: ", err)
		return
	}
	check2(fmt.Fprint(w, string(b)))
}

func classesHandler(w http.ResponseWriter, r *http.Request) {
	var inputBody []sinput
	err := getInput(r, &inputBody)
	if err != nil {
		fmt.Println("while reading input: ", err)
		return
	}

	res := []interface{}{}
	for i := 0; i < len(inputBody); i++ {
		res = append(res, []map[string]interface{}{{
			"name": "class-" + inputBody[i].ID,
		}})
	}

	b, err := json.Marshal(res)
	if err != nil {
		fmt.Println("while marshaling result: ", err)
		return
	}
	check2(fmt.Fprint(w, string(b)))
}

type entity interface {
	Name() string
}

func nameHandler(w http.ResponseWriter, r *http.Request, input entity) {
	err := getInput(r, input)
	if err != nil {
		fmt.Println("while reading input: ", err)
		return
	}

	n := fmt.Sprintf(`"%s"`, input.Name())
	check2(fmt.Fprint(w, n))
}

func userNameHandler(w http.ResponseWriter, r *http.Request) {
	var inputBody input
	nameHandler(w, r, &inputBody)
}

func carHandler(w http.ResponseWriter, r *http.Request) {
	var inputBody input
	err := getInput(r, &inputBody)
	if err != nil {
		fmt.Println("while reading input: ", err)
		return
	}

	res := map[string]interface{}{
		"name": "car-" + inputBody.ID,
	}

	b, err := json.Marshal(res)
	if err != nil {
		fmt.Println("while marshaling result: ", err)
		return
	}
	check2(fmt.Fprint(w, string(b)))
}

func classHandler(w http.ResponseWriter, r *http.Request) {
	var inputBody sinput
	err := getInput(r, &inputBody)
	if err != nil {
		fmt.Println("while reading input: ", err)
		return
	}

	res := make(map[string]interface{})
	res["name"] = "class-" + inputBody.ID

	b, err := json.Marshal([]interface{}{res})
	if err != nil {
		fmt.Println("while marshaling result: ", err)
		return
	}
	check2(fmt.Fprint(w, string(b)))
}

func teacherNameHandler(w http.ResponseWriter, r *http.Request) {
	var inputBody tinput
	nameHandler(w, r, &inputBody)
}

func schoolNameHandler(w http.ResponseWriter, r *http.Request) {
	var inputBody sinput
	nameHandler(w, r, &inputBody)
}

func introspectedSchemaForQuery(fieldName, idsField string) string {
	return generateIntrospectionResult(
		fmt.Sprintf(graphqlResponses["introspectedSchemaForQuery"].Schema, fieldName, idsField))
}

type request struct {
	Query     string
	Variables map[string]interface{}
}

func gqlUserNameHandler(w http.ResponseWriter, r *http.Request) {
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return
	}

	if strings.Contains(string(b), "__schema") {
		fmt.Fprint(w, introspectedSchemaForQuery("userName", "id"))
		return
	}

	var req request
	if err := json.Unmarshal(b, &req); err != nil {
		return
	}
	// TODO - Have tests in place either here or as part of unit tests to verify the queries
	// that are finally sent.
	userID := req.Variables["id"].(string)
	fmt.Fprintf(w, `
	{
		"data": {
		  "userName": "uname-%s"
		}
	}`, userID)
}

func gqlUserNameWithErrorHandler(w http.ResponseWriter, r *http.Request) {
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return
	}

	if strings.Contains(string(b), "__schema") {
		fmt.Fprint(w, introspectedSchemaForQuery("userName", "id"))
		return
	}

	var req request
	if err := json.Unmarshal(b, &req); err != nil {
		return
	}
	userID := req.Variables["id"].(string)
	fmt.Fprintf(w, `
	{
		"data": {
		  "userName": "uname-%s"
		},
		"errors": [
			{
				"message": "error-1 from username"
			},
			{
				"message": "error-2 from username"
			}
		]
	}`, userID)
}

func gqlCarHandler(w http.ResponseWriter, r *http.Request) {
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return
	}

	// FIXME - Return type isn't validated yet.
	if strings.Contains(string(b), "__schema") {
		fmt.Fprint(w, generateIntrospectionResult(graphqlResponses["carschema"].Schema))
		return
	}

	var req request
	if err := json.Unmarshal(b, &req); err != nil {
		return
	}

	userID := req.Variables["id"]
	fmt.Fprintf(w, `
	{
		"data": {
		  	"car": {
				"name": "car-%s"
			}
		}
	}`, userID)
}

func gqlClassHandler(w http.ResponseWriter, r *http.Request) {
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return
	}

	if strings.Contains(string(b), "__schema") {
		fmt.Fprint(w, generateIntrospectionResult(graphqlResponses["classschema"].Schema))
		return
	}

	var req request
	if err := json.Unmarshal(b, &req); err != nil {
		return
	}
	schoolID := req.Variables["id"]
	fmt.Fprintf(w, `
	{
		"data": {
		  "class": [{
			  "name": "class-%s"
		  }]
		}
	}`, schoolID)
}

func gqlTeacherNameHandler(w http.ResponseWriter, r *http.Request) {
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return
	}

	if strings.Contains(string(b), "__schema") {
		fmt.Fprint(w, introspectedSchemaForQuery("teacherName", "id"))
		return
	}

	var req request
	if err := json.Unmarshal(b, &req); err != nil {
		return
	}
	teacherID := req.Variables["tid"]
	fmt.Fprintf(w, `
	{
		"data": {
		  "teacherName": "tname-%s"
		}
	}`, teacherID)
}

func gqlSchoolNameHandler(w http.ResponseWriter, r *http.Request) {
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return
	}

	if strings.Contains(string(b), "__schema") {
		fmt.Fprint(w, introspectedSchemaForQuery("schoolName", "id"))
		return
	}

	var req request
	if err := json.Unmarshal(b, &req); err != nil {
		return
	}
	schoolID := req.Variables["id"]
	fmt.Fprintf(w, `
	{
		"data": {
		  "schoolName": "sname-%s"
		}
	}`, schoolID)
}

func introspectionResult(name string) string {
	return generateIntrospectionResult(fmt.Sprintf(graphqlResponses["introspectionresults"].Schema,
		name))
}

func makeResponse(b []byte, id, key, prefix string) (string, error) {
	var req request
	if err := json.Unmarshal(b, &req); err != nil {
		return "", err
	}
	input := req.Variables["input"]
	output := []string{}
	for _, i := range input.([]interface{}) {
		im := i.(map[string]interface{})
		id := im[id].(string)
		output = append(output, prefix+id)
	}

	response := map[string]interface{}{
		"data": map[string]interface{}{
			key: output,
		},
	}

	b, err := json.Marshal(response)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func gqlUserNamesHandler(w http.ResponseWriter, r *http.Request) {
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return
	}

	if strings.Contains(string(b), "__schema") {
		fmt.Fprint(w, introspectionResult("userNames"))
		return
	}

	res, err := makeResponse(b, "id", "userNames", "uname-")
	if err != nil {
		return
	}
	fmt.Fprint(w, res)
}

func gqlTeacherNamesHandler(w http.ResponseWriter, r *http.Request) {
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return
	}

	if strings.Contains(string(b), "__schema") {
		fmt.Fprint(w, introspectionResult("teacherNames"))
		return
	}

	res, err := makeResponse(b, "tid", "teacherNames", "tname-")
	if err != nil {
		return
	}
	fmt.Fprint(w, res)
}

func gqlSchoolNamesHandler(w http.ResponseWriter, r *http.Request) {
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return
	}

	if strings.Contains(string(b), "__schema") {
		fmt.Fprint(w, introspectionResult("schoolNames"))
		return
	}

	res, err := makeResponse(b, "id", "schoolNames", "sname-")
	if err != nil {
		return
	}
	fmt.Fprint(w, res)
}

func buildCarBatchOutput(b []byte, req request) []interface{} {
	input := req.Variables["input"]
	output := []interface{}{}
	for _, i := range input.([]interface{}) {
		im := i.(map[string]interface{})
		id := im["id"].(string)
		output = append(output, map[string]interface{}{
			"name": "car-" + id,
		})
	}
	return output
}

func gqlCarsHandler(w http.ResponseWriter, r *http.Request) {
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return
	}

	if strings.Contains(string(b), "__schema") {
		fmt.Fprint(w, generateIntrospectionResult(graphqlResponses["carsschema"].Schema))
		return
	}

	var req request
	if err := json.Unmarshal(b, &req); err != nil {
		return
	}

	output := buildCarBatchOutput(b, req)
	response := map[string]interface{}{
		"data": map[string]interface{}{
			"cars": output,
		},
	}

	b, err = json.Marshal(response)
	if err != nil {
		return
	}
	check2(fmt.Fprint(w, string(b)))
}

func gqlCarsWithErrorHandler(w http.ResponseWriter, r *http.Request) {
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return
	}

	if strings.Contains(string(b), "__schema") {
		fmt.Fprint(w, generateIntrospectionResult(graphqlResponses["carsschema"].Schema))
		return
	}

	var req request
	if err := json.Unmarshal(b, &req); err != nil {
		return
	}

	output := buildCarBatchOutput(b, req)
	response := map[string]interface{}{
		"data": map[string]interface{}{
			"cars": output,
		},
		"errors": []map[string]interface{}{
			map[string]interface{}{
				"message": "error-1 from cars",
			},
			map[string]interface{}{
				"message": "error-2 from cars",
			},
		},
	}

	b, err = json.Marshal(response)
	if err != nil {
		return
	}
	check2(fmt.Fprint(w, string(b)))
}

func gqlClassesHandler(w http.ResponseWriter, r *http.Request) {
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return
	}

	if strings.Contains(string(b), "__schema") {
		fmt.Fprint(w, generateIntrospectionResult(graphqlResponses["classesschema"].Schema))
		return
	}

	var req request
	if err := json.Unmarshal(b, &req); err != nil {
		return
	}
	input := req.Variables["input"]
	output := []interface{}{}
	for _, i := range input.([]interface{}) {
		im := i.(map[string]interface{})
		id := im["id"].(string)
		output = append(output, []map[string]interface{}{
			{
				"name": "class-" + id,
			},
		})
	}

	response := map[string]interface{}{
		"data": map[string]interface{}{
			"classes": output,
		},
	}

	b, err = json.Marshal(response)
	if err != nil {
		return
	}
	check2(fmt.Fprint(w, string(b)))
}

func main() {
	/*************************************
	* For testing http without graphql
	*************************************/

	// for queries
	http.HandleFunc("/favMovies/", getFavMoviesHandler)
	http.HandleFunc("/favMoviesPost/", postFavMoviesHandler)
	http.HandleFunc("/verifyHeaders", verifyHeadersHandler)

	// for mutations
	http.HandleFunc("/favMoviesCreate", favMoviesCreateHandler)
	http.HandleFunc("/favMoviesUpdate/", favMoviesUpdateHandler)
	http.HandleFunc("/favMoviesDelete/", favMoviesDeleteHandler)

	// The endpoints below are for testing custom resolution of fields within type definitions.
	// for testing batch mode
	http.HandleFunc("/userNames", userNamesHandler)
	http.HandleFunc("/cars", carsHandler)
	http.HandleFunc("/classes", classesHandler)
	http.HandleFunc("/teacherNames", teacherNamesHandler)
	http.HandleFunc("/schoolNames", schoolNamesHandler)

	// for testing single mode
	http.HandleFunc("/userName", userNameHandler)
	http.HandleFunc("/car", carHandler)
	http.HandleFunc("/class", classHandler)
	http.HandleFunc("/teacherName", teacherNameHandler)
	http.HandleFunc("/schoolName", schoolNameHandler)

	/*************************************
	* For testing http with graphql
	*************************************/

	// for remote schema validation
	http.HandleFunc("/noquery", emptyQuerySchema)
	http.HandleFunc("/invalidargument", commonGraphqlHandler("invalidargument"))
	http.HandleFunc("/invalidtype", commonGraphqlHandler("invalidtype"))
	http.HandleFunc("/nullQueryAndMutationType", nullQueryAndMutationType)
	http.HandleFunc("/missingQueryAndMutationType", missingQueryAndMutationType)
	http.HandleFunc("/invalidInputForBatchedField", invalidInputForBatchedField)
	http.HandleFunc("/missingTypeForBatchedFieldInput", missingTypeForBatchedFieldInput)

	// for queries
	http.HandleFunc("/validcountry", commonGraphqlHandler("validcountry"))
	http.HandleFunc("/validcountrywitherror", commonGraphqlHandler("validcountrywitherror"))
	http.HandleFunc("/graphqlerr", commonGraphqlHandler("graphqlerr"))
	http.HandleFunc("/validcountries", commonGraphqlHandler("validcountries"))
	http.HandleFunc("/validinpputfield", commonGraphqlHandler("validinpputfield"))
	http.HandleFunc("/invalidfield", commonGraphqlHandler("invalidfield"))
	http.HandleFunc("/nestedinvalid", commonGraphqlHandler("nestedinvalid"))

	// for mutations
	http.HandleFunc("/setCountry", commonGraphqlHandler("setcountry"))
	http.HandleFunc("/updateCountries", commonGraphqlHandler("updatecountries"))

	// for testing single mode
	http.HandleFunc("/gqlUserName", gqlUserNameHandler)
	http.HandleFunc("/gqlUserNameWithError", gqlUserNameWithErrorHandler)
	http.HandleFunc("/gqlCar", gqlCarHandler)
	http.HandleFunc("/gqlClass", gqlClassHandler)
	http.HandleFunc("/gqlTeacherName", gqlTeacherNameHandler)
	http.HandleFunc("/gqlSchoolName", gqlSchoolNameHandler)

	// for testing in batch mode
	http.HandleFunc("/getPosts", getPosts)
	http.HandleFunc("/getPostswithLike", getPostswithLike)
	http.HandleFunc("/gqlUserNames", gqlUserNamesHandler)
	http.HandleFunc("/gqlCars", gqlCarsHandler)
	http.HandleFunc("/gqlCarsWithErrors", gqlCarsWithErrorHandler)
	http.HandleFunc("/gqlClasses", gqlClassesHandler)
	http.HandleFunc("/gqlTeacherNames", gqlTeacherNamesHandler)
	http.HandleFunc("/gqlSchoolNames", gqlSchoolNamesHandler)

	fmt.Println("Listening on port 8888")
	log.Fatal(http.ListenAndServe(":8888", nil))
}
