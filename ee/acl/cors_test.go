//go:build (!oss && integration) || upgrade

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

package acl

import (
	"net/http"
	"strings"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/dgraphtest"
	"github.com/dgraph-io/dgraph/x"
)

func (asuite *AclTestSuite) TestCORS() {
	t := asuite.T()

	hcli, err := asuite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli.LoginIntoNamespace(dgraphtest.DefaultUser, dgraphtest.DefaultPassword, x.GalaxyNamespace)
	require.NotNil(t, hcli.AccessJwt, "galaxy token is nil")
	require.NoErrorf(t, err, "login as groot into namespace %d failed", x.GalaxyNamespace)

	sch := `type Product {
	productID: ID!
	name: String @search(by: [term])
}
# Dgraph.Allow-Origin "http://testorigin.com"
# Dgraph.Allow-Origin "http://www.testorigin.com"`
	require.NoError(t, hcli.UpdateGQLSchema(sch))

	query := `query {
		queryProduct {
			productID
		}
	}`

	resp, err := hcli.QueryWithCors(query, "http://testorigin.com")
	require.NoError(t, err)

	require.Equal(t, resp.StatusCode, http.StatusOK)
	require.Equal(t, strings.ToLower(resp.Header.Get("Content-Type")), "application/json")

	isAncestor, err := dgraphtest.IsHigherVersion(asuite.uc.Before, "v20.11.0-11-gb36b4862")
	require.NoError(t, err)
	if isAncestor {
		require.NotEqual(t, resp.Header.Get("Access-Control-Allow-Origin"), "http://testorigin.com")
	} else {
		require.Equal(t, resp.Header.Get("Access-Control-Allow-Origin"), "http://testorigin.com")
	}

	// Upgrade
	asuite.Upgrade()

	// run upgrade tool for 20.11.x
	require.NoError(t, asuite.lc.RunUpgradeTool(asuite.uc.Before, asuite.uc.After))

	hcli, err = asuite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli.LoginIntoNamespace(dgraphtest.DefaultUser, dgraphtest.DefaultPassword, x.GalaxyNamespace)
	require.NotNil(t, hcli.AccessJwt, "galaxy token is nil")
	require.NoErrorf(t, err, "login as groot into namespace %d failed", x.GalaxyNamespace)

	resp, err = hcli.QueryWithCors(query, "http://testorigin.com")
	require.NoError(t, err)

	require.Equal(t, resp.StatusCode, http.StatusOK)
	require.Equal(t, strings.ToLower(resp.Header.Get("Content-Type")), "application/json")
	require.Equal(t, resp.Header.Get("Access-Control-Allow-Origin"), "http://testorigin.com")
}
