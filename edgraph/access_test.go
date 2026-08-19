/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package edgraph

import (
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"

	"github.com/dgraph-io/dgraph/v25/acl"
	"github.com/dgraph-io/dgraph/v25/worker"
	"github.com/dgraph-io/dgraph/v25/x"
)

func generateJWT(namespace uint64, userId string, groupIds []string, expiry int64) string {
	claims := jwt.MapClaims{"namespace": namespace, "userid": userId, "exp": expiry}
	if groupIds != nil {
		claims["groups"] = groupIds
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &claims)
	tokenString, err := token.SignedString(x.MaybeKeyToBytes(worker.Config.AclSecretKey))
	if err != nil {
		panic(err)
	}

	return tokenString
}

func TestValidateToken(t *testing.T) {
	expiry := time.Now().Add(time.Minute * 30).Unix()
	userDataList := []userData{
		{1234567890, "user1", []string{"701", "702"}},
		{2345678901, "user2", []string{"703", "701"}},
		{3456789012, "user3", []string{"702", "703"}},
	}

	for _, userdata := range userDataList {
		tokenString := generateJWT(userdata.namespace, userdata.userId, userdata.groupIds, expiry)
		ud, err := validateToken(tokenString)
		require.NoError(t, err)
		require.Equal(t, userdata.namespace, ud.namespace)
		require.Equal(t, userdata.userId, ud.userId)
		require.Equal(t, userdata.groupIds, ud.groupIds)
	}
}

func TestGetAccessJwt(t *testing.T) {
	grpLst := []acl.Group{
		{
			Uid:     "100",
			GroupID: "1001",
			Users:   []acl.User{},
			Rules:   []acl.Acl{},
		},
		{
			Uid:     "101",
			GroupID: "1011",
			Users:   []acl.User{},
			Rules:   []acl.Acl{},
		},
		{
			Uid:     "102",
			GroupID: "1021",
			Users:   []acl.User{},
			Rules:   []acl.Acl{},
		},
	}

	g := acl.GetGroupIDs(grpLst)
	userDataList := []userData{
		{1234567890, "user1", []string{"701", "702"}},
		{2345678901, "user2", []string{"703", "701"}},
		{3456789012, "user3", []string{"702", "703"}},
	}

	for _, userdata := range userDataList {
		jwtstr, err := getAccessJwt(userdata.userId, grpLst, userdata.namespace)
		require.NoError(t, err)
		ud, err := validateToken(jwtstr)
		require.NoError(t, err)
		require.Equal(t, userdata.namespace, ud.namespace)
		require.Equal(t, userdata.userId, ud.userId)
		require.Equal(t, g, ud.groupIds)
	}
}

func TestGetRefreshJwt(t *testing.T) {
	userDataList := []userData{
		{1234567890, "user1", []string{"701", "702"}},
		{2345678901, "user2", []string{"703", "701"}},
		{3456789012, "user3", []string{"702", "703"}},
	}

	for _, userdata := range userDataList {
		jwtstr, _ := getRefreshJwt(userdata.userId, userdata.namespace)
		ud, err := validateToken(jwtstr)
		require.NoError(t, err)
		require.Equal(t, userdata.namespace, ud.namespace)
		require.Equal(t, userdata.userId, ud.userId)
	}
}

func TestMain(m *testing.M) {
	worker.Config.AclJwtAlg = jwt.SigningMethodHS256
	x.WorkerConfig.AclJwtAlg = jwt.SigningMethodHS256
	x.WorkerConfig.AclPublicKey = x.Sensitive("6ABBAA2014CFF00289D20D20DA296F67")
	worker.Config.AccessJwtTtl = 20 * time.Second
	worker.Config.RefreshJwtTtl = 20 * time.Second
	worker.Config.AclSecretKey = x.Sensitive("6ABBAA2014CFF00289D20D20DA296F67")
	m.Run()
}

// TestExtractUserAndGroupsPrefersThePrincipal is the equivalence proof for the
// fast path. Both routes are run over the same inputs and required to agree,
// rather than the fast path being checked against remembered behavior.
//
// The inputs that must NOT take the fast path are the point of the test. Each one
// is a way the Principal alone is insufficient, and each would be a privilege
// change if admitted.
func TestExtractUserAndGroupsPrefersThePrincipal(t *testing.T) {
	// aclAuthenticator reports no identity at all unless ACL is on, and the fast
	// path is only reachable when it does. Scoped here rather than in TestMain,
	// which other tests in this package read as ACL-off.
	prev := x.WorkerConfig.AclEnabled
	t.Cleanup(func() { x.WorkerConfig.AclEnabled = prev })
	x.WorkerConfig.AclEnabled = true

	expiry := time.Now().Add(30 * time.Minute).Unix()

	// withPrincipal resolves identity onto the context the way the interceptor
	// does, then attaches the token so the slow path stays available.
	withPrincipal := func(token string) context.Context {
		ctx := metadata.NewIncomingContext(context.Background(),
			metadata.Pairs("accessJwt", token))
		return x.WithResolvedIdentity(ctx)
	}

	t.Run("both routes agree on a valid token", func(t *testing.T) {
		for _, want := range []userData{
			{1234567890, "user1", []string{"701", "702"}},
			{2345678901, "user2", []string{"703", "701"}},
			{0, "groot", []string{"guardians"}},
			{7, "no-groups", nil},
		} {
			token := generateJWT(want.namespace, want.userId, want.groupIds, expiry)

			slow, err := validateToken(token)
			require.NoError(t, err)

			fast, err := extractUserAndGroups(withPrincipal(token))
			require.NoError(t, err)

			require.Equal(t, slow, fast, "the fast path disagreed with validateToken")
			require.Equal(t, want.namespace, fast.namespace)
			require.Equal(t, want.userId, fast.userId)
			require.Equal(t, want.groupIds, fast.groupIds)

			// And the fast path must actually have been taken, or this proves
			// nothing about it.
			_, ok := userDataFromPrincipal(withPrincipal(token))
			require.True(t, ok, "expected the fast path to apply")
		}
	})

	t.Run("a token with no namespace claim still falls through and is rejected", func(t *testing.T) {
		// aclAuthenticator ignores the namespace claim, so a Principal exists here.
		// Taking the fast path would accept the request and let the caller keep
		// whatever namespace metadata they sent, because aclTenantResolver tolerates
		// a token it cannot extract one from.
		claims := jwt.MapClaims{"userid": "sneaky", "exp": expiry, "groups": []string{"guardians"}}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, &claims)
		signed, err := token.SignedString(x.MaybeKeyToBytes(worker.Config.AclSecretKey))
		require.NoError(t, err)

		ctx := withPrincipal(signed)
		require.NotNil(t, x.PrincipalFrom(ctx), "precondition: identity resolved")

		_, ok := userDataFromPrincipal(ctx)
		require.False(t, ok, "the fast path must decline a token with no namespace claim")

		_, err = extractUserAndGroups(ctx)
		require.Error(t, err, "the request must still be rejected")
		require.Contains(t, err.Error(), "namespace in claims is not valid")
	})

	t.Run("an external issuer's groups never take the fast path", func(t *testing.T) {
		// The escalation this guards: IsSuperAdmin matches on the group name, so an
		// external issuer asserting `guardians` would otherwise become superadmin.
		ctx := x.WithPrincipal(context.Background(), &x.Principal{
			Issuer:  "https://idp.example/identity",
			Subject: "some-agent",
			Groups:  []string{"guardians"},
			Claims:  map[string]any{"namespace": float64(0)},
			Method:  x.MethodExternalJWT,
		})
		_, ok := userDataFromPrincipal(ctx)
		require.False(t, ok, "only Dgraph's own token speaks for Dgraph's own groups")
	})

	t.Run("no principal falls through to the token", func(t *testing.T) {
		token := generateJWT(42, "user", []string{"701"}, expiry)
		ctx := metadata.NewIncomingContext(context.Background(),
			metadata.Pairs("accessJwt", token))

		_, ok := userDataFromPrincipal(ctx)
		require.False(t, ok)

		ud, err := extractUserAndGroups(ctx)
		require.NoError(t, err)
		require.Equal(t, uint64(42), ud.namespace)
	})

	t.Run("an expired token is rejected on both routes", func(t *testing.T) {
		token := generateJWT(1, "user", []string{"701"}, time.Now().Add(-time.Hour).Unix())

		_, slowErr := validateToken(token)
		require.Error(t, slowErr)

		ctx := withPrincipal(token)
		require.Nil(t, x.PrincipalFrom(ctx),
			"an expired token must not resolve to a Principal")
		_, err := extractUserAndGroups(ctx)
		require.Error(t, err)
	})

	t.Run("no credential at all is rejected", func(t *testing.T) {
		_, err := extractUserAndGroups(x.WithResolvedIdentity(context.Background()))
		require.Error(t, err)
	})
}
