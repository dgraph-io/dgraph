/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
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
	"strings"
	"testing"

	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
)

/*
The docker-compose config  in dgraph/docker-compose.yml intializes and runs
the GraphQL server.

Check the compose file to find the graphql schema file used to initialize
the server.
*/

// Parameters for the constructing http post body of a graphql query.
type GraphqlParams struct {
	Query     string                       `json:"query"`
	Variables map[string]map[string]string `json:"variables"`
}

// GraphQL sever response structure.
type GraphqlResponse struct {
	Data       json.RawMessage  `json:"data"`
	Extensions query.Extensions `json:"extensions,omitempty"`
	Errors     []x.GqlError     `json:"errors,omitempty"`
}

// GraphQL types.
// Data structures below are used to match the response from the GraphQL server.
// These structs match the types present and generated in the GraphQL schema
// which is used to initialize the GraphQL server.

// Type corresponding to the Message type from the GraphQL schema.
// check dgraph/docker-compose.yml to find the schema file used for tests.
type message struct {
	ID         string `json:"id"`
	Content    string `json:"content"`
	Author     string `json:"author"`
	DatePosted string `json:"datePosted"`
}

type addMessage struct {
	Message message `json:"message"`
}

// Data structure respresenting the json response for the auto generated mutation type.
type MessageMutation struct {
	AddMessage addMessage `json:"addMessage"`
}

const graphqlURL = "http://localhost:9000/graphql"
const dgraphAddr = "http://localhost:8180"

type graphqlQueryType int32

// A Graphql query is represented by 0.
// A GraphQL mutation is represented by 1.
const (
	QUERY    graphqlQueryType = 0
	MUTATION graphqlQueryType = 1
)

// Graphql queries/mutations to be used in test cases.
var queries = []string{
	// Query 1.
	// Valid Graphql mutation for the test schema.
	`mutation addMessage($message: MessageInput!) { 
		addMessage(input: $message) {
			message {
				id 
				content 
				author 
				datePosted
			}
		}
	}`,
	// Query 2.
	// Invalid Graphql mutation for the test schema.
	// for type Message in the GraphQL schema,
	// addMessage is one of the valid mutations.
	`mutation additionMessages($message: MessageInput!) { 
		addMessagessss(input: $message) {
			message {
				id 
				content 
				author 
				datePosted
			}
		}
	}`,
}

// Converts a multiline string to single line.
// The GraphQL server expects the mutation/query to be a single line string.
func toSingleLine(graphqlBody string) string {
	graphqlBody = strings.Replace(graphqlBody, "\n", "", -1)
	graphqlBody = strings.Replace(graphqlBody, "\t\t", "", -1)
	graphqlBody = strings.Replace(graphqlBody, "  ", "", -1)

	return graphqlBody
}

// Types to parse the fetch schema query on Dgraph.
type DgraphTypeField struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type DgraphType struct {
	Fields []DgraphTypeField `json:"fields"`
	Name   string            `json:"name"`
}

type SchemaData struct {
	Types []DgraphType `json:"types"`
}

type SchemaResponse struct {
	Data SchemaData `json:"data"`
}

// The Graphql layer autogenerates Dgraph DB schema
// corresponding to the GraphQL schema that is used to initialize the layer.
// This test verifies and validates the Dgraph schema with the expected value.
func TestDgraphSchema(t *testing.T) {
	// Expected Dgraph types.
	want := SchemaData{
		Types: []DgraphType{
			DgraphType{
				Fields: []DgraphTypeField{
					DgraphTypeField{
						Name: "Message.content",
						Type: "string",
					},
					DgraphTypeField{
						Name: "Message.author",
						Type: "string",
					},
					DgraphTypeField{
						Name: "Message.datePosted",
						Type: "datetime",
					},
				},
				Name: "Message",
			},
		},
	}

	// Query to fetch the database schema.
	dgraphQuery := "schema {}"
	contentType := "application/graphql+-"
	url := dgraphAddr + "/query"
	// Send the request to the GraphQL server running at "graphqlURL".
	req, err := http.NewRequest("POST", url, bytes.NewBufferString(dgraphQuery))
	require.NoError(t, err)

	req.Header.Set("Content-Type", contentType)
	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err)

	// read the response body.
	bodyBytes, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	// Parse the response from the GraphQL server.
	var schemaData SchemaResponse
	err = json.Unmarshal(bodyBytes, &schemaData)
	require.NoError(t, err)
	// Verify the response status code.
	require.Equal(t, want, schemaData.Data)
}

// Runs mutations derived from the test schema.
// Verifies the response.
// Any changes to the response format for error and
// non-error cases for addMutations should be caught by this test.
func TestGraphQLMutation(t *testing.T) {

	type testData struct {
		// Simple name for the test case.
		// For example [Simple mutations 1, Simple mutations 2, wrong mutation body.
		name string
		// POST data for the GraphQL request.
		graphqlReq GraphqlParams
		// GraphQL mutation or Query
		queryType graphqlQueryType
		// Short Description of the test case.
		explain string
		// expected http response status code.
		wantCode int
		// expected Result.
		wantResult MessageMutation
		// is an error response expected.
		isErr bool
		// expected error messasge.
		wantError []x.GqlError
	}

	// The following image shows an example HTTP POST request body for a graphql query/mutation.
	// https://user-images.githubusercontent.com/2609511/62793240-b731fc80-baee-11e9-915f-2c19f0396229.png
	// In the testData we will construct different Graphql queries and mutations.
	tests := []testData{
		// Test case 1
		{
			name: "Simple mutation 1",
			graphqlReq: GraphqlParams{
				Query: toSingleLine(queries[0]),
				Variables: map[string]map[string]string{
					"message": map[string]string{
						"content":    "Unit test post",
						"author":     "Karthic Rao",
						"datePosted": "2019-07-07",
					},
				},
			},
			queryType: MUTATION,
			explain:   "Send Mutation to the GraphQL server and verify response data",
			wantCode:  200,
			wantResult: MessageMutation{
				AddMessage: addMessage{
					Message: message{
						Content:    "Unit test post",
						Author:     "Karthic Rao",
						DatePosted: "2019-07-07T00:00:00Z",
					},
				},
			},
		},
		// Test case 2.
		{
			name: "Simple mutation 2",
			graphqlReq: GraphqlParams{
				Query: toSingleLine(queries[0]),
				Variables: map[string]map[string]string{
					"message": map[string]string{
						"content":    "Unit test second post",
						"author":     "Michael Compton",
						"datePosted": "2019-08-08T05:04:33Z",
					},
				},
			},
			queryType: MUTATION,
			explain: "Add data with Mutation and verify return data." +
				" Time is set to 2019-08-08T05:04:33Z format",
			wantCode: 200,
			wantResult: MessageMutation{
				AddMessage: addMessage{
					Message: message{
						Content:    "Unit test second post",
						Author:     "Michael Compton",
						DatePosted: "2019-08-08T05:04:33Z",
					},
				},
			},
		},
		// Test case 3.
		{
			name: "Invalid Mutation name",
			graphqlReq: GraphqlParams{
				Query: toSingleLine(queries[1]),
				Variables: map[string]map[string]string{
					"message": map[string]string{
						"content":    "Invalid mutation",
						"author":     "Prashant Shahi",
						"datePosted": "2019-08-08T05:04:33Z",
					},
				},
			},
			queryType: MUTATION,
			explain: "The mutation name is invalid. Will be checking for " +
				"valid error response when the mutation function doesn't of given name doesn't exist",
			wantCode: 200,
			isErr:    true,
			wantError: []x.GqlError{
				{
					Message: "Cannot query field \"addMessagessss\" on type \"Mutation\". " +
						"Did you mean \"addMessage\"?",
					Locations: []x.Location{
						{
							Line:   1,
							Column: 54,
						},
					},
				},
			},
		},
	}

	// Iterate through each test case and validate the response.
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Construct the post request body for the GraphQL request.
			d2, err := json.Marshal(test.graphqlReq)
			require.NoError(t, err)

			buf := bytes.NewBuffer(d2)
			// Send the request to the GraphQL server running at "graphqlURL".
			req, err := http.NewRequest("POST", graphqlURL, buf)
			require.NoError(t, err)

			req.Header.Set("Content-Type", "application/json")
			client := &http.Client{}
			resp, err := client.Do(req)
			require.NoError(t, err)

			// read the response body.
			bodyBytes, err := ioutil.ReadAll(resp.Body)
			require.NoError(t, err)
			// Parse the response from the GraphQL server.
			var graphqlResponse GraphqlResponse
			err = json.Unmarshal(bodyBytes, &graphqlResponse)
			require.NoError(t, err)

			// Verify the response status code.
			require.Equal(t, test.wantCode, resp.StatusCode)

			// Validate the error response for cases
			// where error is expected.
			// Explicitly adding if statement for readability.
			if test.isErr {
				require.Equal(t, 1, len(graphqlResponse.Errors), "Expected error response"+
					" differs from the actual one")
				require.Equal(t, test.wantError, graphqlResponse.Errors)
				// If the test case is to validate the error input,
				// return after the error validation.
				return
			}

			// validation of output for cases where
			// successful mutation is expected.
			var mutationResponse MessageMutation
			err = json.Unmarshal(graphqlResponse.Data, &mutationResponse)
			require.NoError(t, err)
			// Verify that the response data exist and
			// the GraphQL mutation has returned UID of the node from Dgraph.
			require.NotEmpty(t, mutationResponse.AddMessage.Message.ID)

			// Since the node ID is generated after the mutation,
			// we cannot have it before hand in the expected data.
			// We fetch it from the response json of the mutation.
			test.wantResult.AddMessage.Message.ID =
				mutationResponse.AddMessage.Message.ID
			// Match and verify the mutation response from the GraphQL server.
			require.Equal(t, test.wantResult, mutationResponse, "Mutation response"+
				" differs from the expected one.")
		})
	}
}
