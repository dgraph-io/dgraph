/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package edgraph

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/dgraph-io/dgraph/v25/acl"
	"github.com/dgraph-io/dgraph/v25/worker"
	"github.com/dgraph-io/dgraph/v25/x"
)

// allCapabilities is every capability the posture must cover. A new constant with
// no entry here fails TestAnonymousPostureCoversEveryCapability.
var allCapabilities = []Capability{CapClusterAdmin, CapTenantAdmin, CapAssumeTenant, CapLeaseUIDs}

func restoreSecurityConfig(t *testing.T) {
	t.Helper()
	prevAcl, prevSecret := x.WorkerConfig.AclEnabled, worker.Config.AclSecretKey
	prevToken, prevIPs := worker.Config.AuthToken, x.WorkerConfig.WhiteListedIPRanges
	prevAnon := x.WorkerConfig.Anonymous
	t.Cleanup(func() {
		x.WorkerConfig.AclEnabled, worker.Config.AclSecretKey = prevAcl, prevSecret
		worker.Config.AuthToken, x.WorkerConfig.WhiteListedIPRanges = prevToken, prevIPs
		x.WorkerConfig.Anonymous = prevAnon
		SetAccessController(nil)
		x.SetAuthenticator(nil)
	})
}

// withBuiltinAuthenticator installs the composition dgraph/cmd/alpha/run.go
// installs, so that x.WithResolvedIdentity resolves a --security token the way it
// does on a live Alpha. Without it the package default is ACL only, and the token
// half of these tests would pass vacuously.
func withBuiltinAuthenticator(t *testing.T) {
	t.Helper()
	x.SetAuthenticator(PresharedAuthenticator())
}

// TestAnonymousPostureCoversEveryCapability pins that the floor is capability
// independent. A posture that denied only the capabilities someone remembered to
// list would reintroduce the omission class it exists to close.
func TestAnonymousPostureCoversEveryCapability(t *testing.T) {
	restoreSecurityConfig(t)

	for _, posture := range []x.AnonymousPosture{x.AnonymousData, x.AnonymousNone} {
		t.Run(posture.String(), func(t *testing.T) {
			inner := &fakeController{}
			ac := anonymousPosture{inner: inner, posture: posture}

			for _, c := range allCapabilities {
				err := ac.AuthorizeCapability(context.Background(), c)
				require.Errorf(t, err, "%s must be denied to a caller with no Principal", c)
				require.Equal(t, codes.Unauthenticated, status.Code(err))
				require.Contains(t, status.Convert(err).Message(), c.String(),
					"the denial must name the capability so an operator knows what to grant")
			}
			require.Empty(t, inner.asked,
				"the wrapped policy must not be consulted once the floor denies")
		})
	}
}

// TestAnonymousPostureAdmitsIdentifiedCallers pins the other half: the floor asks
// only whether the caller was identified, and hands every other question to the
// wrapped policy.
func TestAnonymousPostureAdmitsIdentifiedCallers(t *testing.T) {
	restoreSecurityConfig(t)

	for _, posture := range []x.AnonymousPosture{x.AnonymousFull, x.AnonymousData, x.AnonymousNone} {
		t.Run(posture.String(), func(t *testing.T) {
			inner := &fakeController{}
			ac := anonymousPosture{inner: inner, posture: posture}
			ctx := x.WithPrincipal(context.Background(),
				&x.Principal{Subject: "someone", Method: x.MethodPreshared})

			for _, c := range allCapabilities {
				require.NoError(t, ac.AuthorizeCapability(ctx, c))
			}
			require.Equal(t, allCapabilities, inner.asked)
		})
	}
}

// TestAnonymousPostureUnderFullIsTransparent is the v25 default. Nothing about an
// unconfigured cluster's authorization may change.
func TestAnonymousPostureUnderFullIsTransparent(t *testing.T) {
	restoreSecurityConfig(t)

	inner := &fakeController{}
	ac := anonymousPosture{inner: inner, posture: x.AnonymousFull}
	for _, c := range allCapabilities {
		require.NoError(t, ac.AuthorizeCapability(context.Background(), c),
			"anonymous=full must defer entirely to the wrapped policy")
	}
	require.Equal(t, allCapabilities, inner.asked)
}

func TestAnonymousPosturePassesPredicatesThrough(t *testing.T) {
	restoreSecurityConfig(t)

	inner := &fakeController{}
	ac := anonymousPosture{inner: inner, posture: x.AnonymousNone}

	// No Principal, the strictest posture, and the predicate half still delegates:
	// a query that names predicates the caller may not read is answered with those
	// dropped, so there is no denial for the floor to express here.
	res, err := ac.AuthorizePredicates(context.Background(), []string{"name"}, acl.Read)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, [][]string{{"name"}}, inner.askedPreds)
}

func TestEnforceAnonymousPosture(t *testing.T) {
	t.Run("full installs nothing", func(t *testing.T) {
		restoreSecurityConfig(t)
		EnforceAnonymousPosture(x.AnonymousFull)
		require.Equal(t, "predicate-acl", currentAccessController().Name(),
			"the shipped default must not add indirection")
	})

	t.Run("wraps the built-in policy", func(t *testing.T) {
		restoreSecurityConfig(t)
		EnforceAnonymousPosture(x.AnonymousData)
		require.Equal(t, "predicate-acl (anonymous=data)", currentAccessController().Name())
	})

	// The floor goes on after ConfigureIdentity precisely so a deployment policy is
	// wrapped rather than replaced. If the order were reversed the floor would be
	// silently discarded, which is the kind of failure that shows up as an
	// advisory rather than a test failure.
	t.Run("wraps an installed policy rather than replacing it", func(t *testing.T) {
		restoreSecurityConfig(t)
		fake := &fakeController{}
		SetAccessController(fake)
		EnforceAnonymousPosture(x.AnonymousNone)

		require.Equal(t, "fake (anonymous=none)", currentAccessController().Name())

		ctx := x.WithPrincipal(context.Background(), &x.Principal{Subject: "s"})
		require.NoError(t, AuthorizeCapability(ctx, CapTenantAdmin))
		require.Equal(t, []Capability{CapTenantAdmin}, fake.asked,
			"the wrapped policy must still decide for an identified caller")
	})
}

// TestBreakGlassIsNotAnIdentity is the behavioral heart of the flag.
//
// With ACL off and no token configured, break-glass grants cluster admin to any
// whitelisted source IP -- and the standalone image ships whitelist=0.0.0.0/0,
// which makes that every caller. A whitelist answers where a request came from and
// carries no credential, so under a closed posture it must stop being sufficient.
func TestBreakGlassIsNotAnIdentity(t *testing.T) {
	restoreSecurityConfig(t)
	x.WorkerConfig.AclEnabled = false
	worker.Config.AuthToken = ""

	// A loopback caller: break-glass is fully satisfied for this context.
	ctx := fromIP(t, "127.0.0.1")
	require.True(t, breakGlassSource{}.Grants(ctx, nil, CapClusterAdmin),
		"precondition: break-glass grants on its own")

	require.NoError(t, predicateACL{}.AuthorizeCapability(ctx, CapClusterAdmin),
		"anonymous=full: unchanged, the whitelist is enough")

	EnforceAnonymousPosture(x.AnonymousData)
	err := AuthorizeCapability(ctx, CapClusterAdmin)
	require.Error(t, err, "anonymous=data: a location check is not a credential")
	require.Equal(t, codes.Unauthenticated, status.Code(err))
}

// TestSecurityTokenIsAnIdentityUnderClosedPosture is the end-to-end claim the flag
// makes: a closed posture is usable without standing up full ACL, because the
// --security token now produces a Principal instead of answering a yes/no.
func TestSecurityTokenIsAnIdentityUnderClosedPosture(t *testing.T) {
	restoreSecurityConfig(t)
	withBuiltinAuthenticator(t)
	x.WorkerConfig.AclEnabled = false
	worker.Config.AuthToken = "s3cr3t-admin-token"
	EnforceAnonymousPosture(x.AnonymousData)

	t.Run("without the token", func(t *testing.T) {
		ctx := x.WithResolvedIdentity(fromIP(t, "127.0.0.1"))
		err := AuthorizeCapability(ctx, CapClusterAdmin)
		require.Error(t, err)
		require.Equal(t, codes.Unauthenticated, status.Code(err))
	})

	t.Run("with the wrong token", func(t *testing.T) {
		ctx := x.WithResolvedIdentity(withAuthToken(fromIP(t, "127.0.0.1"), "wrong"))
		err := AuthorizeCapability(ctx, CapClusterAdmin)
		require.Error(t, err)
		require.Equal(t, codes.Unauthenticated, status.Code(err))
	})

	t.Run("with the token", func(t *testing.T) {
		ctx := x.WithResolvedIdentity(withAuthToken(fromIP(t, "127.0.0.1"), "s3cr3t-admin-token"))
		require.NotNil(t, x.PrincipalFrom(ctx), "the token must resolve to a Principal")
		require.NoError(t, AuthorizeCapability(ctx, CapClusterAdmin),
			"an identified caller then falls through to break-glass as before")
	})

	// The token is an identity, not a bypass: a non-whitelisted caller presenting it
	// is identified, so the floor admits them, and break-glass then refuses on the IP.
	t.Run("the token does not bypass the whitelist", func(t *testing.T) {
		ctx := x.WithResolvedIdentity(withAuthToken(fromIP(t, "203.0.113.7"), "s3cr3t-admin-token"))
		require.NotNil(t, x.PrincipalFrom(ctx))
		err := AuthorizeCapability(ctx, CapClusterAdmin)
		require.Error(t, err)
		require.Equal(t, codes.PermissionDenied, status.Code(err),
			"denied by break-glass on the source IP, not by the posture floor")
	})
}

func TestRequireIdentifiedCaller(t *testing.T) {
	restoreSecurityConfig(t)
	identified := x.WithPrincipal(context.Background(), &x.Principal{Subject: "s"})

	tests := []struct {
		posture      x.AnonymousPosture
		wantAnonErr  bool
		wantIdentErr bool
	}{
		{posture: x.AnonymousFull, wantAnonErr: false},
		// data deliberately leaves the data plane alone: an unauthenticated query is
		// the posture's whole point of difference from none.
		{posture: x.AnonymousData, wantAnonErr: false},
		{posture: x.AnonymousNone, wantAnonErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.posture.String(), func(t *testing.T) {
			x.WorkerConfig.Anonymous = tt.posture

			err := RequireIdentifiedCaller(context.Background(), "query")
			if tt.wantAnonErr {
				require.Error(t, err)
				require.Equal(t, codes.Unauthenticated, status.Code(err))
				require.Contains(t, status.Convert(err).Message(), "query")
			} else {
				require.NoError(t, err)
			}

			require.NoError(t, RequireIdentifiedCaller(identified, "query"),
				"an identified caller is never refused by the data-plane floor")
		})
	}
}
