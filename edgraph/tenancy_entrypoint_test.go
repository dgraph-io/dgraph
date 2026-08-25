/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package edgraph

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/dgraph-io/dgraph/v25/x"
)

// errProbeRefused is what the probe resolver returns instead of attributing a
// request. Refusing is what keeps these unit tests: every entry point returns the
// resolver's error immediately, so none of them reach worker, posting, or storage.
var errProbeRefused = errors.New("probe resolver: refusing to attribute")

// namespaceProbe records what an entry point handed the tenant resolver.
type namespaceProbe struct {
	called bool
	saw    []string
}

// installNamespaceProbe installs a fail-closed tenant resolver that captures the
// incoming namespace metadata it was given.
//
// The resolver is the seam the entry points already resolve through, so this needs
// no production change: whatever md["namespace"] the resolver observes is exactly
// what a deployment-specific resolver would observe in production, and a resolver
// that must not trust that value can only be safe if it never arrives.
func installNamespaceProbe(t *testing.T) *namespaceProbe {
	t.Helper()
	p := &namespaceProbe{}
	x.SetTenantResolver(func(ctx context.Context) (context.Context, error) {
		p.called = true
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			p.saw = md.Get("namespace")
		}
		return ctx, errProbeRefused
	})
	t.Cleanup(func() { x.SetTenantResolver(nil) })
	return p
}

// spoofedCtx is a request naming namespace 9 while presenting no credential of any
// kind — the uncredentialed cross-tenant caller the guard exists to stop.
func spoofedCtx() context.Context {
	return metadata.NewIncomingContext(context.Background(),
		metadata.New(map[string]string{"namespace": "9"}))
}

// assertGuarded is the whole assertion: the resolver ran, and it was not shown the
// caller's namespace.
func assertGuarded(t *testing.T, p *namespaceProbe, err error) {
	t.Helper()
	require.ErrorIs(t, err, errProbeRefused,
		"the entry point must return the resolver's refusal, not continue past it")
	require.True(t, p.called,
		"the resolver was never reached; this entry point no longer resolves a tenant "+
			"and the test proves nothing")
	require.Empty(t, p.saw,
		"the resolver was handed the client's own namespace. On the server side "+
			"md[\"namespace\"] is client-controlled, and the built-in resolver leaves it in "+
			"place when it cannot derive one from a credential — so this is an "+
			"uncredentialed caller acting in a tenant of their choosing. Restore the "+
			"x.ClearIncomingNamespace call at this entry point.")
}

// TestEntryPointsClearClientNamespace is the regression test for the entry-point
// guards. Deleting any x.ClearIncomingNamespace call left every unit and package
// test in the repo green, which is why these exist.
func TestEntryPointsClearClientNamespace(t *testing.T) {
	const q = "{ q(func: uid(0x1)) { uid } }"

	t.Run("Server.Query", func(t *testing.T) {
		p := installNamespaceProbe(t)
		_, err := (&Server{}).Query(spoofedCtx(), &api.Request{Query: q})
		assertGuarded(t, p, err)
	})

	t.Run("Server.Alter", func(t *testing.T) {
		p := installNamespaceProbe(t)
		_, err := (&Server{}).Alter(spoofedCtx(), &api.Operation{Schema: "name: string ."})
		assertGuarded(t, p, err)
	})

	t.Run("Server.RunDQL", func(t *testing.T) {
		p := installNamespaceProbe(t)
		_, err := (&Server{}).RunDQL(spoofedCtx(), &api.RunDQLRequest{DqlQuery: q})
		assertGuarded(t, p, err)
	})
}

// TestAttributedContinuationsKeepTheirNamespace is the other half of the invariant:
// the guard belongs at entry points only. A continuation that cleared the namespace
// would break the in-process callers that arrive already attributed and hold no
// credential to re-present.
func TestAttributedContinuationsKeepTheirNamespace(t *testing.T) {
	p := installNamespaceProbe(t)
	// AlterNoAuth is reached through alter() — the same continuation Alter guards
	// above — with a context an in-process caller attributed itself.
	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.New(map[string]string{"namespace": "9"}))
	_, err := (&Server{}).AlterNoAuth(ctx, &api.Operation{Schema: "name: string ."})
	require.ErrorIs(t, err, errProbeRefused)
	require.True(t, p.called)
	require.Equal(t, []string{"9"}, p.saw,
		"the guard has moved into the shared continuation: a trusted in-process caller's "+
			"namespace is being stripped")
}
