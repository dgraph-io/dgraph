//go:build integration2

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package edgraph

import (
	"context"
	"testing"

	"github.com/dgraph-io/dgo/v250"
	"github.com/stretchr/testify/require"

	"github.com/hypermodeinc/dgraph/v25/dgraphtest"
)

func TestNamespaces(t *testing.T) {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1)
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, err)
	defer func() { c.Cleanup(t.Failed()) }()
	require.NoError(t, c.Start())

	// ensure that Open works with no ACL
	alphaGrpcPort, err := c.GetAlphaGrpcPublicPort(0)
	require.NoError(t, err)
	_, err = dgo.Open("dgraph://localhost:" + alphaGrpcPort)
	require.NoError(t, err)

	client, cleanup, err := c.Client()
	require.NoError(t, err)
	defer cleanup()

	// Drop all data
	require.NoError(t, client.DropAll())

	// Create two namespaces
	ctx := context.Background()
	require.NoError(t, client.CreateNamespace(ctx, "ns1"))
	require.NoError(t, client.CreateNamespace(ctx, "ns2"))

	// namespace 1
	require.NoError(t, client.SetSchema(ctx, "ns1", `name: string @index(exact) .`))
	resp, err := client.RunDQL(ctx, "ns1", `{ set {_:a <name> "Alice" .}}`)
	require.NoError(t, err)
	require.Equal(t, 1, len(resp.BlankUids))
	resp, err = client.RunDQL(ctx, "ns1", `{ q(func: has(name)) { name } }`)
	require.NoError(t, err)
	require.JSONEq(t, `{"q":[{"name":"Alice"}]}`, string(resp.GetQueryResult()))

	// namespace 2
	require.NoError(t, client.SetSchema(ctx, "ns2", `name: string @index(exact) .`))
	_, err = client.RunDQL(ctx, "ns2", `{ set {_:a <name> "Bob" .}}`)
	require.NoError(t, err)
	resp, err = client.RunDQL(ctx, "ns2", `{ q(func: has(name)) { name } }`)
	require.NoError(t, err)
	require.JSONEq(t, `{"q":[{"name":"Bob"}]}`, string(resp.GetQueryResult()))

	// rename ns2 namespace
	require.NoError(t, client.RenameNamespace(ctx, "ns2", "ns2-new"))

	// check if the data is still there
	resp, err = client.RunDQL(ctx, "ns2-new", `{ q(func: has(name)) { name } }`)
	require.NoError(t, err)
	require.JSONEq(t, `{"q":[{"name":"Bob"}]}`, string(resp.GetQueryResult()))

	// List Namespaces
	nsMaps, err := client.ListNamespaces(ctx)
	require.NoError(t, err)
	require.Len(t, nsMaps, 3)

	// drop ns2-new namespace
	require.NoError(t, client.DropNamespace(ctx, "ns2-new"))
	_, err = client.RunDQL(ctx, "ns2-new", `{ q(func: has(name)) { name } }`)
	require.ErrorContains(t, err, "namespace \"ns2-new\" not found")
	nsMaps, err = client.ListNamespaces(ctx)
	require.NoError(t, err)
	require.Len(t, nsMaps, 2)

	// drop ns1 namespace
	require.NoError(t, client.DropNamespace(ctx, "ns1"))
	_, err = client.RunDQL(ctx, "ns1", `{ q(func: has(name)) { name } }`)
	require.ErrorContains(t, err, "namespace \"ns1\" not found")
}
