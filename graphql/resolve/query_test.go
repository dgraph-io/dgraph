/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package resolve

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/dgraph-io/dgraph/v25/graphql/dgraph"
	"github.com/dgraph-io/dgraph/v25/graphql/schema"
	"github.com/dgraph-io/dgraph/v25/graphql/test"
	"github.com/dgraph-io/dgraph/v25/testutil"
	_ "github.com/dgraph-io/gqlparser/v2/validator/rules" // make gql validator init() all rules
)

// Tests showing that the query rewriter produces the expected Dgraph queries

type QueryRewritingCase struct {
	Name         string
	GQLQuery     string
	GQLVariables string
	DGQuery      string
	DGVars       map[string]string
}

func TestQueryRewriting(t *testing.T) {
	b, err := os.ReadFile("query_test.yaml")
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
				require.NoError(t, json.Unmarshal([]byte(tcase.GQLVariables), &vars))
			}
			op, err := gqlSchema.Operation(
				&schema.Request{
					Query:     tcase.GQLQuery,
					Variables: vars,
				})
			require.NoError(t, err)
			gqlQuery := test.GetQuery(t, op)

			dgQuery, dgVars, err := testRewriter.Rewrite(context.Background(), gqlQuery)
			require.NoError(t, err)
			require.Equal(t, tcase.DGQuery, dgraph.AsString(dgQuery))
			if len(tcase.DGVars) == 0 {
				require.Empty(t, dgVars)
			} else {
				require.Equal(t, tcase.DGVars, dgVars)
			}
		})
	}
}

// TestRegexpFilterInjectionIsContained is a regression guard for
// GHSA-33p8-wc97-5qcj (CWE-943). A `regexp` filter argument is written into the
// DQL query verbatim (it is a /.../ literal, not a %q-quotable string). A value
// crafted to close the regexp() call and append boolean DQL must NOT reach the
// query as raw text, or an unauthenticated caller can bypass the intended
// filter and read every node of the type. Country.name is
// @search(by: ["trigram"]), so it exposes a regexp filter.
func TestRegexpFilterInjectionIsContained(t *testing.T) {
	gqlSchema := test.LoadSchemaFromFile(t, "schema.graphql")
	testRewriter := NewQueryRewriter()

	// The payload closes the regexp literal and the regexp() call, then ORs in
	// has(Country.name) so the filter matches every Country.
	const gqlQuery = `query {
  queryCountry(filter: { name: { regexp: "/x/) OR has(Country.name" }}) {
    name
  }
}`

	op, err := gqlSchema.Operation(&schema.Request{Query: gqlQuery})
	require.NoError(t, err)
	q := test.GetQuery(t, op)

	dgQuery, _, err := testRewriter.Rewrite(context.Background(), q)
	require.NoError(t, err)
	got := dgraph.AsString(dgQuery)

	// Vulnerable rewriting emits the payload raw, turning it into executable DQL.
	require.NotContains(t, got, "regexp(Country.name, /x/) OR has(Country.name)",
		"regexp argument emitted as raw DQL — injection breakout is possible:\n%s", got)
	// Fixed rewriting contains the payload as a single quoted argument.
	require.Contains(t, got, `regexp(Country.name, "/x/) OR has(Country.name")`,
		"regexp argument should be quoted and contained:\n%s", got)
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
		Transport: fn,
	}
}

func newClient(t *testing.T, hrc HTTPRewritingCase) *http.Client {
	return NewTestClient(func(req *http.Request) *http.Response {
		require.Equal(t, hrc.Method, req.Method)
		require.Equal(t, hrc.URL, req.URL.String())
		if hrc.Body != "" {
			body, err := io.ReadAll(req.Body)
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
			Body: io.NopCloser(bytes.NewBufferString(hrc.HTTPResponse)),
			// Must be set to non-nil value or it panics
			Header: make(http.Header),
		}
	})
}

func TestCustomHTTPQuery(t *testing.T) {
	b, err := os.ReadFile("custom_query_test.yaml")
	require.NoError(t, err, "Unable to read test file")

	var tests []HTTPRewritingCase
	err = yaml.Unmarshal(b, &tests)
	require.NoError(t, err, "Unable to unmarshal tests to yaml.")

	gqlSchema := test.LoadSchemaFromFile(t, "schema.graphql")

	for _, tcase := range tests {
		t.Run(tcase.Name, func(t *testing.T) {
			var vars map[string]interface{}
			if tcase.Variables != "" {
				require.NoError(t, json.Unmarshal([]byte(tcase.Variables), &vars))
			}

			op, err := gqlSchema.Operation(
				&schema.Request{
					Query:     tcase.GQLQuery,
					Variables: vars,
					Header: map[string][]string{
						"bogus":       {"header"},
						"X-App-Token": {"val"},
						"Auth0-Token": {"tok"},
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
