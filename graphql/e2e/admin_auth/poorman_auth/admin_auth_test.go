/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
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

package admin_auth

import (
	"net/http"
	"testing"

	"github.com/dgraph-io/dgraph/x"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/graphql/e2e/common"
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

func assertAuthTokenError(t *testing.T, schema string, headers http.Header) {
	resp := common.RetryUpdateGQLSchema(t, common.Alpha1HTTP, schema, headers)
	require.Equal(t, x.GqlErrorList{{
		Message:    "Invalid X-Dgraph-AuthToken",
		Extensions: map[string]interface{}{"code": "ErrorUnauthorized"},
	}}, resp.Errors)
	require.Nil(t, resp.Data)
}
