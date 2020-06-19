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
	"context"
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

	graphql "github.com/graph-gophers/graphql-go"
	"github.com/graph-gophers/graphql-go/relay"
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

func compareHeaders(headers map[string][]string, actual http.Header) error {
	if headers == nil {
		return nil
	}
	actualHeaderLen := len(actual)
	expectedHeaderLen := len(headers)
	if actualHeaderLen != expectedHeaderLen {
		return getError(fmt.Sprintf("Wanted %d headers in request, got", expectedHeaderLen),
			strconv.Itoa(actualHeaderLen))
	}

	for k, v := range headers {
		rv, ok := actual[k]
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
	return nil
}

func verifyRequest(r *http.Request, expectedRequest expectedRequest) error {
	if r.Method != expectedRequest.method {
		return getError("Invalid HTTP method", r.Method)
	}

	if !strings.HasSuffix(r.URL.String(), expectedRequest.urlSuffix) {
		return getError("Invalid URL", r.URL.String())
	}

	if expectedRequest.body == "" && r.Body != http.NoBody {
		return getError("Expected No body", "but got some body to read")
	}

	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return getError("Unable to read request body", err.Error())
	}
	if string(b) != expectedRequest.body {
		return getError("Unexpected value for request body", string(b))
	}

	return compareHeaders(expectedRequest.headers, r.Header)
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
			"X-App-Token":      {"app-token"},
			"X-User-Id":        {"123"},
			"Github-Api-Token": {"random-fake-token"},
			"Accept-Encoding":  nil,
			"User-Agent":       nil,
		},
	})
	if err != nil {
		check2(w.Write([]byte(err.Error())))
		return
	}
	check2(w.Write([]byte(`[{"id":"0x3","name":"Star Wars"}]`)))
}

func verifyCustomNameHeadersHandler(w http.ResponseWriter, r *http.Request) {
	err := verifyRequest(r, expectedRequest{
		method:    http.MethodGet,
		urlSuffix: "/verifyCustomNameHeaders",
		body:      "",
		headers: map[string][]string{
			"X-App-Token":      {"app-token"},
			"X-User-Id":        {"123"},
			"Authorization": {"random-fake-token"},
			"Accept-Encoding":  nil,
			"User-Agent":       nil,
		},
	})
	if err != nil {
		check2(w.Write([]byte(err.Error())))
		return
	}
	check2(w.Write([]byte(`[{"id":"0x3","name":"Star Wars"}]`)))
}

func twitterFollwerHandler(w http.ResponseWriter, r *http.Request) {
	err := verifyRequest(r, expectedRequest{
		method:    http.MethodGet,
		urlSuffix: "/twitterfollowers?screen_name=manishrjain",
		body:      "",
	})
	if err != nil {
		check2(w.Write([]byte(err.Error())))
		return
	}
	check2(w.Write([]byte(`
	{
		"users": [{
			"id": 1231723732206411776,
			"name": "hi_balaji",
			"screen_name": "hi_balaji",
			"location": "",
			"description": "",
			"followers_count": 0,
			"friends_count": 117,
			"statuses_count": 0
		}]
	}`)))
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

func deleteCommonHeaders(headers http.Header) {
	delete(headers, "Accept-Encoding")
	delete(headers, "Content-Length")
	delete(headers, "User-Agent")
}

func carsHandlerWithHeaders(w http.ResponseWriter, r *http.Request) {
	deleteCommonHeaders(r.Header)
	if err := compareHeaders(map[string][]string{
		"Stripe-Api-Key": []string{"some-api-key"},
	}, r.Header); err != nil {
		check2(w.Write([]byte(err.Error())))
		return
	}
	check2(fmt.Fprint(w, `[{"name": "foo"},{"name": "foo"},{"name": "foo"}]`))
}

func userNameHandlerWithHeaders(w http.ResponseWriter, r *http.Request) {
	deleteCommonHeaders(r.Header)
	if err := compareHeaders(map[string][]string{
		"Github-Api-Token": []string{"some-api-token"},
	}, r.Header); err != nil {
		check2(w.Write([]byte(err.Error())))
		return
	}
	check2(fmt.Fprint(w, `"foo"`))
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

type query struct{}

type country struct {
	Code graphql.ID
	Name string
}

type countryResolver struct {
	c *country
}

func (r countryResolver) Code() *string {
	s := string(r.c.Code)
	return &(s)
}

func (r countryResolver) Name() *string {
	return &(r.c.Name)
}

func (_ *query) Country(ctx context.Context, args struct {
	Code string
}) countryResolver {
	return countryResolver{&country{Code: graphql.ID(args.Code), Name: "Burundi"}}
}

func (_ *query) Countries(ctx context.Context, args struct {
	Filter struct {
		Code string
		Name string
	}
}) []countryResolver {
	return []countryResolver{countryResolver{&country{
		Code: graphql.ID(args.Filter.Code),
		Name: args.Filter.Name,
	}}}
}

func (_ *query) ValidCountries(ctx context.Context, args struct {
	Code string
}) *[]*countryResolver {
	return &[]*countryResolver{{&country{Code: graphql.ID(args.Code), Name: "Burundi"}}}
}

func (_ *query) UserName(ctx context.Context, args struct {
	Id string
}) *string {
	s := fmt.Sprintf(`uname-%s`, args.Id)
	return &s
}

func (_ *query) TeacherName(ctx context.Context, args struct {
	Id string
}) *string {
	s := fmt.Sprintf(`tname-%s`, args.Id)
	return &s
}

func (_ *query) SchoolName(ctx context.Context, args struct {
	Id string
}) *string {
	s := fmt.Sprintf(`sname-%s`, args.Id)
	return &s
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

type car struct {
	ID graphql.ID
}

type carResolver struct {
	c *car
}

func (r *carResolver) ID() graphql.ID {
	return r.c.ID
}

func (r *carResolver) Name() string {
	return "car-" + string(r.c.ID)
}

func (_ *query) Car(ctx context.Context, args struct {
	Id string
}) *carResolver {
	return &carResolver{&car{ID: graphql.ID(args.Id)}}
}

type class struct {
	ID graphql.ID
}

type classResolver struct {
	c *class
}

func (r *classResolver) ID() graphql.ID {
	return r.c.ID
}

func (r *classResolver) Name() string {
	return "class-" + string(r.c.ID)
}

func (_ *query) Class(ctx context.Context, args struct {
	Id string
}) *[]*classResolver {
	return &[]*classResolver{&classResolver{&class{ID: graphql.ID(args.Id)}}}
}

func (_ *query) UserNames(ctx context.Context, args struct {
	Users *[]*struct {
		Id  string
		Age float64
	}
}) *[]*string {
	res := make([]*string, 0)
	if args.Users == nil {
		return nil
	}
	for _, arg := range *args.Users {
		n := fmt.Sprintf(`uname-%s`, arg.Id)
		res = append(res, &n)
	}
	return &res
}

func (_ *query) Cars(ctx context.Context, args struct {
	Users *[]*struct {
		Id  string
		Age float64
	}
}) *[]*carResolver {
	if args.Users == nil {
		return nil
	}
	resolvers := make([]*carResolver, 0, len(*args.Users))
	for _, user := range *args.Users {
		resolvers = append(resolvers, &carResolver{&car{ID: graphql.ID(user.Id)}})
	}
	return &resolvers
}

func (_ *query) Classes(ctx context.Context, args struct {
	Schools *[]*struct {
		Id          string
		Established float64
	}
}) *[]*[]*classResolver {
	if args.Schools == nil {
		return nil
	}
	resolvers := make([]*[]*classResolver, 0, len(*args.Schools))
	for _, user := range *args.Schools {
		resolvers = append(resolvers, &[]*classResolver{
			&classResolver{&class{ID: graphql.ID(user.Id)}}})
	}
	return &resolvers
}

func (_ *query) TeacherNames(ctx context.Context, args struct {
	Teachers *[]*struct {
		Tid string
		Age float64
	}
}) *[]*string {
	if args.Teachers == nil {
		return nil
	}
	res := make([]*string, 0)
	for _, arg := range *args.Teachers {
		n := fmt.Sprintf(`tname-%s`, arg.Tid)
		res = append(res, &n)
	}
	return &res
}

func (_ *query) SchoolNames(ctx context.Context, args struct {
	Schools *[]*struct {
		Id          string
		Established float64
	}
}) *[]*string {
	if args.Schools == nil {
		return nil
	}
	res := make([]*string, 0)
	for _, arg := range *args.Schools {
		n := fmt.Sprintf(`sname-%s`, arg.Id)
		res = append(res, &n)
	}
	return &res
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

func main() {
	/*************************************
	* For testing http without graphql
	*************************************/

	// for queries
	http.HandleFunc("/favMovies/", getFavMoviesHandler)
	http.HandleFunc("/favMoviesPost/", postFavMoviesHandler)
	http.HandleFunc("/verifyHeaders", verifyHeadersHandler)
	http.HandleFunc("/verifyCustomNameHeaders", verifyCustomNameHeadersHandler)
	http.HandleFunc("/twitterfollowers", twitterFollwerHandler)

	// for mutations
	http.HandleFunc("/favMoviesCreate", favMoviesCreateHandler)
	http.HandleFunc("/favMoviesUpdate/", favMoviesUpdateHandler)
	http.HandleFunc("/favMoviesDelete/", favMoviesDeleteHandler)

	// The endpoints below are for testing custom resolution of fields within type definitions.
	// for testing batch mode
	http.HandleFunc("/userNames", userNamesHandler)
	http.HandleFunc("/cars", carsHandler)
	http.HandleFunc("/checkHeadersForCars", carsHandlerWithHeaders)
	http.HandleFunc("/classes", classesHandler)
	http.HandleFunc("/teacherNames", teacherNamesHandler)
	http.HandleFunc("/schoolNames", schoolNamesHandler)

	// for testing single mode
	http.HandleFunc("/userName", userNameHandler)
	http.HandleFunc("/checkHeadersForUserName", userNameHandlerWithHeaders)
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
	vsch := graphql.MustParseSchema(graphqlResponses["validcountry"].Schema, &query{})
	http.Handle("/validcountry", &relay.Handler{Schema: vsch})
	http.HandleFunc("/argsonfields", commonGraphqlHandler("argsonfields"))
	http.HandleFunc("/validcountrywitherror", commonGraphqlHandler("validcountrywitherror"))
	http.HandleFunc("/graphqlerr", commonGraphqlHandler("graphqlerr"))
	http.Handle("/validcountries", &relay.Handler{
		Schema: graphql.MustParseSchema(graphqlResponses["validcountries"].Schema, &query{}),
	})
	http.Handle("/validinputfield", &relay.Handler{
		Schema: graphql.MustParseSchema(graphqlResponses["validinputfield"].Schema, &query{}),
	})
	http.HandleFunc("/invalidfield", commonGraphqlHandler("invalidfield"))
	http.HandleFunc("/nestedinvalid", commonGraphqlHandler("nestedinvalid"))
	http.HandleFunc("/validatesecrettoken", func(w http.ResponseWriter, r *http.Request) {
		if h := r.Header.Get("Github-Api-Token"); h != "random-api-token" {
			return
		}
		rh := &relay.Handler{
			Schema: graphql.MustParseSchema(graphqlResponses["validinputfield"].Schema, &query{}),
		}
		rh.ServeHTTP(w, r)
	})

	// for mutations
	http.HandleFunc("/setCountry", commonGraphqlHandler("setcountry"))
	http.HandleFunc("/updateCountries", commonGraphqlHandler("updatecountries"))

	// for testing single mode
	sch := graphql.MustParseSchema(graphqlResponses["singleOperationSchema"].Schema, &query{})
	h := &relay.Handler{Schema: sch}
	http.Handle("/gqlUserName", h)
	// TODO - Figure out how to return multiple errors and then replace the handler below.
	http.HandleFunc("/gqlUserNameWithError", gqlUserNameWithErrorHandler)
	http.Handle("/gqlCar", h)
	http.Handle("/gqlClass", h)
	http.Handle("/gqlTeacherName", h)
	http.Handle("/gqlSchoolName", h)

	// for testing in batch mode
	bsch := graphql.MustParseSchema(graphqlResponses["batchOperationSchema"].Schema, &query{})
	bh := &relay.Handler{Schema: bsch}
	http.HandleFunc("/getPosts", getPosts)
	http.HandleFunc("/getPostswithLike", getPostswithLike)
	http.Handle("/gqlUserNames", bh)
	http.Handle("/gqlCars", bh)
	http.HandleFunc("/gqlCarsWithErrors", gqlCarsWithErrorHandler)
	http.Handle("/gqlClasses", bh)
	http.Handle("/gqlTeacherNames", bh)
	http.Handle("/gqlSchoolNames", bh)

	fmt.Println("Listening on port 8888")
	log.Fatal(http.ListenAndServe(":8888", nil))
}
