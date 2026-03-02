//go:build integration

/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package edgraph

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/dgraphapi"
	"github.com/dgraph-io/dgraph/v25/dgraphtest"
)

// waitForNamespaceCount polls ListNamespaces until the expected count is reached or timeout.
// This is needed because DropAll and DropNamespace are asynchronous operations.
func waitForNamespaceCount(t *testing.T, client *dgraphapi.GrpcClient, expected int, timeout time.Duration) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		nsMaps, err := client.ListNamespaces(ctx)
		if err != nil {
			t.Logf("ListNamespaces error (will retry): %v", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if len(nsMaps) == expected {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Final check with assertion on timeout
	nsMaps, err := client.ListNamespaces(ctx)
	require.NoError(t, err)
	require.Len(t, nsMaps, expected)
}

func TestNamespaces(t *testing.T) {
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

	// namespace 1
	require.NoError(t, client.LoginIntoNamespace(ctx,
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, ns1))
	require.NoError(t, client.SetSchema(ctx, `name: string @index(exact) .`))
	resp, err := client.RunDQL(ctx, `{ set {_:a <name> "Alice" .}}`)
	require.NoError(t, err)
	require.Equal(t, 1, len(resp.Uids))
	resp, err = client.RunDQL(ctx, `{ q(func: has(name)) { name } }`)
	require.NoError(t, err)
	require.JSONEq(t, `{"q":[{"name":"Alice"}]}`, string(resp.GetJson()))

	// namespace 2
	require.NoError(t, client.LoginIntoNamespace(ctx,
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, ns2))
	require.NoError(t, client.SetSchema(ctx, `name: string @index(exact) .`))
	_, err = client.RunDQL(ctx, `{ set {_:a <name> "Bob" .}}`)
	require.NoError(t, err)
	resp, err = client.RunDQL(ctx, `{ q(func: has(name)) { name } }`)
	require.NoError(t, err)
	require.JSONEq(t, `{"q":[{"name":"Bob"}]}`, string(resp.GetJson()))

	// List Namespaces
	require.NoError(t, client.Login(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword))
	nsMaps, err := client.ListNamespaces(ctx)
	require.NoError(t, err)
	require.Len(t, nsMaps, 3)

	// drop ns2-new namespace
	require.NoError(t, client.DropNamespace(ctx, ns2))
	require.ErrorContains(t, client.LoginIntoNamespace(ctx,
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, ns2),
		"invalid username or password")

	nsMaps, err = client.ListNamespaces(ctx)
	require.NoError(t, err)
	require.Len(t, nsMaps, 2)
}

func TestDropNamespaceErr(t *testing.T) {
	dc := dgraphtest.NewComposeCluster()
	client, cleanup, err := dc.Client()
	require.NoError(t, err)
	defer cleanup()

	// Drop all data
	require.NoError(t, client.Login(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword))
	require.NoError(t, client.DropAll())

	// Wait for DropAll to complete (async operation)
	waitForNamespaceCount(t, client, 1, 30*time.Second)

	// create ns1
	ctx := context.Background()
	ns1, err := client.CreateNamespace(ctx)
	require.NoError(t, err)

	// Dropping a non-existent namespace should not be an error
	require.NoError(t, client.DropNamespace(ctx, ns1))
	require.NoError(t, client.DropNamespace(ctx, uint64(10000000)))

	// Wait for DropNamespace to complete (async operation)
	waitForNamespaceCount(t, client, 1, 30*time.Second)
}
