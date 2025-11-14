//go:build integration

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package edgraph

import (
	"context"
	"testing"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/hypermodeinc/dgraph/v25/dgraphapi"
	"github.com/hypermodeinc/dgraph/v25/dgraphtest"

	"github.com/stretchr/testify/require"
)

// TODO: add a test that talks to different alphas in the same cluster
func TestDropAll(t *testing.T) {
	dc := dgraphtest.NewComposeCluster()
	client, cleanup, err := dc.Client()
	require.NoError(t, err)
	defer cleanup()

	// Drop all data
	require.NoError(t, client.Login(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword))
	require.NoError(t, client.Dgraph.DropAll(context.Background()))

	// Create two namespaces
	ctx := context.Background()
	ns1, err := client.CreateNamespace(ctx)
	require.NoError(t, err)
	ns2, err := client.CreateNamespace(ctx)
	require.NoError(t, err)

	nsMaps, err := client.ListNamespaces(ctx)
	require.NoError(t, err)
	ns1ID := nsMaps[ns1].Id
	require.NotZero(t, ns1ID)
	ns2ID := nsMaps[ns2].Id
	require.NotZero(t, ns2ID)

	// namespace 1
	require.NoError(t, client.LoginIntoNamespace(ctx,
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, ns1))
	require.NoError(t, client.SetSchema(ctx, `name: string @index(exact) .`))
	_, err = client.Mutate(&api.Mutation{
		SetNquads: []byte(`_:a <name> "Alice" .`),
		CommitNow: true,
	})
	require.NoError(t, err)
	resp, err := client.Query(`{ q(func: has(name)) { name } }`)
	require.NoError(t, err)
	require.JSONEq(t, `{"q":[{"name":"Alice"}]}`, string(resp.GetJson()))

	// namespace 2
	require.NoError(t, client.LoginIntoNamespace(ctx,
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, ns2))
	require.NoError(t, client.SetSchema(ctx, `name: string @index(exact) .`))
	_, err = client.Mutate(&api.Mutation{
		SetNquads: []byte(`_:a <name> "Bob" .`),
		CommitNow: true,
	})
	require.NoError(t, err)
	resp, err = client.Query(`{ q(func: has(name)) { name } }`)
	require.NoError(t, err)
	require.JSONEq(t, `{"q":[{"name":"Bob"}]}`, string(resp.GetJson()))

	// Drop all data
	require.NoError(t, client.Login(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword))
	require.NoError(t, client.Dgraph.DropAll(context.Background()))

	resp, err = client.Query(`{ q(func: has(name)) { name } }`)
	require.NoError(t, err)
	require.JSONEq(t, `{"q":[]}`, string(resp.GetJson()))

	require.ErrorContains(t, client.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, ns1ID),
		"invalid username or password")
}

func TestDropData(t *testing.T) {
	dc := dgraphtest.NewComposeCluster()
	client, cleanup, err := dc.Client()
	require.NoError(t, err)
	defer cleanup()

	// Drop all data
	require.NoError(t, client.Login(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword))
	require.NoError(t, client.Dgraph.DropAll(context.Background()))

	// Create two namespaces
	ctx := context.Background()
	ns1, err := client.CreateNamespace(ctx)
	require.NoError(t, err)
	ns2, err := client.CreateNamespace(ctx)
	require.NoError(t, err)

	nsMaps, err := client.ListNamespaces(ctx)
	require.NoError(t, err)
	ns1ID := nsMaps[ns1].Id
	require.NotZero(t, ns1ID)
	ns2ID := nsMaps[ns2].Id
	require.NotZero(t, ns2ID)

	// namespace 1
	require.NoError(t, client.LoginIntoNamespace(ctx,
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, ns1))
	require.NoError(t, client.SetSchema(ctx, `name: string @index(exact) .`))
	_, err = client.Mutate(&api.Mutation{
		SetNquads: []byte(`_:a <name> "Alice" .`),
		CommitNow: true,
	})
	require.NoError(t, err)
	resp, err := client.Query(`{ q(func: has(name)) { name } }`)
	require.NoError(t, err)
	require.JSONEq(t, `{"q":[{"name":"Alice"}]}`, string(resp.GetJson()))

	require.NoError(t, client.DropData(context.Background()))

	resp, err = client.Query(`{ q(func: has(name)) { name } }`)
	require.NoError(t, err)
	require.JSONEq(t, `{"q":[]}`, string(resp.GetJson()))

	resp, err = client.Query(`schema{}`)
	require.NoError(t, err)
	require.Contains(t, string(resp.GetJson()), `"predicate":"name"`)
}

func TestDropPredicate(t *testing.T) {
	dc := dgraphtest.NewComposeCluster()
	client, cleanup, err := dc.Client()
	require.NoError(t, err)
	defer cleanup()

	// Drop all data
	require.NoError(t, client.Login(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword))
	require.NoError(t, client.DropAll())

	// Create two namespaces
	ctx := context.Background()
	ns1, err := client.CreateNamespace(ctx)
	require.NoError(t, err)
	ns2, err := client.CreateNamespace(ctx)
	require.NoError(t, err)

	nsMaps, err := client.ListNamespaces(ctx)
	require.NoError(t, err)
	ns1ID := nsMaps[ns1].Id
	require.NotZero(t, ns1ID)
	ns2ID := nsMaps[ns2].Id
	require.NotZero(t, ns2ID)

	// namespace 1
	require.NoError(t, client.LoginIntoNamespace(ctx,
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, ns1))
	require.NoError(t, client.SetSchema(ctx, `name: string @index(exact) .`))
	_, err = client.Mutate(&api.Mutation{
		SetNquads: []byte(`_:a <name> "Alice" .`),
		CommitNow: true,
	})
	require.NoError(t, err)
	resp, err := client.Query(`{ q(func: has(name)) { name } }`)
	require.NoError(t, err)
	require.JSONEq(t, `{"q":[{"name":"Alice"}]}`, string(resp.GetJson()))

	require.NoError(t, client.Dgraph.DropPredicate(context.Background(), "name"))

	resp, err = client.Query(`{ q(func: has(name)) { name } }`)
	require.NoError(t, err)
	require.JSONEq(t, `{"q":[]}`, string(resp.GetJson()))

	resp, err = client.Query(`schema{}`)
	require.NoError(t, err)
	require.NotContains(t, string(resp.GetJson()), `"predicate":"name"`)
}

func TestDropType(t *testing.T) {
	dc := dgraphtest.NewComposeCluster()
	client, cleanup, err := dc.Client()
	require.NoError(t, err)
	defer cleanup()

	// Drop all data
	require.NoError(t, client.Login(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword))
	require.NoError(t, client.DropAll())

	// Create two namespaces
	ctx := context.Background()
	ns1, err := client.CreateNamespace(ctx)
	require.NoError(t, err)
	ns2, err := client.CreateNamespace(ctx)
	require.NoError(t, err)

	nsMaps, err := client.ListNamespaces(ctx)
	require.NoError(t, err)
	ns1ID := nsMaps[ns1].Id
	require.NotZero(t, ns1ID)
	ns2ID := nsMaps[ns2].Id
	require.NotZero(t, ns2ID)

	// namespace 1
	require.NoError(t, client.LoginIntoNamespace(ctx,
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, ns1))
	require.NoError(t, client.SetSchema(context.Background(),
		`name: string .
		age: int .
		type Person {
			name
			age
		}`))
	resp, err := client.Query(`schema{}`)
	require.NoError(t, err)
	require.Contains(t, string(resp.GetJson()), `"predicate":"name"`)
	resp, err = client.Query(`schema(type: Person) { }`)
	require.NoError(t, err)
	require.Contains(t, string(resp.GetJson()), `"name":"name"`)

	require.NoError(t, client.DropType(context.Background(), "Person"))
	resp, err = client.Query(`schema{}`)
	require.NoError(t, err)
	require.NotContains(t, string(resp.GetJson()), `"name":"Person"`)
	resp, err = client.Query(`schema(type: Person) { }`)
	require.NoError(t, err)
	require.NotContains(t, string(resp.GetJson()), `"name":"name"`)
}
