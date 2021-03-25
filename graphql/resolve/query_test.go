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

package resolve

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/dgraph-io/dgraph/graphql/dgraph"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/graphql/test"
	"github.com/dgraph-io/dgraph/testutil"
	_ "github.com/dgraph-io/gqlparser/v2/validator/rules" // make gql validator init() all rules
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

// Tests showing that the query rewriter produces the expected Dgraph queries

type QueryRewritingCase struct {
	Name         string
	GQLQuery     string
	GQLVariables string
	DGQuery      string
}

func TestQueryRewriting(t *testing.T) {
	b, err := ioutil.ReadFile("query_test.yaml")
	require.NoError(t, err, "Unable to read test file")

	var tests []QueryRewritingCase
	err = yaml.Unmarshal(b, &tests)
	require.NoError(t, err, "Unable to unmarshal tests to yaml.")

	gqlSchema := test.LoadSchemaFromFile(t, "schema.graphql")

	testRewriter := NewQueryRewriter()

	for _, tcase := range tests {
		t.Run(tcase.Name, func(t *testing.T) {
			var vars map[string]interface{}
			if tcase.GQLVariables != "" {
				err := json.Unmarshal([]byte(tcase.GQLVariables), &vars)
				require.NoError(t, err)
			}
			op, err := gqlSchema.Operation(
				&schema.Request{
					Query:     tcase.GQLQuery,
					Variables: vars,
				})
			require.NoError(t, err)
			gqlQuery := test.GetQuery(t, op)

			dgQuery, err := testRewriter.Rewrite(context.Background(), gqlQuery)
			require.Nil(t, err)
			require.Equal(t, tcase.DGQuery, dgraph.AsString(dgQuery))
		})
	}
}

type HTTPRewritingCase struct {
	Name             string
	GQLQuery         string
	Variables        string
	HTTPResponse     string
	ResolvedResponse string
	Method           string
	URL              string
	Body             string
	Headers          map[string][]string
}

// RoundTripFunc .
type RoundTripFunc func(req *http.Request) *http.Response

// RoundTrip .
func (f RoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

// NewTestClient returns *http.Client with Transport replaced to avoid making real calls
func NewTestClient(fn RoundTripFunc) *http.Client {
	return &http.Client{
		Transport: RoundTripFunc(fn),
	}
}

func newClient(t *testing.T, hrc HTTPRewritingCase) *http.Client {
	return NewTestClient(func(req *http.Request) *http.Response {
		require.Equal(t, hrc.Method, req.Method)
		require.Equal(t, hrc.URL, req.URL.String())
		if hrc.Body != "" {
			body, err := ioutil.ReadAll(req.Body)
			require.NoError(t, err)
			require.JSONEq(t, hrc.Body, string(body))
		}
		expectedHeaders := http.Header{}
		for h, v := range hrc.Headers {
			expectedHeaders.Set(h, v[0])
		}
		require.Equal(t, expectedHeaders, req.Header)

		return &http.Response{
			StatusCode: 200,
			// Send response to be tested
			Body: ioutil.NopCloser(bytes.NewBufferString(hrc.HTTPResponse)),
			// Must be set to non-nil value or it panics
			Header: make(http.Header),
		}
	})
}

func TestCustomHTTPQuery(t *testing.T) {
	b, err := ioutil.ReadFile("custom_query_test.yaml")
	require.NoError(t, err, "Unable to read test file")

	var tests []HTTPRewritingCase
	err = yaml.Unmarshal(b, &tests)
	require.NoError(t, err, "Unable to unmarshal tests to yaml.")

	gqlSchema := test.LoadSchemaFromFile(t, "schema.graphql")

	for _, tcase := range tests {
		t.Run(tcase.Name, func(t *testing.T) {
			var vars map[string]interface{}
			if tcase.Variables != "" {
				err := json.Unmarshal([]byte(tcase.Variables), &vars)
				require.NoError(t, err)
			}

			op, err := gqlSchema.Operation(
				&schema.Request{
					Query:     tcase.GQLQuery,
					Variables: vars,
					Header: map[string][]string{
						"bogus":       []string{"header"},
						"X-App-Token": []string{"val"},
						"Auth0-Token": []string{"tok"},
					},
				})
			require.NoError(t, err)
			gqlQuery := test.GetQuery(t, op)

			client := newClient(t, tcase)
			resolver := NewHTTPQueryResolver(client)
			resolved := resolver.Resolve(context.Background(), gqlQuery)

			testutil.CompareJSON(t, tcase.ResolvedResponse, string(resolved.Data))
		})
	}
}
