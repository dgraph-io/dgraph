//go:build integration

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package admin_auth

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hypermodeinc/dgraph/v25/graphql/e2e/common"
	"github.com/hypermodeinc/dgraph/v25/x"
)

const (
	authTokenHeader = "X-Dgraph-AuthToken"
	authToken       = "itIsSecret"
	wrongAuthToken  = "wrongToken"
)

func TestAdminOnlyPoorManAuth(t *testing.T) {
	schema := `type Person {
		id: ID!
		name: String!
	}`
	// without X-Dgraph-AuthToken should give error
	headers := http.Header{}
	assertAuthTokenError(t, schema, headers)

	// setting a wrong value for the token should still give error
	headers.Set(authTokenHeader, wrongAuthToken)
	assertAuthTokenError(t, schema, headers)

	// setting correct value for the token should successfully update the schema
	headers.Set(authTokenHeader, authToken)
	common.SafelyUpdateGQLSchema(t, common.Alpha1HTTP, schema, headers)
}

func TestPoorManAuthOnAdminSchemaHttpEndpoint(t *testing.T) {
	// without X-Dgraph-AuthToken should give error
	require.Contains(t, makeAdminSchemaRequest(t, ""), "Invalid X-Dgraph-AuthToken")

	// setting a wrong value for the token should still give error
	require.Contains(t, makeAdminSchemaRequest(t, wrongAuthToken), "Invalid X-Dgraph-AuthToken")

	// setting correct value for the token should successfully update the schema
	oldCounter := common.RetryProbeGraphQL(t, common.Alpha1HTTP, nil).SchemaUpdateCounter
	require.JSONEq(t, `{"data":{"code":"Success","message":"Done"}}`, makeAdminSchemaRequest(t,
		authToken))
	common.AssertSchemaUpdateCounterIncrement(t, common.Alpha1HTTP, oldCounter, nil)
}

func assertAuthTokenError(t *testing.T, schema string, headers http.Header) {
	resp := common.RetryUpdateGQLSchema(t, common.Alpha1HTTP, schema, headers)
	require.Equal(t, x.GqlErrorList{{
		Message:    "Invalid X-Dgraph-AuthToken",
		Extensions: map[string]interface{}{"code": "ErrorUnauthorized"},
	}}, resp.Errors)
	require.Nil(t, resp.Data)
}

func makeAdminSchemaRequest(t *testing.T, authTokenValue string) string {
	schema := `type Person {
		id: ID!
		name: String! @id
	}`
	req, err := http.NewRequest(http.MethodPost, common.GraphqlAdminURL+"/schema",
		strings.NewReader(schema))
	require.NoError(t, err)
	if authTokenValue != "" {
		req.Header.Set(authTokenHeader, authTokenValue)
	}

	resp, err := (&http.Client{}).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return string(b)
}
