//go:build integration2

/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/dgraph-io/dgraph/v25/dgraphtest"
)

// aclOffDenial is the message authorizeClusterAdmin produces when ACL is disabled and
// no capability source grants. Asserting on it is what stops these tests passing
// because the RPC failed for some unrelated reason — a bare PermissionDenied could
// come from anywhere.
const aclOffDenial = "cluster-admin authority is required and this caller holds none"

// TestClusterAdminAclOffRequiresAuthToken covers the token half of break-glass. It is
// the test whose absence let the ACL-off behavior change ship unnoticed: before the
// change, an ACL-off cluster granted cluster authority to anyone who could open a
// connection, so the no-token CreateNamespace below succeeded.
//
// It needs no new harness capability — WithSecurityToken already exists — which is
// why it is the half that can be trusted first.
func TestClusterAdminAclOffRequiresAuthToken(t *testing.T) {
	const token = "break-glass-token"
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1).
		WithSecurityToken(token)
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, err)
	defer func() { c.Cleanup(t.Failed()) }()
	require.NoError(t, c.Start())

	gc, cleanup, err := c.Client()
	require.NoError(t, err)
	defer cleanup()

	// The three RPCs that gate on CapClusterAdmin and nothing else. Deliberately
	// gRPC, not the admin GraphQL surface: every admin GraphQL op carries
	// IpWhitelistingMW and was already gated before the change, so it cannot
	// distinguish the old rule from the new one.
	_, err = gc.CreateNamespace(context.Background())
	require.Error(t, err,
		"an ACL-off cluster must not create namespaces for an unauthenticated caller")
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	// Assert the reason, not just the code: this text comes only from
	// authorizeClusterAdmin's ACL-off branch, so a PermissionDenied raised anywhere
	// else cannot satisfy it.
	require.ErrorContains(t, err, aclOffDenial)

	_, err = gc.ListNamespaces(context.Background())
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))

	// Assert the reason here too. Namespace 1 does not exist on a fresh cluster, so a
	// bare require.Error would pass on "no such namespace" just as readily as on a
	// refusal, and prove nothing.
	err = gc.DropNamespace(context.Background(), 1)
	require.Error(t, err,
		"dropping a namespace must not be reachable without a credential either")
	require.ErrorContains(t, err, aclOffDenial)

	// The token is the half of break-glass this cluster can supply, and it grants.
	adminCtx := metadata.AppendToOutgoingContext(context.Background(), "auth-token", token)
	ns, err := gc.CreateNamespace(adminCtx)
	require.NoError(t, err)
	require.Greater(t, ns, uint64(0))

	nsList, err := gc.ListNamespaces(adminCtx)
	require.NoError(t, err)
	require.Contains(t, nsList, ns)
	require.NoError(t, gc.DropNamespace(adminCtx, ns))
}

// TestClusterAdminAclOffRequiresWhitelistedIP covers the whitelist half — the half no
// integration test could reach before WithWhitelist, because dgraphtest hard-coded
// whitelist=0.0.0.0/0 for every cluster it built. It is the integration-level twin of
// the fromIP("203.0.113.7") case in edgraph/capability_test.go.
//
// One assumption is load-bearing and is asserted rather than assumed: the alpha must
// see this caller's source address as neither loopback nor inside the whitelist, or
// the test would pass for the wrong reason. The control test below is what detects
// that — if the source address were somehow admitted here, it would be admitted there
// too, and both would agree. They cannot both be right.
func TestClusterAdminAclOffRequiresWhitelistedIP(t *testing.T) {
	// TEST-NET-1 (RFC 5737). It cannot be the source address of a connection arriving
	// through a published Docker port, so this cluster admits no remote admin caller.
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1).
		WithWhitelist("192.0.2.0/24")
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, err)
	defer func() { c.Cleanup(t.Failed()) }()
	require.NoError(t, c.Start())

	gc, cleanup, err := c.Client()
	require.NoError(t, err)
	defer cleanup()

	// No auth token is configured, so the source IP is the only half of break-glass in
	// play, and it does not grant.
	_, err = gc.CreateNamespace(context.Background())
	require.Error(t, err, "a non-whitelisted caller must not hold cluster authority")
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	require.ErrorContains(t, err, aclOffDenial)

	_, err = gc.ListNamespaces(context.Background())
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))

	err = gc.DropNamespace(context.Background(), 1)
	require.Error(t, err)
	require.ErrorContains(t, err, aclOffDenial)
}

// TestClusterAdminAclOffWhitelistedIPGrants is the control for the test above: same
// ACL-off, no-token configuration, whitelist that admits the caller, and now the same
// three RPCs succeed. The whitelist is spelled out rather than left defaulted so the
// pairing is readable in the diff.
//
// Without this control, TestClusterAdminAclOffRequiresWhitelistedIP could pass because
// the RPCs are broken for some unrelated reason rather than because the whitelist
// refused the caller.
func TestClusterAdminAclOffWhitelistedIPGrants(t *testing.T) {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1).
		WithWhitelist("0.0.0.0/0")
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, err)
	defer func() { c.Cleanup(t.Failed()) }()
	require.NoError(t, c.Start())

	gc, cleanup, err := c.Client()
	require.NoError(t, err)
	defer cleanup()

	ns, err := gc.CreateNamespace(context.Background())
	require.NoError(t, err,
		"a whitelisted caller with no token configured holds cluster authority")
	require.Greater(t, ns, uint64(0))

	nsList, err := gc.ListNamespaces(context.Background())
	require.NoError(t, err)
	require.Contains(t, nsList, ns)
	require.NoError(t, gc.DropNamespace(context.Background(), ns))
}
