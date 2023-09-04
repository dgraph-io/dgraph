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
	"crypto/sha256"
	"encoding/hex"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/dgraphtest"
	"github.com/dgraph-io/dgraph/x"
)

func (asuite *AclTestSuite) TestPersistentQuery() {
	t := asuite.T()

	// Galaxy Login
	hcli, err := asuite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli.LoginIntoNamespace(dgraphtest.DefaultUser, dgraphtest.DefaultPassword, x.GalaxyNamespace)
	require.NotNil(t, hcli.AccessJwt, "galaxy token is nil")
	require.NoErrorf(t, err, "login as groot into namespace %d failed", x.GalaxyNamespace)

	sch := `type Product {
		productID: ID!
		name: String @search(by: [term])
	}`
	require.NoError(t, hcli.UpdateGQLSchema(sch))

	query := "query {queryProduct{productID}}"
	b := sha256.Sum256([]byte(query))
	hash := hex.EncodeToString(b[:])

	var data1, data2 []byte
	data1, err = hcli.PostPersistentQuery(query, hash)
	require.NoError(t, err)

	// Upgrade
	asuite.Upgrade()

	// run upgrade tool for 20.11.x
	asuite.RunUpgradeTool()

	// Galaxy Login
	hcli, err = asuite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli.LoginIntoNamespace(dgraphtest.DefaultUser, dgraphtest.DefaultPassword, x.GalaxyNamespace)
	require.NotNil(t, hcli.AccessJwt, "galaxy token is nil")
	require.NoErrorf(t, err, "login as groot into namespace %d failed", x.GalaxyNamespace)

	data2, err = hcli.PostPersistentQuery("", hash)
	require.NoError(t, err)

	require.NoError(t, dgraphtest.CompareJSON(string(data1), string(data2)))
}
