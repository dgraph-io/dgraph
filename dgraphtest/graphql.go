/*
 * Copyright 2023 Dgraph Labs, Inc. and Contributors
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

package dgraphtest

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"runtime/debug"
	"strings"
	"time"

	"github.com/dgraph-io/dgraph/dql"
	"github.com/dgraph-io/dgraph/x"
)

type GqlSchema struct {
	Id              string
	Schema          string
	GeneratedSchema string
}

type ProbeGraphQLResp struct {
	Healthy             bool `json:"-"`
	Status              string
	SchemaUpdateCounter uint64
}

type GraphQLResponse struct {
	Data       json.RawMessage        `json:"data,omitempty"`
	Errors     x.GqlErrorList         `json:"errors,omitempty"`
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

var retryableUpdateGQLSchemaErrors = []string{
	"errIndexingInProgress",
	"is already running",
	"retry again, server is not ready", // given by Dgraph while applying the snapshot
	"Unavailable: Server not ready",    // given by GraphQL layer, during init on admin server
}

func RequireNoGQLErrors(resp *GraphQLResponse) {
	if resp != nil {
		panic(fmt.Sprintf("Expected value not to be nil."))
	}
	RequireNoError(resp.Errors)
}

func RequireNoError(err error) {
	if err != nil {
		debug.PrintStack()
		panic(fmt.Sprintf("required no GraphQL errors, but received: %s\n", err.Error()))
	}
}

func Equal(expected interface{}, actual interface{}) bool {
	// Use the reflect package to get the values of the interfaces
	value1 := reflect.ValueOf(expected)
	value2 := reflect.ValueOf(actual)

	// If the types are different, they are not equal
	if value1.Type() != value2.Type() {
		return false
	}

	// Compare the values of the interfaces
	return reflect.DeepEqual(expected, actual)
}

func (hc *HTTPClient) SafelyUpdateGQLSchema(schema string, alphaPort string) *GqlSchema {
	// first, make an initial probe to get the schema update counter
	oldCounter := hc.RetryProbeGraphQL(alphaPort).SchemaUpdateCounter

	fmt.Println("oldCounter>>>>>>>>>>>", oldCounter)

	// update the GraphQL schema
	gqlSchema := hc.AssertUpdateGQLSchemaSuccess(alphaPort, schema)

	fmt.Println("gqlSchema>>>>>>>>>>>", gqlSchema)
	// // now, return only after the GraphQL layer has seen the schema update.
	// // This makes sure that one can make queries as per the new schema.
	// AssertSchemaUpdateCounterIncrement(t, authority, oldCounter, headers)
	// return gqlSchema
	return nil
}

// **********************************************************************************************************

func (hc *HTTPClient) probeGraphQL(alphaPort string) (*ProbeGraphQLResp, error) {

	request, err := http.NewRequest("GET", "http://localhost:"+alphaPort+"/probe/graphql", nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{}
	//request.Header = header
	resp, err := client.Do(request)
	if err != nil {
		return nil, err
	}

	probeResp := ProbeGraphQLResp{}
	if resp.StatusCode == http.StatusOK {
		probeResp.Healthy = true
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(b, &probeResp); err != nil {
		return nil, err
	}
	return &probeResp, nil
}

func (hc *HTTPClient) retryProbeGraphQL(alphaPort string) *ProbeGraphQLResp {
	for i := 0; i < 10; i++ {
		resp, err := hc.probeGraphQL(alphaPort)
		if err == nil && resp.Healthy {
			return resp
		}
		time.Sleep(time.Second)
	}
	return nil
}

func (hc *HTTPClient) RetryProbeGraphQL(alphaPort string) *ProbeGraphQLResp {
	if resp := hc.retryProbeGraphQL(alphaPort); resp != nil {
		return resp
	}
	debug.PrintStack()
	//t.Fatal("Unable to get healthy response from /probe/graphql after 10 retries")
	return nil
}

// **********************************************************************************************************

// AssertUpdateGQLSchemaSuccess updates the GraphQL schema, asserts that the update succeeded and the
// returned response is correct. It returns a *GqlSchema it received in the response.
func (hc *HTTPClient) AssertUpdateGQLSchemaSuccess(alphaPort string, schema string) *GqlSchema {
	// update the GraphQL schema
	updateResp := hc.RetryUpdateGQLSchema(alphaPort, schema)
	// sanity: we shouldn't get any errors from update
	//RequireNoGQLErrors(updateResp)

	// sanity: update response should reflect the new schema
	var updateResult struct {
		UpdateGQLSchema struct {
			GqlSchema *GqlSchema
		}
	}
	if err := json.Unmarshal(updateResp, &updateResult); err != nil {
		debug.PrintStack()
		panic(fmt.Sprintf("failed to unmarshal updateGQLSchema response: %s", err.Error()))
	}
	if updateResult.UpdateGQLSchema.GqlSchema == nil {
		panic(fmt.Sprintf("Expected value not to be nil."))
	}
	_, err := dql.ParseUid(updateResult.UpdateGQLSchema.GqlSchema.Id)
	RequireNoError(err)
	// require.Equalf(t, updateResult.UpdateGQLSchema.GqlSchema.Schema, schema,
	// 	"updateGQLSchema response doesn't reflect the updated schema")
	if !Equal(updateResult.UpdateGQLSchema.GqlSchema.Schema, schema) {
		panic(fmt.Sprintf("updateGQLSchema response doesn't reflect the updated schema"))
	}

	return updateResult.UpdateGQLSchema.GqlSchema
}

// RetryUpdateGQLSchema tries to update the GraphQL schema and if it receives a retryable error, it
// keeps retrying until it either receives no error or a non-retryable error. Then it returns the
// GraphQLResponse it received as a result of calling updateGQLSchema.
func (hc *HTTPClient) RetryUpdateGQLSchema(alphaPort string, schema string) []byte {
	for {
		resp, err := hc.updateGQLSchema(alphaPort, schema)
		// return the response if we didn't get any error or get a non-retryable error
		if err == nil || !containsRetryableUpdateGQLSchemaError(err.Error()) {
			return resp
		}

		// otherwise, retry schema update
		panic(fmt.Sprintf("Got error while updateGQLSchema: %s. Retrying...\n", err.Error()))
		time.Sleep(time.Second)
	}
}

func (hc *HTTPClient) updateGQLSchema(alphaPort string, schema string) ([]byte, error) {
	updateSchemaParams := GraphQLParams{
		Query: `mutation updateGQLSchema($sch: String!) {
			updateGQLSchema(input: { set: { schema: $sch }}) {
				gqlSchema {
					id
					schema
					generatedSchema
				}
			}
		}`,
		Variables: map[string]interface{}{"sch": schema},
	}
	return hc.RunGraphqlQuery(updateSchemaParams, true)
}

func containsRetryableUpdateGQLSchemaError(str string) bool {
	for _, retryableErr := range retryableUpdateGQLSchemaErrors {
		if strings.Contains(str, retryableErr) {
			return true
		}
	}
	return false
}
