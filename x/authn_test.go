/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// oracleUserData mirrors edgraph.userData, and oracleValidateToken mirrors
// edgraph.validateToken's identity extraction verbatim. aclAuthenticator
// duplicates that logic in package x, so the duplicate is held against a copy of
// the original rather than against remembered behavior — the same discipline the
// tenant seam used.
//
// The one deliberate divergence is the namespace claim: validateToken requires it
// and aclAuthenticator ignores it, because tenancy is the resolver's concern now.
// That divergence is asserted explicitly below rather than left implicit.
type oracleUserData struct {
	userId   string
	groupIds []string
}

func oracleValidateToken(jwtStr string) (*oracleUserData, error) {
	claims, err := ParseJWT(jwtStr)
	if err != nil {
		return nil, err
	}
	if exp, err := claims.GetExpirationTime(); err != nil || exp == nil {
		return nil, errors.New("Token is expired")
	}
	userId, ok := claims["userid"].(string)
	if !ok {
		return nil, errors.New("userid in claims is not a string")
	}
	groups, ok := claims["groups"].([]interface{})
	var groupIds []string
	if ok {
		groupIds = make([]string, 0, len(groups))
		for _, group := range groups {
			groupId, ok := group.(string)
			if !ok {
				return nil, errors.New("unable to convert group to string")
			}
			groupIds = append(groupIds, groupId)
		}
	}
	return &oracleUserData{userId: userId, groupIds: groupIds}, nil
}

func ctxWithToken(token string) context.Context {
	md := metadata.New(nil)
	if token != "" {
		md.Set("accessJwt", token)
	}
	return metadata.NewIncomingContext(context.Background(), md)
}

// TestACLAuthenticatorMatchesValidateToken pins that the identity extraction
// moved into x agrees with edgraph.validateToken on every input either accepts
// or rejects.
func TestACLAuthenticatorMatchesValidateToken(t *testing.T) {
	cases := []struct {
		name   string
		claims jwt.MapClaims
	}{
		{"userid and groups", jwt.MapClaims{
			"userid": "alice", "groups": []string{"dev", "ops"},
			"namespace": float64(5), "exp": float64(1 << 40)}},
		{"userid, no groups", jwt.MapClaims{
			"userid": "groot", "namespace": float64(0), "exp": float64(1 << 40)}},
		{"empty groups list", jwt.MapClaims{
			"userid": "bob", "groups": []string{},
			"namespace": float64(0), "exp": float64(1 << 40)}},
		{"guardians member", jwt.MapClaims{
			"userid": "groot", "groups": []string{"guardians"},
			"namespace": float64(0), "exp": float64(1 << 40)}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withACL(t, true)
			tok := tokenWith(t, tc.claims)

			want, wantErr := oracleValidateToken(tok)
			got, gotErr := aclAuthenticator{}.Authenticate(ctxWithToken(tok))

			require.NoError(t, wantErr)
			require.NoError(t, gotErr)
			require.NotNil(t, got)
			require.Equal(t, want.userId, got.Subject, "subject must match validateToken's userId")
			require.Equal(t, want.groupIds, got.Groups, "groups must match validateToken's groupIds")
			require.Equal(t, "acl", got.Method)
			require.Equal(t, "dgraph-acl", got.Issuer)
		})
	}
}

// TestACLAuthenticatorRejectsBadCredentials covers the inputs that must produce
// an error — a credential was presented and it did not verify. Each is also
// rejected by the oracle, so the two agree on failure as well as success.
func TestACLAuthenticatorRejectsBadCredentials(t *testing.T) {
	withACL(t, true)

	expired := tokenWith(t, jwt.MapClaims{
		"userid": "alice", "namespace": float64(0), "exp": float64(1)}) // 1970
	noExp := tokenWith(t, jwt.MapClaims{"userid": "alice", "namespace": float64(0)})
	wrongKey, err := jwt.NewWithClaims(jwt.SigningMethodHS256,
		jwt.MapClaims{"userid": "alice", "exp": float64(1 << 40)}).
		SignedString([]byte("a-different-32-byte-hmac-key!!!!!"))
	require.NoError(t, err)

	for name, tok := range map[string]string{
		"expired":                expired,
		"no exp claim":           noExp,
		"signed with wrong key":  wrongKey,
		"malformed":              "not.a.jwt",
		"userid is not a string": tokenWith(t, jwt.MapClaims{"userid": 42, "exp": float64(1 << 40)}),
		"group is not a string": tokenWith(t, jwt.MapClaims{
			"userid": "alice", "groups": []any{"dev", 7}, "exp": float64(1 << 40)}),
	} {
		t.Run(name, func(t *testing.T) {
			_, oracleErr := oracleValidateToken(tok)
			require.Error(t, oracleErr, "oracle must also reject this, or the comparison is meaningless")

			p, err := aclAuthenticator{}.Authenticate(ctxWithToken(tok))
			require.Error(t, err)
			require.Nil(t, p)
		})
	}
}

// TestACLAuthenticatorNoCredential covers the case that must NOT be an error.
// Login, health checks, and CheckVersion present nothing, and returning an error
// for them is what makes a rejecting interceptor deadlock a cluster.
func TestACLAuthenticatorNoCredential(t *testing.T) {
	t.Run("acl on, no token", func(t *testing.T) {
		withACL(t, true)
		p, err := aclAuthenticator{}.Authenticate(ctxWithToken(""))
		require.NoError(t, err, "an absent credential is not a verification failure")
		require.Nil(t, p)
	})

	t.Run("acl on, no metadata at all", func(t *testing.T) {
		withACL(t, true)
		p, err := aclAuthenticator{}.Authenticate(context.Background())
		require.NoError(t, err)
		require.Nil(t, p)
	})

	t.Run("acl off", func(t *testing.T) {
		withACL(t, false)
		p, err := aclAuthenticator{}.Authenticate(ctxWithToken(tokenWith(t, jwt.MapClaims{
			"userid": "alice", "exp": float64(1 << 40)})))
		require.NoError(t, err)
		require.Nil(t, p, "no ACL configured means no identity to report")
	})
}

// TestACLAuthenticatorIgnoresNamespaceClaim pins the one deliberate divergence
// from validateToken. Tenancy is the resolver's concern, so a token with no
// namespace claim still authenticates — where validateToken would reject it.
func TestACLAuthenticatorIgnoresNamespaceClaim(t *testing.T) {
	withACL(t, true)
	tok := tokenWith(t, jwt.MapClaims{"userid": "alice", "exp": float64(1 << 40)}) // no namespace

	_, oracleErr := oracleValidateToken(tok)
	require.NoError(t, oracleErr, "the oracle checks identity only; namespace is checked separately")

	p, err := aclAuthenticator{}.Authenticate(ctxWithToken(tok))
	require.NoError(t, err)
	require.NotNil(t, p)
	require.Equal(t, "alice", p.Subject)
}

// TestWithResolvedIdentityNeverRejects is the load-bearing property. Every input
// that could fail must yield a usable context, because the interceptor has no way
// to signal rejection and must not acquire one.
func TestWithResolvedIdentityNeverRejects(t *testing.T) {
	withACL(t, true)

	for name, tok := range map[string]string{
		"no token":               "",
		"malformed":              "not.a.jwt",
		"expired":                tokenWith(t, jwt.MapClaims{"userid": "alice", "exp": float64(1)}),
		"userid is not a string": tokenWith(t, jwt.MapClaims{"userid": 42, "exp": float64(1 << 40)}),
	} {
		t.Run(name, func(t *testing.T) {
			ctx := WithResolvedIdentity(ctxWithToken(tok))
			require.NotNil(t, ctx)
			require.Nil(t, PrincipalFrom(ctx), "a failed or absent credential attaches no Principal")
		})
	}

	t.Run("a valid token attaches the principal", func(t *testing.T) {
		ctx := WithResolvedIdentity(ctxWithToken(tokenWith(t, jwt.MapClaims{
			"userid": "alice", "groups": []string{"dev"}, "exp": float64(1 << 40)})))
		p := PrincipalFrom(ctx)
		require.NotNil(t, p)
		require.Equal(t, "alice", p.Subject)
		require.Equal(t, []string{"dev"}, p.Groups)
	})

	t.Run("an authenticator that errors does not reject", func(t *testing.T) {
		t.Cleanup(func() { SetAuthenticator(nil) })
		SetAuthenticator(erroringAuthenticator{})
		ctx := WithResolvedIdentity(context.Background())
		require.NotNil(t, ctx)
		require.Nil(t, PrincipalFrom(ctx))
	})
}

type erroringAuthenticator struct{}

func (erroringAuthenticator) Name() string { return "erroring" }
func (erroringAuthenticator) Authenticate(context.Context) (*Principal, error) {
	return nil, errors.New("issuer unreachable")
}

// TestSetAuthenticator covers installation and restoration of the built-in.
func TestSetAuthenticator(t *testing.T) {
	t.Cleanup(func() { SetAuthenticator(nil) })
	withACL(t, true)

	SetAuthenticator(fixedAuthenticator{subject: "service-account"})
	ctx := WithResolvedIdentity(context.Background())
	p := PrincipalFrom(ctx)
	require.NotNil(t, p)
	require.Equal(t, "service-account", p.Subject)
	require.Equal(t, "fixed", p.Method)

	// nil restores the built-in, which reports nothing without a credential.
	SetAuthenticator(nil)
	require.Nil(t, PrincipalFrom(WithResolvedIdentity(context.Background())))
}

type fixedAuthenticator struct{ subject string }

func (fixedAuthenticator) Name() string { return "fixed" }
func (f fixedAuthenticator) Authenticate(context.Context) (*Principal, error) {
	return &Principal{Subject: f.subject, Method: "fixed"}, nil
}

// TestIdentityInterceptors confirms both interceptors attach the identity and
// pass the call through, including that the streaming one overrides the stream's
// context rather than silently dropping the principal.
func TestIdentityInterceptors(t *testing.T) {
	withACL(t, true)
	tok := tokenWith(t, jwt.MapClaims{"userid": "alice", "exp": float64(1 << 40)})

	t.Run("unary", func(t *testing.T) {
		var seen *Principal
		intc := IdentityUnaryInterceptor()
		_, err := intc(ctxWithToken(tok), "req", &grpc.UnaryServerInfo{FullMethod: "/api.Dgraph/Query"},
			func(ctx context.Context, _ any) (any, error) {
				seen = PrincipalFrom(ctx)
				return nil, nil
			})
		require.NoError(t, err)
		require.NotNil(t, seen)
		require.Equal(t, "alice", seen.Subject)
	})

	t.Run("stream", func(t *testing.T) {
		var seen *Principal
		intc := IdentityStreamInterceptor()
		err := intc(nil, fakeServerStream{ctx: ctxWithToken(tok)},
			&grpc.StreamServerInfo{FullMethod: "/api.Dgraph/StreamExtSnapshot"},
			func(_ any, ss grpc.ServerStream) error {
				seen = PrincipalFrom(ss.Context())
				return nil
			})
		require.NoError(t, err)
		require.NotNil(t, seen, "the stream's context must carry the principal")
		require.Equal(t, "alice", seen.Subject)
	})
}

type fakeServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s fakeServerStream) Context() context.Context { return s.ctx }

// TestAttachRequestIdentity covers the HTTP-edge helper that replaces the
// duplicated prelude, including that it preserves the metadata the individual
// Attach* calls used to set.
func TestAttachRequestIdentity(t *testing.T) {
	withACL(t, true)
	tok := tokenWith(t, jwt.MapClaims{"userid": "alice", "exp": float64(1 << 40)})

	r := &http.Request{Header: http.Header{}, RemoteAddr: "10.0.0.7:4242"}
	r.Header.Set("X-Dgraph-AccessToken", tok)
	r.Header.Set("X-Dgraph-AuthToken", "poor-mans")

	ctx := AttachRequestIdentity(context.Background(), r)

	p := PrincipalFrom(ctx)
	require.NotNil(t, p)
	require.Equal(t, "alice", p.Subject)

	md, ok := metadata.FromIncomingContext(ctx)
	require.True(t, ok)
	require.Equal(t, []string{tok}, md.Get("accessJwt"))
	require.Equal(t, []string{"poor-mans"}, md.Get("auth-token"))
}
