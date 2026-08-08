//go:build integration2

/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */
package dgraphimport

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/dgraph-io/dgraph/v25/dgraphapi"
	"github.com/dgraph-io/dgraph/v25/dgraphtest"
	"github.com/dgraph-io/dgraph/v25/x"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const canaryQuery = `{ q(func: eq(name, "canary")) { name } }`

// writeCanary sets a small schema and writes a single canary triple that later assertions use to
// detect whether the group store was wiped. The schema Alter is an admin operation, so adminCtx
// must carry whatever credential the deployment requires (an auth-token; under ACL the logged-in
// client supplies its JWT automatically).
func writeCanary(t *testing.T, adminCtx context.Context, gc *dgraphapi.GrpcClient) {
	require.NoError(t, gc.Alter(adminCtx, &api.Operation{Schema: `name: string @index(exact) .`}))
	_, err := gc.Mutate(&api.Mutation{SetNquads: []byte(`_:a <name> "canary" .`), CommitNow: true})
	require.NoError(t, err)
	requireCanaryServes(t, gc, "canary must be present before the attack")
}

// requireCanaryServes asserts the canary triple is still queryable, i.e. the store was not wiped.
func requireCanaryServes(t *testing.T, gc *dgraphapi.GrpcClient, msg string) {
	require.NoError(t, validateClientConnection(t, gc, 60*time.Second))
	resp, err := gc.Query(canaryQuery)
	require.NoError(t, err)
	require.Contains(t, string(resp.Json), "canary", msg)
}

// TestUnauthenticatedExtSnapshotStreamWipe reproduces the external-snapshot import
// authorization bug. An unauthenticated client that can reach the public gRPC port opens
// StreamExtSnapshot and sends a "Done-only" stream: no UpdateExtSnapshotStreamingState(Start)
// to arm import mode, no credentials, no data packets. The receiver previously called Badger's
// StreamWriter.Prepare() (which is dropAll()) before consuming any stream data, so the group
// store was wiped on contact.
//
// The cluster runs with neither ACL nor an --security auth-token — the bare-OSS case from the
// fix plan, where both auth gates fail open. The only control that can stop the wipe is the
// arming requirement, so this test proves that control carries the load. It fails before the
// fix (the canary triple is dropped) and passes after (the un-armed stream is rejected and the
// data survives).
func TestUnauthenticatedExtSnapshotStreamWipe(t *testing.T) {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1)
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, err)
	defer func() { c.Cleanup(t.Failed()) }()
	require.NoError(t, c.Start())

	gc, cleanup, err := c.Client()
	require.NoError(t, err)
	defer cleanup()
	require.NoError(t, waitForClusterReady(t, c, gc, 60*time.Second))

	writeCanary(t, context.Background(), gc)

	// Attacker path: a raw client with no login, no auth-token, and crucially no
	// UpdateExtSnapshotStreamingState(Start) call to arm import mode.
	url, err := c.GetAlphaGrpcEndpoint(0)
	require.NoError(t, err)
	dc, err := newClient(fmt.Sprintf("dgraph://%s", url))
	require.NoError(t, err)

	attemptDoneOnlyStream(t, context.Background(), dc, 1)

	// The store must still serve the canary. Before the fix, Prepare()/dropAll() had wiped it.
	requireCanaryServes(t, gc, "canary data was wiped by an unauthenticated, un-armed StreamExtSnapshot")
}

// TestExtSnapshotStreamAuthZACL covers the ACL-on deployment. An unauthenticated caller must be
// denied at both the unary arming RPC and the stream RPC, the store must survive, and a guardian
// JWT must be accepted. Group 1 holds the ACL/internal predicates, so an unauthenticated wipe
// here is also a privilege-escalation vector — the gates close that off.
func TestExtSnapshotStreamAuthZACL(t *testing.T) {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1).
		WithACL(time.Hour)
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, err)
	defer func() { c.Cleanup(t.Failed()) }()
	require.NoError(t, c.Start())

	gc, cleanup, err := c.Client()
	require.NoError(t, err)
	defer cleanup()
	require.NoError(t, gc.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace))
	require.NoError(t, waitForClusterReady(t, c, gc, 60*time.Second))

	// The logged-in client attaches its guardian JWT automatically.
	writeCanary(t, context.Background(), gc)

	url, err := c.GetAlphaGrpcEndpoint(0)
	require.NoError(t, err)
	dc, err := newClient(fmt.Sprintf("dgraph://%s", url))
	require.NoError(t, err)

	// Unauthenticated arming is denied (no JWT in context).
	_, err = dc.UpdateExtSnapshotStreamingState(context.Background(),
		&api.UpdateExtSnapshotStreamingStateRequest{Start: true})
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))

	// Unauthenticated streaming is denied, and the canary survives.
	attemptDoneOnlyStream(t, context.Background(), dc, 1)
	requireCanaryServes(t, gc, "canary was wiped by an unauthenticated StreamExtSnapshot under ACL")

	// A guardian JWT is accepted at the arming RPC. Obtain it via HTTP login and attach it to the
	// gRPC context the same way the dgo client does.
	hc, err := c.HTTPClient()
	require.NoError(t, err)
	require.NoError(t, hc.LoginIntoNamespace(dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace))
	gctx := metadata.AppendToOutgoingContext(context.Background(), "accessJwt", hc.AccessJwt)
	resp, err := dc.UpdateExtSnapshotStreamingState(gctx,
		&api.UpdateExtSnapshotStreamingStateRequest{Start: true})
	require.NoError(t, err)
	require.NotEmpty(t, resp.Groups, "guardian arming should report the groups it armed")
}

// TestExtSnapshotStreamAuthZToken covers the auth-token-on, ACL-off deployment. A missing or
// wrong --security token must be denied at the unary arming RPC and the stream RPC, the store
// must survive, and the correct token must be accepted.
func TestExtSnapshotStreamAuthZToken(t *testing.T) {
	const token = "super-secret-admin-token"
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1).
		WithSecurityToken(token)
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, err)
	defer func() { c.Cleanup(t.Failed()) }()
	require.NoError(t, c.Start())

	gc, cleanup, err := c.Client()
	require.NoError(t, err)
	defer cleanup()
	require.NoError(t, waitForClusterReady(t, c, gc, 60*time.Second))

	// The schema Alter is an admin op, so it must carry the auth-token.
	adminCtx := metadata.AppendToOutgoingContext(context.Background(), "auth-token", token)
	writeCanary(t, adminCtx, gc)

	url, err := c.GetAlphaGrpcEndpoint(0)
	require.NoError(t, err)
	dc, err := newClient(fmt.Sprintf("dgraph://%s", url))
	require.NoError(t, err)

	// Missing token is denied.
	_, err = dc.UpdateExtSnapshotStreamingState(context.Background(),
		&api.UpdateExtSnapshotStreamingStateRequest{Start: true})
	require.Error(t, err)
	require.ErrorContains(t, err, "No Auth Token found")

	// Wrong token is denied.
	wrongCtx := metadata.AppendToOutgoingContext(context.Background(), "auth-token", "not-the-token")
	_, err = dc.UpdateExtSnapshotStreamingState(wrongCtx,
		&api.UpdateExtSnapshotStreamingStateRequest{Start: true})
	require.Error(t, err)
	require.ErrorContains(t, err, "does not match")

	// Unauthenticated (no token) streaming is denied, and the canary survives.
	attemptDoneOnlyStream(t, context.Background(), dc, 1)
	requireCanaryServes(t, gc, "canary was wiped by a StreamExtSnapshot with no auth-token")

	// The correct token is accepted at the arming RPC.
	goodCtx := metadata.AppendToOutgoingContext(context.Background(), "auth-token", token)
	resp, err := dc.UpdateExtSnapshotStreamingState(goodCtx,
		&api.UpdateExtSnapshotStreamingStateRequest{Start: true})
	require.NoError(t, err)
	require.NotEmpty(t, resp.Groups, "token-authorized arming should report the groups it armed")
}

// attemptDoneOnlyStream opens StreamExtSnapshot, names the target group, then immediately sends
// Done with no data packets — the advisory PoC. Stream errors are tolerated and logged: after
// the fix the server rejects the stream, which is the desired outcome. The assertion that
// matters is made by the caller, on whether the store survived.
func attemptDoneOnlyStream(t *testing.T, ctx context.Context, dc api.DgraphClient, groupId uint32) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	out, err := dc.StreamExtSnapshot(ctx)
	if err != nil {
		t.Logf("StreamExtSnapshot rejected at open (expected after fix): %v", err)
		return
	}

	// First message names the target group, exactly as the legitimate client does.
	if err := out.Send(&api.StreamExtSnapshotRequest{GroupId: groupId}); err != nil {
		t.Logf("send group id failed (expected after fix): %v", err)
		return
	}
	if _, err := out.Recv(); err != nil {
		t.Logf("recv group ack failed (expected after fix): %v", err)
		return
	}

	// Done-only: no data packets. This alone triggered Prepare()/dropAll() before the fix.
	if err := out.Send(&api.StreamExtSnapshotRequest{Pkt: &api.StreamPacket{Done: true}}); err != nil {
		t.Logf("send done failed (expected after fix): %v", err)
		return
	}
	_ = out.CloseSend()

	// Drain until the server signals completion or closes the stream.
	for {
		resp, err := out.Recv()
		if err != nil {
			t.Logf("stream closed: %v", err)
			return
		}
		if resp.Finish {
			t.Logf("server reported Finish=true")
			return
		}
	}
}
