//go:build integration
// +build integration

/*
 * Copyright 2024 Dgraph Labs, Inc. and Contributors
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

package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v240/protos/api"
	"github.com/dgraph-io/dgraph/v24/dgraphapi"
	"github.com/dgraph-io/dgraph/v24/x"
)

func addData(t *testing.T, gc *dgraphapi.GrpcClient) {
	require.NoError(t, gc.SetupSchema(`name: string  .`))

	rdfs := `
	_:a <name> "alice" .
	_:b <name> "bob" .
	_:c <name> "sagar" .
	_:d <name> "ajay" .`
	_, err := gc.Mutate(&api.Mutation{SetNquads: []byte(rdfs), CommitNow: true})
	require.NoError(t, err)
}

func (msuite *MultitenancyTestSuite) TestLoggingIntoTheNSAfterDropDataFromTheNS() {
	t := msuite.T()
	gc, cleanup, err := msuite.dc.Client()
	require.NoError(t, err)
	defer cleanup()

	hc, err := msuite.dc.HTTPClient()
	require.NoError(t, err)

	require.NoError(t, gc.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.GalaxyNamespace))
	require.NoError(t, gc.DropAll())

	require.NoError(t, hc.LoginIntoNamespace(dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.GalaxyNamespace))
	for i := 1; i < 5; i++ {
		ns, err := hc.AddNamespace()
		require.NoError(t, err)
		require.NoError(t, gc.LoginIntoNamespace(context.Background(), dgraphapi.DefaultUser, dgraphapi.DefaultPassword, ns))

		addData(t, gc)

		// Drop data from the namespace
		require.NoError(t, gc.Alter(context.Background(), &api.Operation{DropOp: api.Operation_DATA}))

		// Login into the namespace
		require.NoError(t, gc.LoginIntoNamespace(context.Background(),
			dgraphapi.DefaultUser, dgraphapi.DefaultPassword, ns))
	}
}

func (msuite *MultitenancyTestSuite) TestLoggingIntoAllNamespacesAfterDropDataOperationFromDefaultNs() {
	t := msuite.T()
	gc, cleanup, err := msuite.dc.Client()
	require.NoError(t, err)
	defer cleanup()

	hc, err := msuite.dc.HTTPClient()
	require.NoError(t, err)

	require.NoError(t, gc.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.GalaxyNamespace))

	require.NoError(t, gc.DropAll())
	require.NoError(t, hc.LoginIntoNamespace(dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.GalaxyNamespace))

	nss := []uint64{}
	for i := 1; i <= 2; i++ {
		ns, err := hc.AddNamespace()
		require.NoError(t, err)
		nss = append(nss, ns)
		require.NoError(t, gc.LoginIntoNamespace(context.Background(),
			dgraphapi.DefaultUser, dgraphapi.DefaultPassword, ns))

		addData(t, gc)
	}

	// Drop data from default namespace
	require.NoError(t, gc.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.GalaxyNamespace))

	require.NoError(t, gc.Alter(context.Background(), &api.Operation{DropOp: api.Operation_DATA}))

	// verify here that login into the namespace should not fail
	require.NoError(t, gc.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.GalaxyNamespace))

	for _, ns := range nss {
		require.NoError(t, gc.LoginIntoNamespace(context.Background(),
			dgraphapi.DefaultUser, dgraphapi.DefaultPassword, ns))
		query := `{
			q(func: has(name)) {
			count(uid)			
			}
		}`
		resp, err := gc.Query(query)
		require.NoError(t, err)
		require.Contains(t, string(resp.Json), `"count":4`)
	}
}
