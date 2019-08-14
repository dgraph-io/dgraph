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
	//"fmt"
	"io/ioutil"
	//"log"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

/*
The docker-compose config  in dgraph/docker-compose.yml intializes and runs the GraphQL server.

Check the compose file to find the graphql schema file used to initialize the server.
*/

// Parameters for the constructing http post body of a graphql query.
type graphqlParams struct {
	Query     string                       `json:"query"`
	Variables map[string]map[string]string `json:"variables"`
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
	Messages []message `json:"message"`
}

type data struct {
	AddMessage addMessage `json:"addMessage"`
}

// Data structure respresenting the json response for the auto generated mutation type.
type MessageMutation struct {
	Data data `json:"data"`
}

var graphqlURL = "http://localhost:9000/graphql"

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
	`mutation additionMessages($message: MessageInput!) { 
		addMessage(input: $message) {
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

// Runs mutations derived from the test schema.
// Verifies the response.
// The Graphql layer is initialized with the test schema in the docker-compose file.
func TestGraphQLMutation(t *testing.T) {

	type testData struct {
		// Simple name for the test case.
		// For example [Simple mutations 1, Simple mutations 2, wrong mutation body]\
		name string
		// POST data for the GraphQL request.
		graphqlReq graphqlParams
		// GraphQL mutation or Query
		queryType graphqlQueryType
		// Short Description of the test case.
		explain string
		// expected http response status code.
		wantCode int
		// expected Result.
		wantResult MessageMutation
	}

	// Expected data from the mutations.
	// Will be used to validate the mutation responses.
	expectedData := []MessageMutation{
		// Expected result 1.
		MessageMutation{
			Data: data{
				AddMessage: addMessage{
					Messages: []message{
						message{
							Content:    "Unit test post",
							Author:     "Karthic Rao",
							DatePosted: "2019-07-07T00:00:00Z",
						},
					},
				},
			},
		},
		// Expected result 2.
		MessageMutation{
			Data: data{
				AddMessage: addMessage{
					Messages: []message{
						message{
							Content:    "Unit test second post",
							Author:     "Michael Compton",
							DatePosted: "2019-08-08T05:04:33Z",
						},
					},
				},
			},
		},
		// Expected result 3.
		MessageMutation{
			Data: data{
				AddMessage: addMessage{
					Messages: []message{
						message{
							Content:    "Invalid mutation",
							Author:     "Prashant Shahi",
							DatePosted: "2019-08-08T05:04:33Z",
						},
					},
				},
			},
		},
	}

	// The following image shows an example HTTP POST request body for a graphql query/mutation.
	// https://user-images.githubusercontent.com/2609511/62793240-b731fc80-baee-11e9-915f-2c19f0396229.png
	// In the testData we will construct different Graphql queries and mutations.
	tests := []testData{
		// Test case 1
		{
			name: "Simple mutation 1",
			graphqlReq: graphqlParams{
				Query: toSingleLine(queries[0]),
				Variables: map[string]map[string]string{
					"message": map[string]string{
						"content":    "Unit test post",
						"author":     "Karthic Rao",
						"datePosted": "2019-07-07",
					},
				},
			},
			queryType:  QUERY,
			explain:    "Send Mutation to the GraphQL server and verify response data",
			wantCode:   200,
			wantResult: expectedData[0],
		},
		// Test case 2.
		{

			name: "Simple mutation 2",
			graphqlReq: graphqlParams{
				Query: toSingleLine(queries[0]),
				Variables: map[string]map[string]string{
					"message": map[string]string{
						"content":    "Unit test second post",
						"author":     "Michael Compton",
						"datePosted": "2019-08-08T05:04:33Z",
					},
				},
			},
			queryType: QUERY,
			explain: "Add data with Mutation and verify return data." +
				" Time is set to 2019-08-08T00:00:00Z format",
			wantCode:   200,
			wantResult: expectedData[1],
		},
		// Test case 3.
		{

			name: "Invalid Mutation function name",
			graphqlReq: graphqlParams{
				Query: toSingleLine(queries[1]),
				Variables: map[string]map[string]string{
					"message": map[string]string{
						"content":    "Invalid mutation",
						"author":     "Prashant Shahi",
						"datePosted": "2019-08-08T05:04:33Z",
					},
				},
			},
			queryType: QUERY,
			explain: "The mutation name is invalid. Will be checking for " +
				"valid error response when the mutation function doesn't of given name doesn't exist",
			wantCode:   200,
			wantResult: expectedData[2],
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

			bodyBytes, err := ioutil.ReadAll(resp.Body)

			require.NoError(t, err)

			//log.Println(string(bodyBytes))

			var mutationResponse MessageMutation
			err = json.Unmarshal(bodyBytes, &mutationResponse)

			require.NoError(t, err)
			//fmt.Printf("%+v", mutationResponse)

			// Verify the response status code.
			require.Equal(t, test.wantCode, resp.StatusCode)
			// Verify that the response data exist.
			require.Equal(t, len(mutationResponse.Data.AddMessage.Messages), 1)
			// Verify that the GraphQL mutation has returned UID of the node from Dgraph.
			require.NotEmpty(t, mutationResponse.Data.AddMessage.Messages[0].ID)

			// Since the node ID is generated after the mutation,
			// We cannot have it before hand in the expected data.
			// We fetch it from the response json of the mutation.
			test.wantResult.Data.AddMessage.Messages[0].ID = mutationResponse.Data.AddMessage.Messages[0].ID

			// Match and verify the mutation response from the GraphQL server.
			require.Equal(t, test.wantResult, mutationResponse)
		})
	}
}
