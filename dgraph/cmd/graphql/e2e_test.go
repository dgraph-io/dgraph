/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
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

package graphql

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/x"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

const (
	graphqlURL = "http://localhost:9000/graphql"
	alphagRPC  = "localhost:9180"
)

// GraphQLParams is parameters for the constructing a GraphQL query - that's
// http POST with this body, or http GET with this in the query string.
//
// https://graphql.org/learn/serving-over-http/ says:
//
// POST
// ----
// 'A standard GraphQL POST request should use the application/json content type,
// and include a JSON-encoded body of the following form:
// {
// 	  "query": "...",
// 	  "operationName": "...",
// 	  "variables": { "myVariable": "someValue", ... }
// }
// operationName and variables are optional fields. operationName is only
// required if multiple operations are present in the query.'
//
//
// GET
// ---
//
// http://myapi/graphql?query={me{name}}
// "Query variables can be sent as a JSON-encoded string in an additional query parameter
// called variables. If the query contains several named operations, an operationName query
// parameter can be used to control which one should be executed."
type GraphQLParams struct {
	Query         string                 `json:"query"`
	OperationName string                 `json:"operationName"`
	Variables     map[string]interface{} `json:"variables"`
}

// GraphQLResponse GraphQL response structure.
// see https://graphql.github.io/graphql-spec/June2018/#sec-Response
type GraphQLResponse struct {
	Data       json.RawMessage        `json:"data,omitempty"`
	Errors     []*x.GqlError          `json:"errors,omitempty"`
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

type country struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type author struct {
	ID         string
	Name       string
	Dob        time.Time
	Reputation float32
	Country    country
	Posts      []post
}

type post struct {
	PostID      string
	Title       string
	Text        string
	Tags        []string
	NulLikes    int
	IsPublished bool
	PostType    string
	Author      author
}

// ExecuteAsPost builds a HTTP POST request from the GraphQL input structure
// and executes the request to url.
func (params *GraphQLParams) ExecuteAsPost(t *testing.T, url string) *GraphQLResponse {
	req, err := params.createGQLPost(url)
	require.NoError(t, err)

	res, err := runGQLRequest(req)
	require.NoError(t, err)

	var result *GraphQLResponse
	err = json.Unmarshal(res, &result)
	require.NoError(t, err)

	requireContainsRequestID(t, result)

	return result
}

// ExecuteAsGet builds a HTTP GET request from the GraphQL input structure
// and executes the request to url.
func (params *GraphQLParams) ExecuteAsGet(url string) ([]byte, error) {
	return nil, errors.New("GET not yet supported")
}

func (params *GraphQLParams) createGQLPost(url string) (*http.Request, error) {
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
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	// GraphQL server should always return OK, even when there are errors
	if status := resp.StatusCode; status != http.StatusOK {
		return nil, errors.Errorf("unexpected status code: %v", status)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Errorf("unable to read response body: %v", err)
	}

	return body, nil
}

func requireContainsRequestID(t *testing.T, resp *GraphQLResponse) {
	v, ok := resp.Extensions["requestID"]
	require.True(t, ok, "GraphQL response didn't contain a request ID")

	str, ok := v.(string)
	require.True(t, ok, "GraphQL requestID is not a string")

	_, err := uuid.Parse(str)
	require.NoError(t, err, "GraphQL requestID is not a UUID")
}

func requireUID(t *testing.T, uid string) {
	_, err := strconv.ParseUint(uid, 0, 64)
	require.NoError(t, err)
}
