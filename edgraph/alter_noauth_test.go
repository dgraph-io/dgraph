/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package edgraph

import (
	"context"
	"net"
	"testing"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/dgraph-io/dgraph/v25/x"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/peer"
)

// TestValidateAlterOperationAuthGate isolates the authorization gate that an
// in-process Alter caller hits. It proves the bug (a context with no gRPC peer
// is rejected at the IP-whitelist check) and the fix (NoAuthorize skips that
// check), without booting a cluster. The full Alter body past
// validateAlterOperation needs a running cluster.
func TestValidateAlterOperationAuthGate(t *testing.T) {
	// validateAlterOperation calls x.HealthCheck before the auth checks; mark
	// the node healthy so we reach the gate we want to exercise.
	x.UpdateHealthStatus(true)
	defer x.UpdateHealthStatus(false)

	op := &api.Operation{Schema: "name: string @index(exact) @upsert ."}

	t.Run("bug: background context is rejected at the IP whitelist", func(t *testing.T) {
		// context.Background() carries no gRPC peer, so x.HasWhitelistedIP can't
		// find a source IP. This is exactly what an in-process caller passes, and
		// it fails on every config — not only under --security whitelist.
		err := validateAlterOperation(context.Background(), op, NeedAuthorize)
		require.ErrorContains(t, err, "unable to find source ip")
	})

	t.Run("the whitelist check is what rejects a non-loopback peer", func(t *testing.T) {
		// A real but non-whitelisted peer is rejected for a different reason,
		// confirming the gate we bypass below is the IP whitelist.
		ctx := peer.NewContext(context.Background(), &peer.Peer{
			Addr: &net.TCPAddr{IP: net.ParseIP("8.8.8.8"), Port: 4321},
		})
		err := validateAlterOperation(ctx, op, NeedAuthorize)
		require.ErrorContains(t, err, "unauthorized ip address: 8.8.8.8")
	})

	t.Run("fix: NoAuthorize skips the IP whitelist for in-process callers", func(t *testing.T) {
		// AlterNoAuth threads NoAuthorize through to here; the same background
		// context that failed above now passes validation.
		err := validateAlterOperation(context.Background(), op, NoAuthorize)
		require.NoError(t, err)
	})

	t.Run("NoAuthorize still enforces structural validation", func(t *testing.T) {
		// Skipping auth must not skip the "at least one field set" check.
		err := validateAlterOperation(context.Background(), &api.Operation{}, NoAuthorize)
		require.ErrorContains(t, err, "at least one field set")
	})

	t.Run("NoAuthorize still enforces the health check", func(t *testing.T) {
		x.UpdateHealthStatus(false)
		defer x.UpdateHealthStatus(true)
		err := validateAlterOperation(context.Background(), op, NoAuthorize)
		require.Error(t, err)
	})
}

// TestAlterNoAuthRejectsDrops proves AlterNoAuth is schema-only: every drop
// form is refused before reaching the cluster, so bypassing auth can never be
// used to remove data. The guard returns early, so this needs no cluster.
func TestAlterNoAuthRejectsDrops(t *testing.T) {
	drops := map[string]*api.Operation{
		"DropAll":         {DropAll: true},
		"DropOp_ALL":      {DropOp: api.Operation_ALL},
		"DropOp_DATA":     {DropOp: api.Operation_DATA},
		"DropOp_ATTR":     {DropOp: api.Operation_ATTR, DropValue: "name"},
		"DropOp_TYPE":     {DropOp: api.Operation_TYPE, DropValue: "Person"},
		"legacy DropAttr": {DropAttr: "name"},
	}
	for name, op := range drops {
		t.Run(name, func(t *testing.T) {
			_, err := (&Server{}).AlterNoAuth(context.Background(), op)
			require.ErrorContains(t, err, "only supports schema operations")
		})
	}
}
