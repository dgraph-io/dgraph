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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
	"testing"

	"google.golang.org/grpc/metadata"

	dgoapi "github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/graphql/authorization"
	"github.com/dgraph-io/dgraph/graphql/dgraph"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/graphql/test"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
	_ "github.com/vektah/gqlparser/v2/validator/rules" // make gql validator init() all rules
	"gopkg.in/yaml.v2"
)

type AuthQueryRewritingCase struct {
	Name string

	// JWT variables
	JWTVar map[string]interface{}

	// GQL query and variables
	GQLQuery  string
	Variables string

	// Dgraph upsert query and mutations built from the GQL
	DGQuery        string
	DGQuerySec     string
	DGMutations    []*dgraphMutation
	DGMutationsSec []*dgraphMutation

	Length string

	// UIDS and json from the Dgraph result
	Uids string
	Json string

	// Post-mutation auth query and result Dgraph returns from that query
	AuthQuery string
	AuthJson  string

	// Indicates if we should skip auth query verification when using authExecutor.
	// Example: Top level RBAC rules is true.
	SkipAuth bool

	Error *x.GqlError
}

type authExecutor struct {
	t      *testing.T
	state  int
	length int

	// initial mutation
	upsertQuery []string
	json        string
	uids        string

	// auth
	authQuery string
	authJson  string

	skipAuth bool
}

func (ex *authExecutor) Execute(ctx context.Context, req *dgoapi.Request) (*dgoapi.Response, error) {
	ex.state++
	switch ex.state {
	case 1:
		// initial mutation
		ex.length -= 1

		// check that the upsert has built in auth, if required
		require.Equal(ex.t, ex.upsertQuery[ex.length], req.Query)

		var assigned map[string]string
		if ex.uids != "" {
			err := json.Unmarshal([]byte(ex.uids), &assigned)
			require.NoError(ex.t, err)
		}

		if len(assigned) == 0 {
			// skip state 2, there's no new nodes to apply auth to
			ex.state++
		}

		// For rules that don't require auth, it should directly go to step 3.
		if ex.skipAuth {
			ex.state++
		}

		if ex.length != 0 {
			ex.state = 0
		}

		return &dgoapi.Response{
			Json:    []byte(ex.json),
			Uids:    assigned,
			Metrics: &dgoapi.Metrics{NumUids: map[string]uint64{touchedUidsKey: 0}},
		}, nil

	case 2:
		// auth

		// check that we got the expected auth query
		require.Equal(ex.t, ex.authQuery, req.Query)

		// respond to query
		return &dgoapi.Response{
			Json:    []byte(ex.authJson),
			Metrics: &dgoapi.Metrics{NumUids: map[string]uint64{touchedUidsKey: 0}},
		}, nil

	case 3:
		// final result

		return &dgoapi.Response{
			Json:    []byte(`{"done": "and done"}`),
			Metrics: &dgoapi.Metrics{NumUids: map[string]uint64{touchedUidsKey: 0}},
		}, nil
	}

	panic("test failed")
}

func (ex *authExecutor) CommitOrAbort(ctx context.Context, tc *dgoapi.TxnContext) error {
	return nil
}

func TestStringCustomClaim(t *testing.T) {
	sch, err := ioutil.ReadFile("../e2e/auth/schema.graphql")
	require.NoError(t, err, "Unable to read schema file")

	authSchema, err := testutil.AppendAuthInfo(sch, authorization.HMAC256, "")
	require.NoError(t, err)

	test.LoadSchemaFromString(t, string(authSchema))
	testutil.SetAuthMeta(string(authSchema))

	// Token with string custom claim
	// "https://xyz.io/jwt/claims": "{\"USER\": \"50950b40-262f-4b26-88a7-cbbb780b2176\", \"ROLE\": \"ADMIN\"}",
	token := "eyJraWQiOiIyRWplN2tIRklLZS92MFRVT3JRYlVJWWJxSWNNUHZ2TFBjM3RSQ25EclBBPSIsImFsZyI6IkhTMjU2In0.eyJzdWIiOiI1MDk1MGI0MC0yNjJmLTRiMjYtODhhNy1jYmJiNzgwYjIxNzYiLCJjb2duaXRvOmdyb3VwcyI6WyJBRE1JTiJdLCJlbWFpbF92ZXJpZmllZCI6dHJ1ZSwiaXNzIjoiaHR0cHM6Ly9jb2duaXRvLWlkcC5hcC1zb3V0aGVhc3QtMi5hbWF6b25hd3MuY29tL2FwLXNvdXRoZWFzdC0yX0dmbWVIZEZ6NCIsImNvZ25pdG86dXNlcm5hbWUiOiI1MDk1MGI0MC0yNjJmLTRiMjYtODhhNy1jYmJiNzgwYjIxNzYiLCJodHRwczovL3h5ei5pby9qd3QvY2xhaW1zIjoie1wiVVNFUlwiOiBcIjUwOTUwYjQwLTI2MmYtNGIyNi04OGE3LWNiYmI3ODBiMjE3NlwiLCBcIlJPTEVcIjogXCJBRE1JTlwifSIsImF1ZCI6IjYzZG8wcTE2bjZlYmpna3VtdTA1a2tlaWFuIiwiZXZlbnRfaWQiOiIzMWM5ZDY4NC0xZDQ1LTQ2ZjctOGMyYi1jYzI3YjFmNmYwMWIiLCJ0b2tlbl91c2UiOiJpZCIsImF1dGhfdGltZSI6MTU5MDMzMzM1NiwibmFtZSI6IkRhdmlkIFBlZWsiLCJleHAiOjQ1OTAzNzYwMzIsImlhdCI6MTU5MDM3MjQzMiwiZW1haWwiOiJkYXZpZEB0eXBlam9pbi5jb20ifQ.g6rAkPdNIJ6wvXOo6F4XmoVqqbGs_CdUHx_k7NrvLY8"
	md := metadata.New(map[string]string{"authorizationJwt": token})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	customClaims, err := authorization.ExtractCustomClaims(ctx)
	require.NoError(t, err)
	authVar := customClaims.AuthVariables
	result := map[string]interface{}{
		"ROLE": "ADMIN",
		"USER": "50950b40-262f-4b26-88a7-cbbb780b2176",
	}
	require.Equal(t, authVar, result)
}

func TestAudienceClaim(t *testing.T) {
	sch, err := ioutil.ReadFile("../e2e/auth/schema.graphql")
	require.NoError(t, err, "Unable to read schema file")

	authSchema, err := testutil.AppendAuthInfo(sch, authorization.HMAC256, "")
	require.NoError(t, err)

	test.LoadSchemaFromString(t, string(authSchema))
	testutil.SetAuthMeta(string(authSchema))

	// Verify that authorization information is set correctly.
	metainfo := authorization.GetAuthMeta()
	require.Equal(t, metainfo.Algo, authorization.HMAC256)
	require.Equal(t, metainfo.Header, "X-Test-Auth")
	require.Equal(t, metainfo.Namespace, "https://xyz.io/jwt/claims")
	require.Equal(t, metainfo.VerificationKey, "secretkey")
	require.Equal(t, metainfo.Audience, []string{"aud1", "63do0q16n6ebjgkumu05kkeian", "aud5"})

	testCases := []struct {
		name  string
		token string
		err   error
	}{
		{
			name:  `Token with valid audience: { "aud": "63do0q16n6ebjgkumu05kkeian" }`,
			token: "eyJraWQiOiIyRWplN2tIRklLZS92MFRVT3JRYlVJWWJxSWNNUHZ2TFBjM3RSQ25EclBBPSIsImFsZyI6IkhTMjU2In0.eyJzdWIiOiI1MDk1MGI0MC0yNjJmLTRiMjYtODhhNy1jYmJiNzgwYjIxNzYiLCJjb2duaXRvOmdyb3VwcyI6WyJBRE1JTiJdLCJlbWFpbF92ZXJpZmllZCI6dHJ1ZSwiaXNzIjoiaHR0cHM6Ly9jb2duaXRvLWlkcC5hcC1zb3V0aGVhc3QtMi5hbWF6b25hd3MuY29tL2FwLXNvdXRoZWFzdC0yX0dmbWVIZEZ6NCIsImNvZ25pdG86dXNlcm5hbWUiOiI1MDk1MGI0MC0yNjJmLTRiMjYtODhhNy1jYmJiNzgwYjIxNzYiLCJodHRwczovL3h5ei5pby9qd3QvY2xhaW1zIjoie1wiVVNFUlwiOiBcIjUwOTUwYjQwLTI2MmYtNGIyNi04OGE3LWNiYmI3ODBiMjE3NlwiLCBcIlJPTEVcIjogXCJBRE1JTlwifSIsImF1ZCI6IjYzZG8wcTE2bjZlYmpna3VtdTA1a2tlaWFuIiwiZXZlbnRfaWQiOiIzMWM5ZDY4NC0xZDQ1LTQ2ZjctOGMyYi1jYzI3YjFmNmYwMWIiLCJ0b2tlbl91c2UiOiJpZCIsImF1dGhfdGltZSI6MTU5MDMzMzM1NiwibmFtZSI6IkRhdmlkIFBlZWsiLCJleHAiOjQ1OTAzNzYwMzIsImlhdCI6MTU5MDM3MjQzMiwiZW1haWwiOiJkYXZpZEB0eXBlam9pbi5jb20ifQ.g6rAkPdNIJ6wvXOo6F4XmoVqqbGs_CdUHx_k7NrvLY8",
		},
		{
			name:  `Token with invalid audience: { "aud": "invalidAudience" }`,
			token: "eyJraWQiOiIyRWplN2tIRklLZS92MFRVT3JRYlVJWWJxSWNNUHZ2TFBjM3RSQ25EclBBPSIsImFsZyI6IkhTMjU2In0.eyJzdWIiOiI1MDk1MGI0MC0yNjJmLTRiMjYtODhhNy1jYmJiNzgwYjIxNzYiLCJjb2duaXRvOmdyb3VwcyI6WyJBRE1JTiJdLCJlbWFpbF92ZXJpZmllZCI6dHJ1ZSwiaXNzIjoiaHR0cHM6Ly9jb2duaXRvLWlkcC5hcC1zb3V0aGVhc3QtMi5hbWF6b25hd3MuY29tL2FwLXNvdXRoZWFzdC0yX0dmbWVIZEZ6NCIsImNvZ25pdG86dXNlcm5hbWUiOiI1MDk1MGI0MC0yNjJmLTRiMjYtODhhNy1jYmJiNzgwYjIxNzYiLCJodHRwczovL3h5ei5pby9qd3QvY2xhaW1zIjoie1wiVVNFUlwiOiBcIjUwOTUwYjQwLTI2MmYtNGIyNi04OGE3LWNiYmI3ODBiMjE3NlwiLCBcIlJPTEVcIjogXCJBRE1JTlwifSIsImF1ZCI6ImludmFsaWRBdWRpZW5jZSIsImV2ZW50X2lkIjoiMzFjOWQ2ODQtMWQ0NS00NmY3LThjMmItY2MyN2IxZjZmMDFiIiwidG9rZW5fdXNlIjoiaWQiLCJhdXRoX3RpbWUiOjE1OTAzMzMzNTYsIm5hbWUiOiJEYXZpZCBQZWVrIiwiZXhwIjo0NTkwMzc2MDMyLCJpYXQiOjE1OTAzNzI0MzIsImVtYWlsIjoiZGF2aWRAdHlwZWpvaW4uY29tIn0.-8UxKvv6_0_hCbV3f6KEoP223BrCrP0eWWdoG-Gf3FQ",
			err:   fmt.Errorf("JWT `aud` value doesn't match with the audience"),
		},
		{
			name:  "Token without audience field",
			token: "eyJraWQiOiIyRWplN2tIRklLZS92MFRVT3JRYlVJWWJxSWNNUHZ2TFBjM3RSQ25EclBBPSIsImFsZyI6IkhTMjU2In0.eyJzdWIiOiI1MDk1MGI0MC0yNjJmLTRiMjYtODhhNy1jYmJiNzgwYjIxNzYiLCJjb2duaXRvOmdyb3VwcyI6WyJBRE1JTiJdLCJlbWFpbF92ZXJpZmllZCI6dHJ1ZSwiaXNzIjoiaHR0cHM6Ly9jb2duaXRvLWlkcC5hcC1zb3V0aGVhc3QtMi5hbWF6b25hd3MuY29tL2FwLXNvdXRoZWFzdC0yX0dmbWVIZEZ6NCIsImNvZ25pdG86dXNlcm5hbWUiOiI1MDk1MGI0MC0yNjJmLTRiMjYtODhhNy1jYmJiNzgwYjIxNzYiLCJodHRwczovL3h5ei5pby9qd3QvY2xhaW1zIjoie1wiVVNFUlwiOiBcIjUwOTUwYjQwLTI2MmYtNGIyNi04OGE3LWNiYmI3ODBiMjE3NlwiLCBcIlJPTEVcIjogXCJBRE1JTlwifSIsImV2ZW50X2lkIjoiMzFjOWQ2ODQtMWQ0NS00NmY3LThjMmItY2MyN2IxZjZmMDFiIiwidG9rZW5fdXNlIjoiaWQiLCJhdXRoX3RpbWUiOjE1OTAzMzMzNTYsIm5hbWUiOiJEYXZpZCBQZWVrIiwiZXhwIjo0NTkwMzc2MDMyLCJpYXQiOjE1OTAzNzI0MzIsImVtYWlsIjoiZGF2aWRAdHlwZWpvaW4uY29tIn0.Fjxh-sZM9eDRBRHKyLJ8MxAsSSZ-IX2f0z-Saq37t7U",
		},
		{
			name:  `Token with multiple audience: {"aud": ["aud1", "aud2", "aud3"]}`,
			token: "eyJraWQiOiIyRWplN2tIRklLZS92MFRVT3JRYlVJWWJxSWNNUHZ2TFBjM3RSQ25EclBBPSIsImFsZyI6IkhTMjU2In0.eyJzdWIiOiI1MDk1MGI0MC0yNjJmLTRiMjYtODhhNy1jYmJiNzgwYjIxNzYiLCJjb2duaXRvOmdyb3VwcyI6WyJBRE1JTiJdLCJlbWFpbF92ZXJpZmllZCI6dHJ1ZSwiaXNzIjoiaHR0cHM6Ly9jb2duaXRvLWlkcC5hcC1zb3V0aGVhc3QtMi5hbWF6b25hd3MuY29tL2FwLXNvdXRoZWFzdC0yX0dmbWVIZEZ6NCIsImNvZ25pdG86dXNlcm5hbWUiOiI1MDk1MGI0MC0yNjJmLTRiMjYtODhhNy1jYmJiNzgwYjIxNzYiLCJodHRwczovL3h5ei5pby9qd3QvY2xhaW1zIjoie1wiVVNFUlwiOiBcIjUwOTUwYjQwLTI2MmYtNGIyNi04OGE3LWNiYmI3ODBiMjE3NlwiLCBcIlJPTEVcIjogXCJBRE1JTlwifSIsImF1ZCI6WyJhdWQxIiwiYXVkMiIsImF1ZDMiXSwiZXZlbnRfaWQiOiIzMWM5ZDY4NC0xZDQ1LTQ2ZjctOGMyYi1jYzI3YjFmNmYwMWIiLCJ0b2tlbl91c2UiOiJpZCIsImF1dGhfdGltZSI6MTU5MDMzMzM1NiwibmFtZSI6IkRhdmlkIFBlZWsiLCJleHAiOjQ1OTAzNzYwMzIsImlhdCI6MTU5MDM3MjQzMiwiZW1haWwiOiJkYXZpZEB0eXBlam9pbi5jb20ifQ.LK31qlAVQHzu5mvEsPPRoNb59u8X9ITL_1re6wYGEtA",
		},
	}

	for _, tcase := range testCases {
		t.Run(tcase.name, func(t *testing.T) {
			md := metadata.New(map[string]string{"authorizationJwt": tcase.token})
			ctx := metadata.NewIncomingContext(context.Background(), md)

			customClaims, err := authorization.ExtractCustomClaims(ctx)
			require.Equal(t, tcase.err, err)
			if err != nil {
				return
			}

			authVar := customClaims.AuthVariables
			result := map[string]interface{}{
				"ROLE": "ADMIN",
				"USER": "50950b40-262f-4b26-88a7-cbbb780b2176",
			}
			require.Equal(t, authVar, result)
		})
	}
}

func TestInvalidAuthInfo(t *testing.T) {
	sch, err := ioutil.ReadFile("../e2e/auth/schema.graphql")
	require.NoError(t, err, "Unable to read schema file")
	authSchema, err := testutil.AppendJWKAndVerificationKey(sch)
	require.NoError(t, err)
	_, err = schema.NewHandler(string(authSchema), false)
	require.Error(t, err, fmt.Errorf("Expecting either JWKUrl or (VerificationKey, Algo), both were given"))
}

//Todo(Minhaj): Add a testcase for token without Expiry
func TestVerificationWithJWKUrl(t *testing.T) {
	sch, err := ioutil.ReadFile("../e2e/auth/schema.graphql")
	require.NoError(t, err, "Unable to read schema file")

	authSchema, err := testutil.AppendAuthInfoWithJWKUrl(sch)
	require.NoError(t, err)
	test.LoadSchemaFromString(t, string(authSchema))

	// Verify that authorization information is set correctly.
	metainfo := authorization.GetAuthMeta()
	require.Equal(t, metainfo.Algo, "")
	require.Equal(t, metainfo.Header, "X-Test-Auth")
	require.Equal(t, metainfo.Namespace, "https://xyz.io/jwt/claims")
	require.Equal(t, metainfo.VerificationKey, "")
	require.Equal(t, metainfo.JWKUrl, "https://www.googleapis.com/service_accounts/v1/jwk/securetoken@system.gserviceaccount.com")

	testCase := struct {
		name  string
		token string
	}{
		name:  `Expired Token`,
		token: "eyJhbGciOiJSUzI1NiIsImtpZCI6IjE2NzUwM2UwYWVjNTJkZGZiODk2NTIxYjkxN2ZiOGUyMGMxZjMzMDAiLCJ0eXAiOiJKV1QifQ.eyJpc3MiOiJodHRwczovL3NlY3VyZXRva2VuLmdvb2dsZS5jb20vZmlyLXByb2plY3QxLTI1OWU3IiwiYXVkIjoiZmlyLXByb2plY3QxLTI1OWU3IiwiYXV0aF90aW1lIjoxNjAxNDQ0NjM0LCJ1c2VyX2lkIjoiMTdHb3h2dU5CWlc5YTlKU3Z3WXhROFc0bjE2MyIsInN1YiI6IjE3R294dnVOQlpXOWE5SlN2d1l4UThXNG4xNjMiLCJpYXQiOjE2MDE0NDQ2MzQsImV4cCI6MTYwMTQ0ODIzNCwiZW1haWwiOiJtaW5oYWpAZGdyYXBoLmlvIiwiZW1haWxfdmVyaWZpZWQiOmZhbHNlLCJmaXJlYmFzZSI6eyJpZGVudGl0aWVzIjp7ImVtYWlsIjpbIm1pbmhhakBkZ3JhcGguaW8iXX0sInNpZ25faW5fcHJvdmlkZXIiOiJwYXNzd29yZCJ9fQ.q5YmOzOUkZHNjlz53hgLNSVg-brIU9tLJ4jLC0_Xurl5wEbyZ6D_KQ9-UFqbl2HR6R1V5kpaf6eDFR3c83i1PpCbJ4LTjHAf_njQvL75ByERld23lZtKZyEeE6ujdFXL8ne4fI2qenD1Xeqx9AnXbLf7U_CvZpbX3l1wj7p0Lpn7qixi0AztuLSJMLkMfFpaiwyFZQivi4cqtnI25VIsK6a4KIpl1Sk0AHT-lv9PRadd_JDjWAIzD0SfhpZOskaeA9PljVMp-Y3Xscwg_Qc6u1MIBPg1jKO-ngjhWkgEWBoz5F836P7phT60LVBHhYuk-jRN6HSSNWQ3ineuN-jBkg",
	}

	md := metadata.New(map[string]string{"authorizationJwt": testCase.token})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	_, err = authorization.ExtractCustomClaims(ctx)
	require.True(t, strings.Contains(err.Error(), "unable to parse jwt token:token is unverifiable: Keyfunc returned an error"))

}

// TODO(arijit): Generate the JWT token instead of using pre generated token.
func TestJWTExpiry(t *testing.T) {
	sch, err := ioutil.ReadFile("../e2e/auth/schema.graphql")
	require.NoError(t, err, "Unable to read schema file")

	authSchema, err := testutil.AppendAuthInfo(sch, authorization.HMAC256, "")
	require.NoError(t, err)

	test.LoadSchemaFromString(t, string(authSchema))
	testutil.SetAuthMeta(string(authSchema))

	// Verify that authorization information is set correctly.
	metainfo := authorization.GetAuthMeta()
	require.Equal(t, metainfo.Algo, authorization.HMAC256)
	require.Equal(t, metainfo.Header, "X-Test-Auth")
	require.Equal(t, metainfo.Namespace, "https://xyz.io/jwt/claims")
	require.Equal(t, metainfo.VerificationKey, "secretkey")

	testCases := []struct {
		name    string
		token   string
		invalid bool
	}{
		{
			name:  `Token without expiry value`,
			token: "eyJraWQiOiIyRWplN2tIRklLZS92MFRVT3JRYlVJWWJxSWNNUHZ2TFBjM3RSQ25EclBBPSIsImFsZyI6IkhTMjU2In0.eyJzdWIiOiI1MDk1MGI0MC0yNjJmLTRiMjYtODhhNy1jYmJiNzgwYjIxNzYiLCJjb2duaXRvOmdyb3VwcyI6WyJBRE1JTiJdLCJlbWFpbF92ZXJpZmllZCI6dHJ1ZSwiaXNzIjoiaHR0cHM6Ly9jb2duaXRvLWlkcC5hcC1zb3V0aGVhc3QtMi5hbWF6b25hd3MuY29tL2FwLXNvdXRoZWFzdC0yX0dmbWVIZEZ6NCIsImNvZ25pdG86dXNlcm5hbWUiOiI1MDk1MGI0MC0yNjJmLTRiMjYtODhhNy1jYmJiNzgwYjIxNzYiLCJodHRwczovL3h5ei5pby9qd3QvY2xhaW1zIjoie1wiVVNFUlwiOiBcIjUwOTUwYjQwLTI2MmYtNGIyNi04OGE3LWNiYmI3ODBiMjE3NlwiLCBcIlJPTEVcIjogXCJBRE1JTlwifSIsImV2ZW50X2lkIjoiMzFjOWQ2ODQtMWQ0NS00NmY3LThjMmItY2MyN2IxZjZmMDFiIiwidG9rZW5fdXNlIjoiaWQiLCJhdXRoX3RpbWUiOjE1OTAzMzMzNTYsIm5hbWUiOiJEYXZpZCBQZWVrIiwiaWF0IjoxNTkwMzcyNDMyLCJlbWFpbCI6ImRhdmlkQHR5cGVqb2luLmNvbSJ9.f79YmZgz_YDBzf0dQ_dY_VQOjpGt4Z_MJ3LsvXrIQeQ",
		},
		{
			name:    `Expired token`,
			token:   "eyJraWQiOiIyRWplN2tIRklLZS92MFRVT3JRYlVJWWJxSWNNUHZ2TFBjM3RSQ25EclBBPSIsImFsZyI6IkhTMjU2In0.eyJzdWIiOiI1MDk1MGI0MC0yNjJmLTRiMjYtODhhNy1jYmJiNzgwYjIxNzYiLCJjb2duaXRvOmdyb3VwcyI6WyJBRE1JTiJdLCJlbWFpbF92ZXJpZmllZCI6dHJ1ZSwiaXNzIjoiaHR0cHM6Ly9jb2duaXRvLWlkcC5hcC1zb3V0aGVhc3QtMi5hbWF6b25hd3MuY29tL2FwLXNvdXRoZWFzdC0yX0dmbWVIZEZ6NCIsImNvZ25pdG86dXNlcm5hbWUiOiI1MDk1MGI0MC0yNjJmLTRiMjYtODhhNy1jYmJiNzgwYjIxNzYiLCJodHRwczovL3h5ei5pby9qd3QvY2xhaW1zIjoie1wiVVNFUlwiOiBcIjUwOTUwYjQwLTI2MmYtNGIyNi04OGE3LWNiYmI3ODBiMjE3NlwiLCBcIlJPTEVcIjogXCJBRE1JTlwifSIsImV2ZW50X2lkIjoiMzFjOWQ2ODQtMWQ0NS00NmY3LThjMmItY2MyN2IxZjZmMDFiIiwidG9rZW5fdXNlIjoiaWQiLCJhdXRoX3RpbWUiOjE1OTAzMzMzNTYsIm5hbWUiOiJEYXZpZCBQZWVrIiwiZXhwIjo1OTAzNzYwMzIsImlhdCI6MTU5MDM3MjQzMiwiZW1haWwiOiJkYXZpZEB0eXBlam9pbi5jb20ifQ.cxTip2mZLf6hYBHYAyJ7pqohhpMdrVOaySFAtp3PfKg",
			invalid: true,
		},
	}

	for _, tcase := range testCases {
		t.Run(tcase.name, func(t *testing.T) {
			md := metadata.New(map[string]string{"authorizationJwt": tcase.token})
			ctx := metadata.NewIncomingContext(context.Background(), md)

			customClaims, err := authorization.ExtractCustomClaims(ctx)
			if tcase.invalid {
				require.True(t, strings.Contains(err.Error(), "token is expired"))
				return
			}

			authVar := customClaims.AuthVariables
			result := map[string]interface{}{
				"ROLE": "ADMIN",
				"USER": "50950b40-262f-4b26-88a7-cbbb780b2176",
			}
			require.Equal(t, authVar, result)
		})
	}
}

// Tests showing that the query rewriter produces the expected Dgraph queries
// when it also needs to write in auth.
func queryRewriting(t *testing.T, sch string, authMeta *testutil.AuthMeta) {
	b, err := ioutil.ReadFile("auth_query_test.yaml")
	require.NoError(t, err, "Unable to read test file")

	var tests []AuthQueryRewritingCase
	err = yaml.Unmarshal(b, &tests)
	require.NoError(t, err, "Unable to unmarshal tests to yaml.")

	testRewriter := NewQueryRewriter()
	gqlSchema := test.LoadSchemaFromString(t, sch)

	for _, tcase := range tests {
		t.Run(tcase.Name, func(t *testing.T) {
			op, err := gqlSchema.Operation(
				&schema.Request{
					Query: tcase.GQLQuery,
					// Variables: tcase.Variables,
				})
			require.NoError(t, err)
			gqlQuery := test.GetQuery(t, op)

			// Clear the map and initialize it.
			authMeta.AuthVars = make(map[string]interface{})
			for k, v := range tcase.JWTVar {
				authMeta.AuthVars[k] = v
			}

			ctx := context.Background()
			if !strings.HasPrefix(tcase.Name, "Query with missing jwt token") {
				ctx, err = authMeta.AddClaimsToContext(ctx)
				require.NoError(t, err)
			}

			dgQuery, err := testRewriter.Rewrite(ctx, gqlQuery)
			require.Nil(t, err)
			require.Equal(t, tcase.DGQuery, dgraph.AsString(dgQuery))

			// Check for unused variables.
			_, err = gql.Parse(gql.Request{Str: dgraph.AsString(dgQuery)})
			require.NoError(t, err)
		})
	}
}

// Tests that the queries that run after a mutation get auth correctly added in.
func mutationQueryRewriting(t *testing.T, sch string, authMeta *testutil.AuthMeta) {
	tests := map[string]struct {
		gqlMut   string
		rewriter func() MutationRewriter
		assigned map[string]string
		result   map[string]interface{}
		dgQuery  string
	}{
		"Add Ticket": {
			gqlMut: `mutation {
				addTicket(input: [{title: "A ticket", onColumn: {colID: "0x1"}}]) {
				  ticket {
					id
					title
					onColumn {
						colID
						name
					}
				  }
				}
			  }`,
			rewriter: NewAddRewriter,
			assigned: map[string]string{"Ticket1": "0x4"},
			dgQuery: `query {
  ticket(func: uid(TicketRoot)) {
    id : uid
    title : Ticket.title
    onColumn : Ticket.onColumn @filter(uid(Column3)) {
      colID : uid
      name : Column.name
    }
  }
  TicketRoot as var(func: uid(Ticket4)) @filter(uid(TicketAuth5))
  Ticket4 as var(func: uid(0x4))
  TicketAuth5 as var(func: uid(Ticket4)) @cascade {
    onColumn : Ticket.onColumn {
      inProject : Column.inProject {
        roles : Project.roles @filter(eq(Role.permission, "VIEW")) {
          assignedTo : Role.assignedTo @filter(eq(User.username, "user1"))
          dgraph.uid : uid
        }
        dgraph.uid : uid
      }
      dgraph.uid : uid
    }
    dgraph.uid : uid
  }
  var(func: uid(TicketRoot)) {
    Column1 as Ticket.onColumn
  }
  Column3 as var(func: uid(Column1)) @filter(uid(ColumnAuth2))
  ColumnAuth2 as var(func: uid(Column1)) @cascade {
    inProject : Column.inProject {
      roles : Project.roles @filter(eq(Role.permission, "VIEW")) {
        assignedTo : Role.assignedTo @filter(eq(User.username, "user1"))
        dgraph.uid : uid
      }
      dgraph.uid : uid
    }
    dgraph.uid : uid
  }
}`,
		},
		"Update Ticket": {
			gqlMut: `mutation {
				updateTicket(input: {filter: {id: ["0x4"]}, set: {title: "Updated title"} }) {
					ticket {
						id
						title
						onColumn {
							colID
							name
						}
					  }
				}
			  }`,
			rewriter: NewUpdateRewriter,
			result: map[string]interface{}{
				"updateTicket": []interface{}{map[string]interface{}{"uid": "0x4"}}},
			dgQuery: `query {
  ticket(func: uid(TicketRoot)) {
    id : uid
    title : Ticket.title
    onColumn : Ticket.onColumn @filter(uid(Column3)) {
      colID : uid
      name : Column.name
    }
  }
  TicketRoot as var(func: uid(Ticket4)) @filter(uid(TicketAuth5))
  Ticket4 as var(func: uid(0x4))
  TicketAuth5 as var(func: uid(Ticket4)) @cascade {
    onColumn : Ticket.onColumn {
      inProject : Column.inProject {
        roles : Project.roles @filter(eq(Role.permission, "VIEW")) {
          assignedTo : Role.assignedTo @filter(eq(User.username, "user1"))
          dgraph.uid : uid
        }
        dgraph.uid : uid
      }
      dgraph.uid : uid
    }
    dgraph.uid : uid
  }
  var(func: uid(TicketRoot)) {
    Column1 as Ticket.onColumn
  }
  Column3 as var(func: uid(Column1)) @filter(uid(ColumnAuth2))
  ColumnAuth2 as var(func: uid(Column1)) @cascade {
    inProject : Column.inProject {
      roles : Project.roles @filter(eq(Role.permission, "VIEW")) {
        assignedTo : Role.assignedTo @filter(eq(User.username, "user1"))
        dgraph.uid : uid
      }
      dgraph.uid : uid
    }
    dgraph.uid : uid
  }
}`,
		},
	}

	gqlSchema := test.LoadSchemaFromString(t, sch)

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			// -- Arrange --
			rewriter := tt.rewriter()
			op, err := gqlSchema.Operation(&schema.Request{Query: tt.gqlMut})
			require.NoError(t, err)
			gqlMutation := test.GetMutation(t, op)

			authMeta.AuthVars = map[string]interface{}{
				"USER": "user1",
			}
			ctx, err := authMeta.AddClaimsToContext(context.Background())
			require.NoError(t, err)

			_, err = rewriter.Rewrite(ctx, gqlMutation)
			require.Nil(t, err)

			// -- Act --
			dgQuery, err := rewriter.FromMutationResult(
				ctx, gqlMutation, tt.assigned, tt.result)

			// -- Assert --
			require.Nil(t, err)
			require.Equal(t, tt.dgQuery, dgraph.AsString(dgQuery))

			// Check for unused variables.
			_, err = gql.Parse(gql.Request{Str: dgraph.AsString(dgQuery)})
			require.NoError(t, err)
		})

	}
}

// Tests showing that the query rewriter produces the expected Dgraph queries
// for delete when it also needs to write in auth - this doesn't extend to other nodes
// it only ever applies at the top level because delete only deletes the nodes
// referenced by the filter, not anything deeper.
func deleteQueryRewriting(t *testing.T, sch string, authMeta *testutil.AuthMeta) {
	b, err := ioutil.ReadFile("auth_delete_test.yaml")
	require.NoError(t, err, "Unable to read test file")

	var tests []AuthQueryRewritingCase
	err = yaml.Unmarshal(b, &tests)
	require.NoError(t, err, "Unable to unmarshal tests to yaml.")

	compareMutations := func(t *testing.T, test []*dgraphMutation, generated []*dgoapi.Mutation) {
		require.Len(t, generated, len(test))
		for i, expected := range test {
			require.Equal(t, expected.Cond, generated[i].Cond)
			if len(generated[i].SetJson) > 0 || expected.SetJSON != "" {
				require.JSONEq(t, expected.SetJSON, string(generated[i].SetJson))
			}
			if len(generated[i].DeleteJson) > 0 || expected.DeleteJSON != "" {
				require.JSONEq(t, expected.DeleteJSON, string(generated[i].DeleteJson))
			}
		}
	}

	gqlSchema := test.LoadSchemaFromString(t, sch)

	for _, tcase := range tests {
		t.Run(tcase.Name, func(t *testing.T) {
			// -- Arrange --
			var vars map[string]interface{}
			if tcase.Variables != "" {
				err := json.Unmarshal([]byte(tcase.Variables), &vars)
				require.NoError(t, err)
			}

			op, err := gqlSchema.Operation(
				&schema.Request{
					Query:     tcase.GQLQuery,
					Variables: vars,
				})
			require.NoError(t, err)
			mut := test.GetMutation(t, op)
			rewriterToTest := NewDeleteRewriter()

			// Clear the map and initialize it.
			authMeta.AuthVars = make(map[string]interface{})
			for k, v := range tcase.JWTVar {
				authMeta.AuthVars[k] = v
			}

			ctx, err := authMeta.AddClaimsToContext(context.Background())
			require.NoError(t, err)

			// -- Act --
			upsert, err := rewriterToTest.Rewrite(ctx, mut)

			// -- Assert --
			if tcase.Error != nil || err != nil {
				require.NotNil(t, err)
				require.NotNil(t, tcase.Error)
				require.Equal(t, tcase.Error.Error(), err.Error())
				return
			}

			require.Equal(t, tcase.DGQuery, dgraph.AsString(upsert[0].Query))
			compareMutations(t, tcase.DGMutations, upsert[0].Mutations)

			if len(upsert) > 1 {
				require.Equal(t, tcase.DGQuerySec, dgraph.AsString(upsert[1].Query))
				compareMutations(t, tcase.DGMutationsSec, upsert[1].Mutations)
			}
		})
	}
}

// In an add mutation
//
// mutation {
// 	addAnswer(input: [
// 	  {
// 		text: "...",
// 		datePublished: "2020-03-26",
// 		author: { username: "u1" },
// 		inAnswerTo: { id: "0x7e" }
// 	  }
// 	]) {
// 	  answer { ... }
//
// There's no initial auth verification.  We add the nodes and then check the auth rules.
// So the only auth to check is through authorizeNewNodes() function.
//
// We don't need to test the json mutations that are created, because those are the same
// as in add_mutation_test.yaml.  What we need to test is the processing around if
// new nodes are checked properly - the query generated to check them, and the post-processing.
func mutationAdd(t *testing.T, sch string, authMeta *testutil.AuthMeta) {
	b, err := ioutil.ReadFile("auth_add_test.yaml")
	require.NoError(t, err, "Unable to read test file")

	var tests []AuthQueryRewritingCase
	err = yaml.Unmarshal(b, &tests)
	require.NoError(t, err, "Unable to unmarshal tests to yaml.")

	gqlSchema := test.LoadSchemaFromString(t, sch)

	for _, tcase := range tests {
		t.Run(tcase.Name, func(t *testing.T) {
			checkAddUpdateCase(t, gqlSchema, tcase, NewAddRewriter, authMeta)
		})
	}
}

// In an update mutation we first need to check that the generated query only finds the
// authorised nodes - it takes the users filter and applies auth.  Then we need to check
// that any nodes added by the mutation were also allowed.
//
// We don't need to test the json mutations that are created, because those are the same
// as in update_mutation_test.yaml.  What we need to test is the processing around if
// new nodes are checked properly - the query generated to check them, and the post-processing.
func mutationUpdate(t *testing.T, sch string, authMeta *testutil.AuthMeta) {
	b, err := ioutil.ReadFile("auth_update_test.yaml")
	require.NoError(t, err, "Unable to read test file")

	var tests []AuthQueryRewritingCase
	err = yaml.Unmarshal(b, &tests)
	require.NoError(t, err, "Unable to unmarshal tests to yaml.")

	gqlSchema := test.LoadSchemaFromString(t, sch)
	for _, tcase := range tests {
		t.Run(tcase.Name, func(t *testing.T) {
			checkAddUpdateCase(t, gqlSchema, tcase, NewUpdateRewriter, authMeta)
		})
	}
}

func checkAddUpdateCase(
	t *testing.T,
	gqlSchema schema.Schema,
	tcase AuthQueryRewritingCase,
	rewriter func() MutationRewriter,
	authMeta *testutil.AuthMeta) {
	// -- Arrange --
	var vars map[string]interface{}
	if tcase.Variables != "" {
		err := json.Unmarshal([]byte(tcase.Variables), &vars)
		require.NoError(t, err)
	}

	op, err := gqlSchema.Operation(
		&schema.Request{
			Query:     tcase.GQLQuery,
			Variables: vars,
		})
	require.NoError(t, err)
	mut := test.GetMutation(t, op)

	// Clear the map and initialize it.
	authMeta.AuthVars = make(map[string]interface{})
	for k, v := range tcase.JWTVar {
		authMeta.AuthVars[k] = v
	}

	ctx, err := authMeta.AddClaimsToContext(context.Background())
	require.NoError(t, err)

	length := 1
	upsertQuery := []string{tcase.DGQuery}

	if tcase.Length != "" {
		length, _ = strconv.Atoi(tcase.Length)
	}

	if length == 2 {
		upsertQuery = []string{tcase.DGQuerySec, tcase.DGQuery}
	}

	ex := &authExecutor{
		t:           t,
		upsertQuery: upsertQuery,
		json:        tcase.Json,
		uids:        tcase.Uids,
		authQuery:   tcase.AuthQuery,
		authJson:    tcase.AuthJson,
		skipAuth:    tcase.SkipAuth,
		length:      length,
	}
	resolver := NewDgraphResolver(rewriter(), ex, StdMutationCompletion(mut.ResponseName()))

	// -- Act --
	resolved, _ := resolver.Resolve(ctx, mut)

	// -- Assert --
	// most cases are built into the authExecutor
	if tcase.Error != nil {
		require.Equal(t, tcase.Error.Error(), resolved.Err.Error())
	}
}

func TestAuthQueryRewriting(t *testing.T) {
	sch, err := ioutil.ReadFile("../e2e/auth/schema.graphql")
	require.NoError(t, err, "Unable to read schema file")

	jwtAlgo := []string{authorization.HMAC256, authorization.RSA256}

	for _, algo := range jwtAlgo {
		result, err := testutil.AppendAuthInfo(sch, algo, "../e2e/auth/sample_public_key.pem")
		require.NoError(t, err)
		strSchema := string(result)

		authMeta, err := authorization.Parse(strSchema)
		authorization.SetAuthMeta(authMeta)

		metaInfo := &testutil.AuthMeta{
			PublicKey: authMeta.VerificationKey,
			Namespace: authMeta.Namespace,
			Algo:      authMeta.Algo,
		}

		require.NoError(t, err)

		t.Run("Query Rewriting "+algo, func(t *testing.T) {
			queryRewriting(t, strSchema, metaInfo)
		})

		t.Run("Mutation Query Rewriting "+algo, func(t *testing.T) {
			mutationQueryRewriting(t, strSchema, metaInfo)
		})

		t.Run("Add Mutation "+algo, func(t *testing.T) {
			mutationAdd(t, strSchema, metaInfo)
		})

		t.Run("Update Mutation "+algo, func(t *testing.T) {
			mutationUpdate(t, strSchema, metaInfo)
		})

		t.Run("Delete Query Rewriting "+algo, func(t *testing.T) {
			deleteQueryRewriting(t, strSchema, metaInfo)
		})
	}
}
