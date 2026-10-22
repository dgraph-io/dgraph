/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package edgraph

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"

	"github.com/dgraph-io/dgraph/v25/audit"
	"github.com/dgraph-io/dgraph/v25/worker"
	"github.com/dgraph-io/dgraph/v25/x"
)

// withAccessJwt puts a raw access token on the context the way a gRPC client does.
func withAccessJwt(ctx context.Context, token string) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		md = metadata.New(nil)
	} else {
		md = md.Copy()
	}
	md.Set("accessJwt", token)
	return metadata.NewIncomingContext(ctx, md)
}

// aclCtxNoResolve is aclCtx without the identity resolution, so a test can drive
// an Authenticator directly rather than observe the result of the package default.
func aclCtxNoResolve(t *testing.T, namespace uint64, userID string, groups []string) context.Context {
	t.Helper()
	token := generateJWT(namespace, userID, groups, time.Now().Add(30*time.Minute).Unix())
	return withAccessJwt(context.Background(), token)
}

// TestPresharedSubjectMatchesAudit pins the one cross-package coupling in this
// change. Audit prefers the resolved Principal over re-parsing the credential, so
// minting one for a token-bearing caller changes what every such request logs
// unless the Subject is the label audit already used.
func TestPresharedSubjectMatchesAudit(t *testing.T) {
	require.Equal(t, audit.PoorManAuth, PresharedSubject,
		"audit output for token-authenticated callers must not change")
}

func TestPresharedPrincipal(t *testing.T) {
	restoreSecurityConfig(t)
	x.WorkerConfig.AclEnabled = false

	t.Run("no token configured yields no identity", func(t *testing.T) {
		worker.Config.AuthToken = ""
		// Deliberately presenting a token the server is not configured with. An
		// unconfigured token means there is nothing to verify, so there is no
		// identity to assert -- reusing hasPoormansAuth's nil here would make every
		// anonymous caller look authenticated.
		require.Nil(t, presharedPrincipal(withAuthToken(context.Background(), "anything")))
		require.Nil(t, presharedPrincipal(context.Background()))
	})

	t.Run("token configured but not presented", func(t *testing.T) {
		worker.Config.AuthToken = "s3cr3t"
		require.Nil(t, presharedPrincipal(context.Background()))
	})

	t.Run("token configured and wrong", func(t *testing.T) {
		worker.Config.AuthToken = "s3cr3t"
		require.Nil(t, presharedPrincipal(withAuthToken(context.Background(), "nope")))
	})

	t.Run("token configured and correct", func(t *testing.T) {
		worker.Config.AuthToken = "s3cr3t"
		p := presharedPrincipal(withAuthToken(context.Background(), "s3cr3t"))
		require.NotNil(t, p)
		require.Equal(t, x.MethodPreshared, p.Method)
		require.Equal(t, PresharedIssuer, p.Issuer)
		require.Equal(t, PresharedSubject, p.Subject)
	})
}

// TestPresharedPrincipalConfersNoGroups guards the escalation the Principal type
// warns about: x.IsSuperAdmin reads Principal.Groups by name, so a "guardians"
// entry here would turn a shared secret into cluster authority without passing
// through authorizeClusterAdmin and its ACL-on exclusion.
func TestPresharedPrincipalConfersNoGroups(t *testing.T) {
	restoreSecurityConfig(t)
	x.WorkerConfig.AclEnabled = false
	worker.Config.AuthToken = "s3cr3t"

	p := presharedPrincipal(withAuthToken(context.Background(), "s3cr3t"))
	require.NotNil(t, p)
	require.Empty(t, p.Groups)
	require.False(t, x.IsSuperAdmin(p.Groups))
}

// TestPresharedAuthenticatorPrefersACL pins the ordering. ACL identifies a user
// and the token identifies a client service, so a request carrying both must
// resolve to the user -- which also keeps userDataFromPrincipal's fast path, since
// it requires Method == MethodACL.
func TestPresharedAuthenticatorPrefersACL(t *testing.T) {
	restoreSecurityConfig(t)
	x.WorkerConfig.AclEnabled = true
	worker.Config.AclSecretKey = x.Sensitive("6ABBAA2014CFF00289D20D20DA296F67")
	worker.Config.AuthToken = "s3cr3t"

	ctx := withAuthToken(aclCtxNoResolve(t, 0, "groot", []string{"guardians"}), "s3cr3t")
	p, err := presharedAuthenticator{}.Authenticate(ctx)
	require.NoError(t, err)
	require.NotNil(t, p)
	require.Equal(t, x.MethodACL, p.Method)
	require.Equal(t, "groot", p.Subject)
}

// TestPresharedAuthenticatorFallsBackOnAnUnusableJWT: a client service with a
// valid token and a stale access JWT is the service, not nobody.
func TestPresharedAuthenticatorFallsBackOnAnUnusableJWT(t *testing.T) {
	restoreSecurityConfig(t)
	x.WorkerConfig.AclEnabled = true
	worker.Config.AclSecretKey = x.Sensitive("6ABBAA2014CFF00289D20D20DA296F67")
	worker.Config.AuthToken = "s3cr3t"

	ctx := withAuthToken(withAccessJwt(context.Background(), "not-a-jwt"), "s3cr3t")
	p, err := presharedAuthenticator{}.Authenticate(ctx)
	require.NoError(t, err)
	require.NotNil(t, p)
	require.Equal(t, x.MethodPreshared, p.Method)
}

// TestPresharedAuthenticatorPropagatesACLErrors: with no token to fall back on, a
// credential that did not verify is still reported, so WithResolvedIdentity can
// log it.
func TestPresharedAuthenticatorPropagatesACLErrors(t *testing.T) {
	restoreSecurityConfig(t)
	x.WorkerConfig.AclEnabled = true
	worker.Config.AclSecretKey = x.Sensitive("6ABBAA2014CFF00289D20D20DA296F67")
	worker.Config.AuthToken = ""

	p, err := presharedAuthenticator{}.Authenticate(withAccessJwt(context.Background(), "not-a-jwt"))
	require.Error(t, err)
	require.Nil(t, p)
}

// TestPresharedAuthenticatorIsSilentWithNothingConfigured pins the shipped
// default: no ACL, no token, no identity, and no error either -- "no credential
// presented" is the normal state for the endpoints that cannot present one.
func TestPresharedAuthenticatorIsSilentWithNothingConfigured(t *testing.T) {
	restoreSecurityConfig(t)
	x.WorkerConfig.AclEnabled = false
	worker.Config.AuthToken = ""

	p, err := presharedAuthenticator{}.Authenticate(context.Background())
	require.NoError(t, err)
	require.Nil(t, p)
}
