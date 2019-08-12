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
	"net/http"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

/*
The docker-compose config  in dgraph/docker-compose.yml runs the GrapQL from container.

Check the compose file to find the graphql schema file used to initialize the server.

These tests follow table driven format.

Check table driven test examples from https://dave.cheney.net/2019/05/07/prefer-table-driven-tests
*/

type params struct {
	Query     string                       `json:"query"`
	Variables map[string]map[string]string `json:"variables"`
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
func TestGraphQLMutation(t *testing.T) {

	type testData struct {
		// POST data for the GraphQL request.
		graphqlReq params
		// GraphQL mutation or Query
		queryType graphqlQueryType
		// Short Description of the test case.
		explain string
		// expected http response status code.
		wantCode int
	}

	// The following image shows an example HTTP POST request body for a graphql query/mutation.
	// https://user-images.githubusercontent.com/2609511/62793240-b731fc80-baee-11e9-915f-2c19f0396229.png
	// In the testData we will construct different Graphql queries and mutations.
	tests := []testData{
		{
			graphqlReq: params{
				Query: toSingleLine(queries[0]),
				Variables: map[string]map[string]string{
					"message": map[string]string{
						"content":    "Unit test post",
						"author":     "Karthic Rao",
						"datePosted": "2019-07-07",
					},
				},
			},
			queryType: QUERY,
			explain:   "Add data with Mutation and verify return data",
			wantCode:  200,

		},
	}

	for _, test := range tests {
		t.Run(test.explain, func(t *testing.T) {
			d2, err := json.Marshal(test.graphqlReq)

			require.NoError(t, err)

			buf := bytes.NewBuffer(d2)
			req, err := http.NewRequest("POST", graphqlURL, buf)

			require.NoError(t, err)

			req.Header.Set("Content-Type", "application/json")
			client := &http.Client{}
			resp, err := client.Do(req)
			bodyBytes, err := ioutil.ReadAll(resp.Body)

			require.NoError(t, err)

			require.NoError(t, err)
			require.Equal(t, test.wantCode, resp.StatusCode)

		})

	}
}
