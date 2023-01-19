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

package upgrade

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/dgraph-io/dgo/v210"
	"github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/x"
)

func hasAclCreds() bool {
	return len(Upgrade.Conf.GetString(user)) > 0
}

// getAccessJwt gets the access jwt token from by logging into the cluster.
func getAccessJwt() (*api.Jwt, error) {
	if !hasAclCreds() {
		return &api.Jwt{}, nil
	}
	user := Upgrade.Conf.GetString(user)
	password := Upgrade.Conf.GetString(password)
	header := http.Header{}
	header.Set("X-Dgraph-AuthToken", Upgrade.Conf.GetString(authToken))
	updateSchemaParams := &GraphQLParams{
		Query: `mutation login($userId: String, $password: String, $namespace: Int) {
			login(userId: $userId, password: $password, namespace: $namespace) {
				response {
					accessJWT
				}
			}
		}`,
		Variables: map[string]interface{}{"userId": user, "password": password,
			"namespace": x.GalaxyNamespace},
		Headers: header,
	}

	adminUrl := Upgrade.Conf.GetString(alphaHttp) + "/admin"
	resp, err := makeGqlRequest(updateSchemaParams, adminUrl)
	if err != nil {
		return nil, err
	}
	if len(resp.Errors) > 0 {
		return nil, errors.Errorf("Error while updating the schema %s\n", resp.Errors.Error())
	}

	type Response struct {
		Login struct {
			Response struct {
				AccessJWT string
			}
		}
	}
	var r Response
	if err := json.Unmarshal(resp.Data, &r); err != nil {
		return nil, err
	}

	jwt := &api.Jwt{AccessJwt: r.Login.Response.AccessJWT}
	return jwt, nil
}

type GraphQLParams struct {
	Query         string                    `json:"query"`
	OperationName string                    `json:"operationName"`
	Variables     map[string]interface{}    `json:"variables"`
	Extensions    *schema.RequestExtensions `json:"extensions,omitempty"`
	Headers       http.Header
}

// GraphQLResponse GraphQL response structure.
// see https://graphql.github.io/graphql-spec/June2018/#sec-Response
type GraphQLResponse struct {
	Data       json.RawMessage        `json:"data,omitempty"`
	Errors     x.GqlErrorList         `json:"errors,omitempty"`
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

func makeGqlRequest(params *GraphQLParams, url string) (*GraphQLResponse, error) {
	req, err := createGQLPost(params, url)
	if err != nil {
		return nil, err
	}
	for h := range params.Headers {
		req.Header.Set(h, params.Headers.Get(h))
	}
	res, err := runGQLRequest(req)
	if err != nil {
		return nil, err
	}

	var result *GraphQLResponse
	if err := json.Unmarshal(res, &result); err != nil {
		return nil, err
	}

	return result, nil
}

func createGQLPost(params *GraphQLParams, url string) (*http.Request, error) {
	body, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	return req, nil
}

// runGQLRequest runs a HTTP GraphQL request and returns the data or any errors.
func runGQLRequest(req *http.Request) ([]byte, error) {
	config, err := x.LoadClientTLSConfig(Upgrade.Conf)
	if err != nil {
		return nil, err
	}
	tr := &http.Transport{TLSClientConfig: config}
	client := &http.Client{Timeout: 50 * time.Second, Transport: tr}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	// GraphQL server should always return OK, even when there are errors
	if status := resp.StatusCode; status != http.StatusOK {
		return nil, errors.Errorf("unexpected status code: %v", status)
	}

	if strings.ToLower(resp.Header.Get("Content-Type")) != "application/json" {
		return nil, errors.Errorf("unexpected content type: %v", resp.Header.Get("Content-Type"))
	}

	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		return nil, errors.Errorf("cors headers weren't set in response")
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Errorf("unable to read response body: %v", err)
	}

	return body, nil
}

// getQueryResult executes the given query and unmarshals the result in given pointer queryResPtr.
// If any error is encountered, it returns the error.
func getQueryResult(dg *dgo.Dgraph, query string, queryResPtr interface{}) error {
	var resp *api.Response
	var err error

	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		resp, err = dg.NewReadOnlyTxn().Query(ctx, query)
		if err != nil {
			fmt.Println("error in query, retrying:", err)
			continue
		}
	}
	if err != nil {
		return err
	}

	return json.Unmarshal(resp.GetJson(), queryResPtr)
}

// mutateWithClient uses the given dgraph client to execute the given mutation.
// It retries max 3 times before returning failure error, if any.
func mutateWithClient(dg *dgo.Dgraph, mutation *api.Mutation) error {
	if mutation == nil {
		return nil
	}

	mutation.CommitNow = true

	var err error
	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err = dg.NewTxn().Mutate(ctx, mutation)
		if err != nil {
			fmt.Println("error in mutation, retrying:", err)
			continue
		}

		return nil
	}

	return err
}

// alterWithClient uses the given dgraph client to execute the given alter operation.
// It retries max 3 times before returning failure error, if any.
func alterWithClient(dg *dgo.Dgraph, operation *api.Operation) error {
	if operation == nil {
		return nil
	}

	var err error
	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = dg.Alter(ctx, operation)
		if err != nil {
			fmt.Println("error in alter, retrying:", err)
			continue
		}

		return nil
	}

	return err
}

// getTypeSchemaString generates a string which can be used to alter a type in schema.
// It generates the type string using new type and predicate names. So, if this type
// previously had a predicate for which we got a new name, then the generated string
// will contain the new name for that predicate. Also, if some predicates need to be
// removed from the type, then they can be supplied in predsToRemove. For example:
// initialType:
//
//	type Person {
//		name
//		age
//		unnecessaryEdge
//	}
//
// also,
//
//	newTypeName = "Human"
//	newPredNames = {
//		"age": "ageOnEarth'
//	}
//	predsToRemove = {
//		"unnecessaryEdge": {}
//	}
//
// then returned type string will be:
//
//	type Human {
//		name
//		ageOnEarth
//	}
func getTypeSchemaString(newTypeName string, typeNode *schemaTypeNode,
	newPredNames map[string]string, predsToRemove map[string]struct{}) string {
	var builder strings.Builder
	builder.WriteString("type ")
	builder.WriteString(newTypeName)
	builder.WriteString(" {\n")

	for _, oldPred := range typeNode.Fields {
		if _, ok := predsToRemove[oldPred.Name]; ok {
			continue
		}

		builder.WriteString("  ")
		newPredName, ok := newPredNames[oldPred.Name]
		if ok {
			builder.WriteString(newPredName)
		} else {
			builder.WriteString(oldPred.Name)
		}
		builder.WriteString("\n")
	}

	builder.WriteString("}\n")

	return builder.String()
}

func getTypeNquad(uid, typeName string) *api.NQuad {
	return &api.NQuad{
		Subject:   uid,
		Predicate: "dgraph.type",
		ObjectValue: &api.Value{
			Val: &api.Value_StrVal{StrVal: typeName},
		},
	}
}
