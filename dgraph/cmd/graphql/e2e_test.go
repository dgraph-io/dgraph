/*
 *    Copyright 2019 Dgraph Labs, Inc. and Contributors
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
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/v2"
	"github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

const (
	graphqlURL      = "http://localhost:9100/graphql"
	graphqlAdminURL = "http://localhost:9100/admin"
	alphagRPC       = "localhost:9180"

	graphqlAdminTestURL      = "http://localhost:9200/graphql"
	graphqlAdminTestAdminURL = "http://localhost:9200/admin"
	alphaAdminTestgRPC       = "localhost:9280"
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
//
// acceptGzip sends "Accept-Encoding: gzip" header to the server, which would return the
// response after gzip.
// gzipEncoding would compress the request to the server and add "Content-Encoding: gzip"
// header to the same.

type GraphQLParams struct {
	Query         string                 `json:"query"`
	OperationName string                 `json:"operationName"`
	Variables     map[string]interface{} `json:"variables"`
	acceptGzip    bool
	gzipEncoding  bool
}

type requestExecutor func(t *testing.T, url string, params *GraphQLParams) *GraphQLResponse

// GraphQLResponse GraphQL response structure.
// see https://graphql.github.io/graphql-spec/June2018/#sec-Response
type GraphQLResponse struct {
	Data       json.RawMessage        `json:"data,omitempty"`
	Errors     x.GqlErrorList         `json:"errors,omitempty"`
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
	NumLikes    int
	IsPublished bool
	PostType    string
	Author      author
}

type state struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Code string `json:"code"`
}

func TestMain(m *testing.M) {
	err := checkGraphQLLayerStarted(graphqlAdminURL)
	if err != nil {
		panic(fmt.Sprintf("Waited for GraphQL test server to become available, but it never did.\n"+
			"Got last error %+v", err.Error()))
	}

	err = checkGraphQLLayerStarted(graphqlAdminTestAdminURL)
	if err != nil {
		panic(fmt.Sprintf("Waited for GraphQL AdminTest server to become available, "+
			"but it never did.\n Got last error: %+v", err.Error()))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	d, err := grpc.DialContext(ctx, alphagRPC, grpc.WithInsecure())
	if err != nil {
		panic(err)
	}
	client := dgo.NewDgraphClient(api.NewDgraphClient(d))

	schemaFile := "e2e_test_schema.graphql"
	schema, err := ioutil.ReadFile(schemaFile)
	if err != nil {
		panic(err)
	}

	err = addSchema(graphqlAdminURL, string(schema))
	if err != nil {
		panic(err)
	}

	err = populateGraphQLData(client)
	if err != nil {
		panic(err)
	}

	err = checkGraphQLHealth(graphqlAdminURL, []string{"Healthy"})
	if err != nil {
		panic(err)
	}

	d.Close()

	os.Exit(m.Run())
}

func gunzipData(data []byte) ([]byte, error) {
	b := bytes.NewBuffer(data)

	r, err := gzip.NewReader(b)
	if err != nil {
		return nil, err
	}

	var resB bytes.Buffer
	if _, err := resB.ReadFrom(r); err != nil {
		return nil, err
	}
	return resB.Bytes(), nil
}

func gzipData(data []byte) ([]byte, error) {
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)

	if _, err := gz.Write(data); err != nil {
		return nil, err
	}

	if err := gz.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// This tests that if a request has gzip header but the body is
// not compressed, then it should return an error
func TestGzipCompressionHeader(t *testing.T) {
	queryCountry := &GraphQLParams{
		Query: `query {
			queryCountry {
				name
			}
		}`,
	}

	req, err := queryCountry.createGQLPost(graphqlURL)
	require.NoError(t, err)

	req.Header.Set("Content-Encoding", "gzip")

	resData, err := runGQLRequest(req)

	var result *GraphQLResponse
	err = json.Unmarshal(resData, &result)
	require.NotNil(t, result.Errors)
	require.Contains(t, result.Errors[0].Message, "Unable to parse gzip")
}

// This tests that if a req's body is compressed but the
// header is not present, then it should return an error
func TestGzipCompressionNoHeader(t *testing.T) {
	queryCountry := &GraphQLParams{
		Query: `query {
			queryCountry {
				name
			}
		}`,
		gzipEncoding: true,
	}

	req, err := queryCountry.createGQLPost(graphqlURL)
	require.NoError(t, err)

	req.Header.Del("Content-Encoding")
	resData, err := runGQLRequest(req)

	var result *GraphQLResponse
	err = json.Unmarshal(resData, &result)
	require.NotNil(t, result.Errors)
	require.Contains(t, result.Errors[0].Message, "Not a valid GraphQL request body")
}

func TestGetRequest(t *testing.T) {
	AddMutation(t, getExecutor)
}

func TestGetQueryEmptyVariable(t *testing.T) {
	queryCountry := &GraphQLParams{
		Query: `query {
			queryCountry {
				name
			}
		}`,
	}
	req, err := queryCountry.createGQLGet(graphqlURL)
	require.NoError(t, err)

	q := req.URL.Query()
	q.Del("variables")
	req.URL.RawQuery = q.Encode()

	res := queryCountry.Execute(t, req)
	require.Nil(t, res.Errors)
}

// Execute takes a HTTP request from either ExecuteAsPost or ExecuteAsGet
// and executes the request
func (params *GraphQLParams) Execute(t *testing.T, req *http.Request) *GraphQLResponse {
	res, err := runGQLRequest(req)
	require.NoError(t, err)

	var result *GraphQLResponse
	if params.acceptGzip {
		res, err = gunzipData(res)
		require.NoError(t, err)
		require.Contains(t, req.Header.Get("Accept-Encoding"), "gzip")
	}
	err = json.Unmarshal(res, &result)
	require.NoError(t, err)

	requireContainsRequestID(t, result)

	return result

}

// ExecuteAsPost builds a HTTP POST request from the GraphQL input structure
// and executes the request to url.
func (params *GraphQLParams) ExecuteAsPost(t *testing.T, url string) *GraphQLResponse {
	req, err := params.createGQLPost(url)
	require.NoError(t, err)

	return params.Execute(t, req)
}

// ExecuteAsGet builds a HTTP GET request from the GraphQL input structure
// and executes the request to url.
func (params *GraphQLParams) ExecuteAsGet(t *testing.T, url string) *GraphQLResponse {
	req, err := params.createGQLGet(url)
	require.NoError(t, err)

	return params.Execute(t, req)
}

func getExecutor(t *testing.T, url string, params *GraphQLParams) *GraphQLResponse {
	return params.ExecuteAsGet(t, url)
}

func postExecutor(t *testing.T, url string, params *GraphQLParams) *GraphQLResponse {
	return params.ExecuteAsPost(t, url)
}

func (params *GraphQLParams) createGQLGet(url string) (*http.Request, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	q.Add("query", params.Query)
	q.Add("operationName", params.OperationName)

	variableString, err := json.Marshal(params.Variables)
	if err != nil {
		return nil, err
	}
	q.Add("variables", string(variableString))

	req.URL.RawQuery = q.Encode()
	if params.acceptGzip {
		req.Header.Set("Accept-Encoding", "gzip")
	}
	return req, nil
}

func (params *GraphQLParams) createGQLPost(url string) (*http.Request, error) {
	body, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	if params.gzipEncoding {
		if body, err = gzipData(body); err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if params.gzipEncoding {
		req.Header.Set("Content-Encoding", "gzip")
	}

	if params.acceptGzip {
		req.Header.Set("Accept-Encoding", "gzip")
	}

	return req, nil
}

// runGQLRequest runs a HTTP GraphQL request and returns the data or any errors.
func runGQLRequest(req *http.Request) ([]byte, error) {
	client := &http.Client{Timeout: 5 * time.Second}
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

func requireContainsRequestID(t *testing.T, resp *GraphQLResponse) {

	v, ok := resp.Extensions["requestID"]
	require.True(t, ok,
		"GraphQL response didn't contain a request ID - response was:\n%s",
		serializeOrError(resp))

	str, ok := v.(string)
	require.True(t, ok, "GraphQL requestID is not a string - response was:\n%s",
		serializeOrError(resp))

	_, err := uuid.Parse(str)
	require.NoError(t, err, "GraphQL requestID is not a UUID - response was:\n%s",
		serializeOrError(resp))
}

func requireUID(t *testing.T, uid string) {
	_, err := strconv.ParseUint(uid, 0, 64)
	require.NoError(t, err)
}

func requireNoGQLErrors(t *testing.T, resp *GraphQLResponse) {
	require.Nil(t, resp.Errors,
		"required no GraphQL errors, but received :\n%s", serializeOrError(resp.Errors))
}

func serializeOrError(toSerialize interface{}) string {
	byts, err := json.Marshal(toSerialize)
	if err != nil {
		return "unable to serialize because " + err.Error()
	}
	return string(byts)
}

func populateGraphQLData(client *dgo.Dgraph) error {

	// Helps in local dev to not re-add data multiple times.
	countries, err := allCountriesAdded()
	if err != nil {
		return errors.Wrap(err, "couldn't determine if GraphQL data had already been added")
	}
	if len(countries) > 0 {
		return nil
	}

	jsonFile := "e2e_test_data.json"
	byts, err := ioutil.ReadFile(jsonFile)
	if err != nil {
		return errors.Wrapf(err, "Unable to read file %s.", jsonFile)
	}

	mu := &api.Mutation{
		CommitNow: true,
		SetJson:   byts,
	}
	_, err = client.NewTxn().Mutate(context.Background(), mu)
	if err != nil {
		return errors.Wrap(err, "Unable to add GraphQL test data")
	}

	return nil
}

func allCountriesAdded() ([]*country, error) {
	body, err := json.Marshal(&GraphQLParams{Query: `query { queryCountry { name } }`})
	if err != nil {
		return nil, errors.Wrap(err, "unable to build GraphQL query")
	}

	req, err := http.NewRequest("POST", graphqlURL, bytes.NewBuffer(body))
	if err != nil {
		return nil, errors.Wrap(err, "unable to build GraphQL request")
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := runGQLRequest(req)
	if err != nil {
		return nil, errors.Wrap(err, "error running GraphQL query")
	}

	var result struct {
		Data struct {
			QueryCountry []*country
		}
	}
	err = json.Unmarshal(resp, &result)
	if err != nil {
		return nil, errors.Wrap(err, "error trying to unmarshal GraphQL query result")
	}

	return result.Data.QueryCountry, nil
}

func checkGraphQLLayerStarted(url string) error {
	var err error
	retries := 6
	sleep := 10 * time.Second

	// Because of how the test containers are brought up, there's no guarantee
	// that the GraphQL layer is running by now.  So we
	// need to try and connect and potentially retry a few times.
	for retries > 0 {
		retries--

		// In local dev, we might already have an instance Healthy.  In CI,
		// we expect the GraphQL layer to be waiting for a first schema.
		err = checkGraphQLHealth(url, []string{"NoGraphQLSchema", "Healthy"})
		if err == nil {
			return nil
		}
		time.Sleep(sleep)
	}
	return err
}

func checkGraphQLHealth(url string, status []string) error {
	health := &GraphQLParams{
		Query: `query {
			health {
				message
				status
			}
		}`,
	}
	req, err := health.createGQLPost(url)

	resp, err := runGQLRequest(req)
	if err != nil {
		return errors.Wrap(err, "error running GraphQL query")
	}

	var healthResult struct {
		Data struct {
			Health struct {
				Message string
				Status  string
			}
		}
		Errors x.GqlErrorList
	}

	err = json.Unmarshal(resp, &healthResult)
	if err != nil {
		return errors.Wrap(err, "error trying to unmarshal GraphQL query result")
	}

	if len(healthResult.Errors) > 0 {
		return healthResult.Errors
	}

	for _, s := range status {
		if healthResult.Data.Health.Status == s {
			return nil
		}
	}

	return errors.Errorf("GraphQL server was not at right health: found %s",
		healthResult.Data.Health.Status)
}

func addSchema(url string, schema string) error {
	add := &GraphQLParams{
		Query: `mutation addSchema($sch: String!) {
			addSchema(input: { schema: $sch }) {
				schema {
					schema
				}
			}
		}`,
		Variables: map[string]interface{}{"sch": schema},
	}
	req, err := add.createGQLPost(url)
	if err != nil {
		return errors.Wrap(err, "error running GraphQL query")
	}

	resp, err := runGQLRequest(req)
	if err != nil {
		return errors.Wrap(err, "error running GraphQL query")
	}

	var addResult struct {
		Data struct {
			AddSchema struct {
				Schema struct {
					Schema string
				}
			}
		}
	}

	err = json.Unmarshal(resp, &addResult)
	if err != nil {
		return errors.Wrap(err, "error trying to unmarshal GraphQL mutation result")
	}

	if addResult.Data.AddSchema.Schema.Schema == "" {
		return errors.New("GraphQL schema mutation failed")
	}

	return nil
}
