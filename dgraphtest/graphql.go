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
	"net/http"
	"reflect"
	"time"

	"github.com/dgraph-io/dgraph/dql"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
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

var safelyUpdateGQLSchemaErr = "New Counter: %v, Old Counter: %v.\n" +
	"Schema update counter didn't increment, " +
	"indicating that the GraphQL layer didn't get the updated schema even after 10" +
	" retries. The most probable cause is the new GraphQL schema is same as the old" +
	" GraphQL schema."

func RequireNoError(err error) {
	if err != nil {
		x.Panic(err)
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
	oldCounter := hc.ProbeGraphQLWithRetry().SchemaUpdateCounter
	fmt.Println("oldCounter>>>>>>>>>>>", oldCounter)

	// update the GraphQL schema
	gqlSchema := hc.AssertUpdateGQLSchemaSuccess(schema)

	// now, return only after the GraphQL layer has seen the schema update.
	// This makes sure that one can make queries as per the new schema.
	hc.AssertSchemaUpdateCounterIncrement(oldCounter)
	return gqlSchema
}

func (hc *HTTPClient) probeGraphQL(url string) (*ProbeGraphQLResp, error) {

	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := doReq(request)
	if err != nil {
		return nil, err
	}

	//fmt.Println("resp>>>>>>>>>>>", resp)

	probeResp := ProbeGraphQLResp{}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(resp, &probeResp); err != nil {
		return nil, err
	}
	probeResp.Healthy = true
	return &probeResp, nil
}

func (hc *HTTPClient) ProbeGraphQLWithRetry() *ProbeGraphQLResp {
	const retryCount = 10

	url := hc.probeGraphqlURL
	for i := 0; i < retryCount; i++ {
		resp, err := hc.probeGraphQL(url)
		if err == nil && resp.Healthy {
			return resp
		}

		time.Sleep(time.Second)
	}

	//t.Fatalf("no healthy response from %v after 10 retries", url)
	return nil
}

// AssertUpdateGQLSchemaSuccess updates the GraphQL schema, asserts that the update succeeded and the
// returned response is correct. It returns a *GqlSchema it received in the response.
func (hc *HTTPClient) AssertUpdateGQLSchemaSuccess(schema string) *GqlSchema {
	// update the GraphQL schema
	updateResp, _ := hc.UpdateGQLSchema(schema)

	// sanity: update response should reflect the new schema
	var updateResult struct {
		UpdateGQLSchema struct {
			GqlSchema *GqlSchema
		}
	}
	if err := json.Unmarshal(updateResp, &updateResult); err != nil {
		x.Panic(err)
	}
	if updateResult.UpdateGQLSchema.GqlSchema == nil {
		x.Panic(errors.New("Expected value not to be nil."))
	}
	_, err := dql.ParseUid(updateResult.UpdateGQLSchema.GqlSchema.Id)
	RequireNoError(err)
	// require.Equalf(t, updateResult.UpdateGQLSchema.GqlSchema.Schema, schema,
	// 	"updateGQLSchema response doesn't reflect the updated schema")
	if !Equal(updateResult.UpdateGQLSchema.GqlSchema.Schema, schema) {
		x.Panic(errors.New("updateGQLSchema response doesn't reflect the updated schema"))
	}

	return updateResult.UpdateGQLSchema.GqlSchema
}

// AssertSchemaUpdateCounterIncrement asserts that the schemaUpdateCounter is greater than the
// oldCounter, indicating that the GraphQL schema has been updated.
// If it can't make the assertion with enough retries, it fails the test.
func (hc *HTTPClient) AssertSchemaUpdateCounterIncrement(oldCounter uint64) {
	var newCounter uint64
	for i := 0; i < 20; i++ {
		if newCounter = hc.ProbeGraphQLWithRetry().SchemaUpdateCounter; newCounter == oldCounter+1 {
			return
		}
		time.Sleep(time.Second)
	}

	// Even after atleast 10 seconds, the schema update hasn't reached GraphQL layer.
	// That indicates something fatal.
	panic(fmt.Sprintf(safelyUpdateGQLSchemaErr, newCounter, oldCounter))
}
