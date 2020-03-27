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
	"context"
	"io/ioutil"
	"testing"

	"github.com/dgraph-io/dgraph/graphql/dgraph"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/graphql/test"
	"github.com/dgrijalva/jwt-go"
	"github.com/stretchr/testify/require"
	_ "github.com/vektah/gqlparser/v2/validator/rules" // make gql validator init() all rules
	"google.golang.org/grpc/metadata"
	"gopkg.in/yaml.v2"
)

// Tests showing that the query rewriter produces the expected Dgraph queries

type QueryRewritingCase struct {
	Name      string
	GQLQuery  string
	Variables map[string]interface{}
	DGQuery   string
	User      string
	Role      string
}

func TestQueryRewriting(t *testing.T) {
	b, err := ioutil.ReadFile("query_test.yaml")
	require.NoError(t, err, "Unable to read test file")

	var tests map[string][]QueryRewritingCase
	err = yaml.Unmarshal(b, &tests)
	require.NoError(t, err, "Unable to unmarshal tests to yaml.")

	testRewriter := NewQueryRewriter()

	type MyCustomClaims struct {
		Foo map[string]interface{} `json:"https://dgraph.io/jwt/claims"`
		jwt.StandardClaims
	}

	// Create the Claims
	claims := MyCustomClaims{
		map[string]interface{}{},
		jwt.StandardClaims{
			ExpiresAt: 15000,
			Issuer:    "test",
		},
	}

	testSchema := map[string]string{
		"QUERY_TESTS": "schema.graphql",
		"AUTH_TESTS":  "auth-schema.graphql",
	}

	for testType, listTests := range tests {
		gqlSchema := test.LoadSchemaFromFile(t, testSchema[testType])
		for _, tcase := range listTests {
			t.Run(tcase.Name, func(t *testing.T) {

				op, err := gqlSchema.Operation(
					&schema.Request{
						Query:     tcase.GQLQuery,
						Variables: tcase.Variables,
					})
				require.NoError(t, err)
				gqlQuery := test.GetQuery(t, op)

				ctx := context.Background()

				if tcase.User != "" || tcase.Role != "" {
					claims.Foo["X-MyApp-User"] = tcase.User
					claims.Foo["X-MyApp-Role"] = tcase.Role

					token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
					ss, err := token.SignedString([]byte("Secret"))
					require.NoError(t, err)

					md := metadata.New(nil)
					md.Append("authorizationJwt", ss)
					ctx = metadata.NewIncomingContext(ctx, md)
				}

				dgQuery, err := testRewriter.Rewrite(ctx, gqlQuery)
				require.Nil(t, err)
				require.Equal(t, tcase.DGQuery, dgraph.AsString(dgQuery))
			})
		}
	}
}
