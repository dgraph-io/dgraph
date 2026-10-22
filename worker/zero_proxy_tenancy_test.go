/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"

	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/x"
)

// TestForwardAssignUidsToZeroClearsClientNamespace is the regression test for the
// privilege escalation this guard fixes: uncredentialed cross-tenant UID leasing.
//
// forwardAssignUidsToZero has no other gate. Before the tenant-resolution seam it
// derived the namespace from the signed access JWT and returned that error on
// failure; after it, the built-in resolver's tolerate-a-bad-token branch left the
// client's own md["namespace"] in place and ExtractNamespace read it back. Deleting
// the x.ClearIncomingNamespace call left every package test in the repo green.
//
// The fail-closed probe resolver is what makes this a unit test: the refusal returns
// before groups().Leader(0), so no Zero connection is needed.
func TestForwardAssignUidsToZeroClearsClientNamespace(t *testing.T) {
	errRefused := errors.New("probe resolver: refusing to attribute")

	var called bool
	var saw []string
	// Installing any resolver also makes x.MultiTenancyEnabled() true, which is what
	// gates the block under test.
	x.SetTenantResolver(func(ctx context.Context) (context.Context, error) {
		called = true
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			saw = md.Get("namespace")
		}
		return ctx, errRefused
	})
	t.Cleanup(func() { x.SetTenantResolver(nil) })
	require.True(t, x.MultiTenancyEnabled(), "the guarded block would be skipped")

	// A caller asking to lease UIDs in namespace 9 with no credential at all.
	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.New(map[string]string{"namespace": "9"}))

	_, err := forwardAssignUidsToZero(ctx, &pb.Num{Val: 10, Type: pb.Num_UID})
	require.ErrorIs(t, err, errRefused,
		"an unattributable lease request must be refused here, not forwarded to Zero")
	require.True(t, called,
		"the resolver was never reached; this path no longer resolves a tenant")
	require.Empty(t, saw,
		"the resolver was handed the client's own namespace, so an uncredentialed caller "+
			"leases UIDs in whichever tenant they name. Restore the "+
			"x.ClearIncomingNamespace call in forwardAssignUidsToZero.")
}
