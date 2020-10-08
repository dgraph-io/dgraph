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

package common

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/v200"
	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

const (
	GraphqlURL      = "http://localhost:8180/graphql"
	graphqlAdminURL = "http://localhost:8180/admin"
	AlphagRPC       = "localhost:9180"

	adminDgraphHealthURL           = "http://localhost:8280/health?all"
	adminDgraphStateURL            = "http://localhost:8280/state"
	graphqlAdminTestURL            = "http://localhost:8280/graphql"
	graphqlAdminTestAdminURL       = "http://localhost:8280/admin"
	graphqlAdminTestAdminSchemaURL = "http://localhost:8280/admin/schema"
	alphaAdminTestgRPC             = "localhost:9280"
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
	Headers       http.Header
}

type requestExecutor func(t *testing.T, url string, params *GraphQLParams) *GraphQLResponse

// GraphQLResponse GraphQL response structure.
// see https://graphql.github.io/graphql-spec/June2018/#sec-Response
type GraphQLResponse struct {
	Data       json.RawMessage        `json:"data,omitempty"`
	Errors     x.GqlErrorList         `json:"errors,omitempty"`
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

type Tweets struct {
	Id        string `json:"id,omitempty"`
	Text      string `json:"text,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
	User      *User  `json:"user,omitempty"`
}

type User struct {
	Username string `json:"username,omitempty"`
	Age      uint64 `json:"age,omitempty"`
	IsPublic bool   `json:"isPublic,omitempty"`
	Disabled bool   `json:"disabled,omitempty"`
}

type country struct {
	ID     string   `json:"id,omitempty"`
	Name   string   `json:"name,omitempty"`
	States []*state `json:"states,omitempty"`
}

type author struct {
	ID         string     `json:"id,omitempty"`
	Name       string     `json:"name,omitempty"`
	Dob        *time.Time `json:"dob,omitempty"`
	Reputation float32    `json:"reputation,omitempty"`
	Country    *country   `json:"country,omitempty"`
	Posts      []*post    `json:"posts,omitempty"`
}

type user struct {
	Name     string `json:"name,omitempty"`
	Password string `json:"password,omitempty"`
}

type post struct {
	PostID      string    `json:"postID,omitempty"`
	Title       string    `json:"title,omitempty"`
	Text        string    `json:"text,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	NumLikes    int       `json:"numLikes,omitempty"`
	NumViews    int64     `json:"numViews,omitempty"`
	IsPublished bool      `json:"isPublished,omitempty"`
	PostType    string    `json:"postType,omitempty"`
	Author      *author   `json:"author,omitempty"`
	Category    *category `json:"category,omitempty"`
}

type category struct {
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Posts []post `json:"posts,omitempty"`
}

type state struct {
	ID      string   `json:"id,omitempty"`
	Name    string   `json:"name,omitempty"`
	Code    string   `json:"xcode,omitempty"`
	Capital string   `json:"capital,omitempty"`
	Country *country `json:"country,omitempty"`
}

type movie struct {
	ID       string      `json:"id,omitempty"`
	Name     string      `json:"name,omitempty"`
	Director []*director `json:"moviedirector,omitempty"`
}

type director struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

type teacher struct {
	ID      string     `json:"id,omitempty"`
	Xid     string     `json:"xid,omitempty"`
	Name    string     `json:"name,omitempty"`
	Subject string     `json:"subject,omitempty"`
	Teaches []*student `json:"teaches,omitempty"`
}

type student struct {
	ID       string     `json:"id,omitempty"`
	Xid      string     `json:"xid,omitempty"`
	Name     string     `json:"name,omitempty"`
	TaughtBy []*teacher `json:"taughtBy,omitempty"`
}

type UserSecret struct {
	Id      string `json:"id,omitempty"`
	ASecret string `json:"aSecret,omitempty"`
	OwnedBy string `json:"ownedBy,omitempty"`
}

func (twt *Tweets) DeleteByID(t *testing.T, user string, metaInfo *testutil.AuthMeta) {
	getParams := &GraphQLParams{
		Headers: GetJWT(t, user, "", metaInfo),
		Query: `
			mutation delTweets ($filter : TweetsFilter!){
			  	deleteTweets (filter: $filter) {
					numUids
			  	}
			}
		`,
		Variables: map[string]interface{}{"filter": map[string]interface{}{
			"id": map[string]interface{}{"eq": twt.Id},
		}},
	}
	gqlResponse := getParams.ExecuteAsPost(t, GraphqlURL)
	require.Nil(t, gqlResponse.Errors)
}

func (us *UserSecret) Delete(t *testing.T, user, role string, metaInfo *testutil.AuthMeta) {
	getParams := &GraphQLParams{
		Headers: GetJWT(t, user, role, metaInfo),
		Query: `
			mutation deleteUserSecret($ids: [ID!]) {
				deleteUserSecret(filter:{id:$ids}) {
					msg
				}
			}
		`,
		Variables: map[string]interface{}{"ids": []string{us.Id}},
	}
	gqlResponse := getParams.ExecuteAsPost(t, GraphqlURL)
	require.Nil(t, gqlResponse.Errors)
}

func BootstrapServer(schema, data []byte) {
	err := checkGraphQLStarted(graphqlAdminURL)
	if err != nil {
		x.Panic(errors.Errorf(
			"Waited for GraphQL test server to become available, but it never did.\n"+
				"Got last error %+v", err.Error()))
	}

	err = checkGraphQLStarted(graphqlAdminTestAdminURL)
	if err != nil {
		x.Panic(errors.Errorf(
			"Waited for GraphQL AdminTest server to become available, "+
				"but it never did.\n Got last error: %+v", err.Error()))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	d, err := grpc.DialContext(ctx, AlphagRPC, grpc.WithInsecure())
	if err != nil {
		x.Panic(err)
	}
	client := dgo.NewDgraphClient(api.NewDgraphClient(d))

	err = addSchema(graphqlAdminURL, string(schema))
	if err != nil {
		x.Panic(err)
	}

	err = maybePopulateData(client, data)
	if err != nil {
		x.Panic(err)
	}
	if err = d.Close(); err != nil {
		x.Panic(err)
	}
}

// RunAll runs all the test functions in this package as sub tests.
func RunAll(t *testing.T) {
	// admin tests
	t.Run("admin", admin)
	t.Run("health", health)
	t.Run("partial health", partialHealth)
	t.Run("alias should work in admin", adminAlias)
	t.Run("state", adminState)
	t.Run("propagate client remote ip", clientInfoLogin)

	// schema tests
	t.Run("graphql descriptions", graphQLDescriptions)

	// header tests
	t.Run("touched uids header", touchedUidsHeader)

	// encoding
	t.Run("gzip compression", gzipCompression)
	t.Run("gzip compression header", gzipCompressionHeader)
	t.Run("gzip compression no header", gzipCompressionNoHeader)

	// query tests
	t.Run("get request", getRequest)
	t.Run("get query empty variable", getQueryEmptyVariable)
	t.Run("post request with application/graphql", queryApplicationGraphQl)
	t.Run("query by type", queryByType)
	t.Run("uid alias", uidAlias)
	t.Run("order at root", orderAtRoot)
	t.Run("page at root", pageAtRoot)
	t.Run("regexp", regExp)
	t.Run("multiple search indexes", multipleSearchIndexes)
	t.Run("multiple search indexes wrong field", multipleSearchIndexesWrongField)
	t.Run("hash search", hashSearch)
	t.Run("in filter", inFilter)
	t.Run("deep filter", deepFilter)
	t.Run("deep has filter", deepHasFilter)
	t.Run("many queries", manyQueries)
	t.Run("query order at root", queryOrderAtRoot)
	t.Run("queries with error", queriesWithError)
	t.Run("date filters", dateFilters)
	t.Run("float filters", floatFilters)
	t.Run("has filters", hasFilters)
	t.Run("Int filters", int32Filters)
	t.Run("Int64 filters", int64Filters)
	t.Run("boolean filters", booleanFilters)
	t.Run("term filters", termFilters)
	t.Run("full text filters", fullTextFilters)
	t.Run("string exact filters", stringExactFilters)
	t.Run("scalar list filters", scalarListFilters)
	t.Run("skip directive", skipDirective)
	t.Run("include directive", includeDirective)
	t.Run("include and skip directive", includeAndSkipDirective)
	t.Run("query by mutliple ids", queryByMultipleIds)
	t.Run("enum filter", enumFilter)
	t.Run("default enum filter", defaultEnumFilter)
	t.Run("query by multiple invalid ids", queryByMultipleInvalidIds)
	t.Run("query typename", queryTypename)
	t.Run("query nested typename", queryNestedTypename)
	t.Run("typename for interface", typenameForInterface)
	t.Run("query only typename", queryOnlyTypename)
	t.Run("query nested only typename", querynestedOnlyTypename)
	t.Run("test onlytypename for interface types", onlytypenameForInterface)

	t.Run("get state by xid", getStateByXid)
	t.Run("get state without args", getStateWithoutArgs)
	t.Run("get state by both xid and uid", getStateByBothXidAndUid)
	t.Run("query state by xid", queryStateByXid)
	t.Run("query state by xid regex", queryStateByXidRegex)
	t.Run("multiple operations", multipleOperations)
	t.Run("query post with author", queryPostWithAuthor)
	t.Run("queries have extensions", queriesHaveExtensions)
	t.Run("alias works for queries", queryWithAlias)
	t.Run("cascade directive", queryWithCascade)
	t.Run("query geo near filter", queryGeoNearFilter)

	// mutation tests
	t.Run("add mutation", addMutation)
	t.Run("update mutation by ids", updateMutationByIds)
	t.Run("update mutation by name", updateMutationByName)
	t.Run("update mutation by name no match", updateMutationByNameNoMatch)
	t.Run("update delete", updateRemove)
	t.Run("filter in update", filterInUpdate)
	t.Run("selection in add object", testSelectionInAddObject)
	t.Run("delete mutation with multiple ids", deleteMutationWithMultipleIds)
	t.Run("delete mutation with single id", deleteMutationWithSingleID)
	t.Run("delete mutation by name", deleteMutationByName)
	t.Run("delete mutation removes references", deleteMutationReferences)
	t.Run("add mutation updates references", addMutationReferences)
	t.Run("update set mutation updates references", updateMutationReferences)
	t.Run("delete wrong id", deleteWrongID)
	t.Run("many mutations", manyMutations)
	t.Run("mutations with deep filter", mutationWithDeepFilter)
	t.Run("many mutations with query error", manyMutationsWithQueryError)
	t.Run("query interface after add mutation", queryInterfaceAfterAddMutation)
	t.Run("add mutation with xid", addMutationWithXID)
	t.Run("deep mutations", deepMutations)
	t.Run("add multiple mutations", testMultipleMutations)
	t.Run("deep XID mutations", deepXIDMutations)
	t.Run("three level xid", testThreeLevelXID)
	t.Run("nested add mutation with @hasInverse", nestedAddMutationWithHasInverse)
	t.Run("add mutation with @hasInverse overrides correctly", addMutationWithHasInverseOverridesCorrectly)
	t.Run("error in multiple mutations", addMultipleMutationWithOneError)
	t.Run("dgraph directive with reverse edge adds data correctly",
		addMutationWithReverseDgraphEdge)
	t.Run("numUids test", testNumUids)
	t.Run("empty delete", mutationEmptyDelete)
	t.Run("password in mutation", passwordTest)
	t.Run("duplicate xid in single mutation", deepMutationDuplicateXIDsSameObjectTest)
	t.Run("query typename in mutation payload", queryTypenameInMutationPayload)
	t.Run("ensure alias in mutation payload", ensureAliasInMutationPayload)
	t.Run("mutations have extensions", mutationsHaveExtensions)
	t.Run("alias works for mutations", mutationsWithAlias)
	t.Run("three level deep", threeLevelDeepMutation)
	t.Run("update mutation without set & remove", updateMutationWithoutSetRemove)
	t.Run("Input coercing for int64 type", int64BoundaryTesting)
	t.Run("Check cascade with mutation without ID field", checkCascadeWithMutationWithoutIDField)
	t.Run("Geo type", mutationGeoType)

	// error tests
	t.Run("graphql completion on", graphQLCompletionOn)
	t.Run("request validation errors", requestValidationErrors)
	t.Run("panic catcher", panicCatcher)
	t.Run("deep mutation errors", deepMutationErrors)

	// fragment tests
	t.Run("fragment in mutation", fragmentInMutation)
	t.Run("fragment in query", fragmentInQuery)
	t.Run("fragment in query on Interface", fragmentInQueryOnInterface)
	t.Run("fragment in query on Object", fragmentInQueryOnObject)

	// lambda tests
	t.Run("lambda on type field", lambdaOnTypeField)
	t.Run("lambda on interface field", lambdaOnInterfaceField)
	t.Run("lambda on query using dql", lambdaOnQueryUsingDql)
	t.Run("lambda on mutation using graphql", lambdaOnMutationUsingGraphQL)
}

// RunCorsTest test all cors related tests.
func RunCorsTest(t *testing.T) {
	testCors(t)
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
func gzipCompressionHeader(t *testing.T) {
	queryCountry := &GraphQLParams{
		Query: `query {
			queryCountry {
				name
			}
		}`,
	}

	req, err := queryCountry.CreateGQLPost(GraphqlURL)
	require.NoError(t, err)

	req.Header.Set("Content-Encoding", "gzip")

	resData, err := RunGQLRequest(req)
	require.NoError(t, err)

	var result *GraphQLResponse
	err = json.Unmarshal(resData, &result)
	require.NoError(t, err)
	require.NotNil(t, result.Errors)
	require.Contains(t, result.Errors[0].Message, "Unable to parse gzip")
}

// This tests that if a req's body is compressed but the
// header is not present, then it should return an error
func gzipCompressionNoHeader(t *testing.T) {
	queryCountry := &GraphQLParams{
		Query: `query {
			queryCountry {
				name
			}
		}`,
		gzipEncoding: true,
	}

	req, err := queryCountry.CreateGQLPost(GraphqlURL)
	require.NoError(t, err)

	req.Header.Del("Content-Encoding")
	resData, err := RunGQLRequest(req)
	require.NoError(t, err)

	var result *GraphQLResponse
	err = json.Unmarshal(resData, &result)
	require.NoError(t, err)
	require.NotNil(t, result.Errors)
	require.Contains(t, result.Errors[0].Message, "Not a valid GraphQL request body")
}

func getRequest(t *testing.T) {
	add(t, getExecutor)
}

func getQueryEmptyVariable(t *testing.T) {
	queryCountry := &GraphQLParams{
		Query: `query {
			queryCountry {
				name
			}
		}`,
	}
	req, err := queryCountry.createGQLGet(GraphqlURL)
	require.NoError(t, err)

	q := req.URL.Query()
	q.Del("variables")
	req.URL.RawQuery = q.Encode()

	res := queryCountry.Execute(t, req)
	require.Nil(t, res.Errors)
}

// Execute takes a HTTP request from either ExecuteAsPost or ExecuteAsGet
// and executes the request
func (params *GraphQLParams) Execute(t require.TestingT, req *http.Request) *GraphQLResponse {
	for h := range params.Headers {
		req.Header.Set(h, params.Headers.Get(h))
	}
	res, err := RunGQLRequest(req)
	require.NoError(t, err)

	var result *GraphQLResponse
	if params.acceptGzip {
		res, err = gunzipData(res)
		require.NoError(t, err)
		require.Contains(t, req.Header.Get("Accept-Encoding"), "gzip")
	}
	err = json.Unmarshal(res, &result)
	require.NoError(t, err)

	return result
}

// ExecuteAsPost builds a HTTP POST request from the GraphQL input structure
// and executes the request to url.
func (params *GraphQLParams) ExecuteAsPost(t require.TestingT, url string) *GraphQLResponse {
	req, err := params.CreateGQLPost(url)
	require.NoError(t, err)

	return params.Execute(t, req)
}

// ExecuteAsPostApplicationGraphql builds an HTTP Post with type application/graphql
// Note, variables are not allowed
func (params *GraphQLParams) ExecuteAsPostApplicationGraphql(t *testing.T, url string) *GraphQLResponse {
	require.Empty(t, params.Variables)

	req, err := params.createApplicationGQLPost(url)
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

func (params *GraphQLParams) buildPostRequest(url string, body []byte, contentType string) (*http.Request, error) {
	var err error
	if params.gzipEncoding {
		if body, err = gzipData(body); err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	if params.gzipEncoding {
		req.Header.Set("Content-Encoding", "gzip")
	}

	if params.acceptGzip {
		req.Header.Set("Accept-Encoding", "gzip")
	}

	return req, nil
}

func (params *GraphQLParams) CreateGQLPost(url string) (*http.Request, error) {
	body, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	return params.buildPostRequest(url, body, "application/json")
}

func (params *GraphQLParams) createApplicationGQLPost(url string) (*http.Request, error) {
	return params.buildPostRequest(url, []byte(params.Query), "application/graphql")
}

// RunGQLRequest runs a HTTP GraphQL request and returns the data or any errors.
func RunGQLRequest(req *http.Request) ([]byte, error) {
	client := &http.Client{Timeout: 50 * time.Second}
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

func requireUID(t *testing.T, uid string) {
	_, err := strconv.ParseUint(uid, 0, 64)
	require.NoError(t, err)
}

func RequireNoGQLErrors(t *testing.T, resp *GraphQLResponse) {
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

func PopulateGraphQLData(client *dgo.Dgraph, data []byte) error {
	mu := &api.Mutation{
		CommitNow: true,
		SetJson:   data,
	}
	_, err := client.NewTxn().Mutate(context.Background(), mu)
	if err != nil {
		return errors.Wrap(err, "Unable to add GraphQL test data")
	}
	return nil
}

func maybePopulateData(client *dgo.Dgraph, data []byte) error {
	if data == nil {
		return nil
	}
	// Helps in local dev to not re-add data multiple times.
	countries, err := allCountriesAdded()
	if err != nil {
		return errors.Wrap(err, "couldn't determine if GraphQL data had already been added")
	}
	if len(countries) > 0 {
		return nil
	}
	return PopulateGraphQLData(client, data)
}

func allCountriesAdded() ([]*country, error) {
	body, err := json.Marshal(&GraphQLParams{Query: `query { queryCountry { name } }`})
	if err != nil {
		return nil, errors.Wrap(err, "unable to build GraphQL query")
	}

	req, err := http.NewRequest("POST", GraphqlURL, bytes.NewBuffer(body))
	if err != nil {
		return nil, errors.Wrap(err, "unable to build GraphQL request")
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := RunGQLRequest(req)
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

func checkGraphQLStarted(url string) error {
	var err error
	retries := 6
	sleep := 10 * time.Second

	// Because of how GraphQL starts (it needs to read the schema from Dgraph),
	// there's no guarantee that GraphQL is available by now.  So we
	// need to try and connect and potentially retry a few times.
	for retries > 0 {
		retries--

		_, err = hasCurrentGraphQLSchema(url)
		if err == nil {
			return nil
		}
		time.Sleep(sleep)
	}
	return err
}

func hasCurrentGraphQLSchema(url string) (bool, error) {

	schemaQry := &GraphQLParams{
		Query: `query { getGQLSchema { schema } }`,
	}
	req, err := schemaQry.CreateGQLPost(url)
	if err != nil {
		return false, errors.Wrap(err, "while creating gql post")
	}

	res, err := RunGQLRequest(req)
	if err != nil {
		return false, errors.Wrap(err, "error running GraphQL query")
	}

	var result *GraphQLResponse
	err = json.Unmarshal(res, &result)
	if err != nil {
		return false, errors.Wrap(err, "error unmarshalling result")
	}

	if len(result.Errors) > 0 {
		return false, result.Errors
	}

	var sch struct {
		GetGQLSchema struct {
			Schema string
		}
	}

	err = json.Unmarshal(result.Data, &sch)
	if err != nil {
		return false, errors.Wrap(err, "error trying to unmarshal GraphQL query result")
	}

	if sch.GetGQLSchema.Schema == "" {
		return false, nil
	}

	return true, nil
}

func addSchema(url, schema string) error {
	add := &GraphQLParams{
		Query: `mutation updateGQLSchema($sch: String!) {
			updateGQLSchema(input: { set: { schema: $sch }}) {
				gqlSchema {
					schema
				}
			}
		}`,
		Variables: map[string]interface{}{"sch": schema},
	}
	req, err := add.CreateGQLPost(url)
	if err != nil {
		return errors.Wrap(err, "error creating GraphQL query")
	}

	resp, err := RunGQLRequest(req)
	if err != nil {
		return errors.Wrap(err, "error running GraphQL query")
	}

	var addResult struct {
		Data struct {
			UpdateGQLSchema struct {
				GQLSchema struct {
					Schema string
				}
			}
		}
		Errors []interface{}
	}

	err = json.Unmarshal(resp, &addResult)
	if err != nil {
		return errors.Wrap(err, "error trying to unmarshal GraphQL mutation result")
	}

	if len(addResult.Errors) > 0 {
		return errors.Errorf("%v", addResult.Errors)
	}

	if addResult.Data.UpdateGQLSchema.GQLSchema.Schema == "" {
		return errors.New("GraphQL schema mutation failed")
	}

	return nil
}

func addSchemaThroughAdminSchemaEndpt(url, schema string) error {
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(schema))
	if err != nil {
		return errors.Wrap(err, "error running GraphQL query")
	}

	resp, err := RunGQLRequest(req)
	if err != nil {
		return errors.Wrap(err, "error running GraphQL query")
	}

	var addResult struct {
		Data struct {
			Code    string
			Message string
		}
	}

	err = json.Unmarshal(resp, &addResult)
	if err != nil {
		return errors.Wrap(err, "error trying to unmarshal GraphQL mutation result")
	}

	if addResult.Data.Code != "Success" && addResult.Data.Message != "Done" {
		return errors.New("GraphQL schema mutation failed")
	}

	return nil
}

func GetJWT(t *testing.T, user, role string, metaInfo *testutil.AuthMeta) http.Header {
	metaInfo.AuthVars = map[string]interface{}{}
	if user != "" {
		metaInfo.AuthVars["USER"] = user
	}

	if role != "" {
		metaInfo.AuthVars["ROLE"] = role
	}

	require.NotNil(t, metaInfo.PrivateKeyPath)
	jwtToken, err := metaInfo.GetSignedToken(metaInfo.PrivateKeyPath, 300*time.Second)
	require.NoError(t, err)

	h := make(http.Header)
	h.Add(metaInfo.Header, jwtToken)
	return h
}
