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
	"google.golang.org/grpc/metadata"
)

// oldAttachJWTNamespace is the pre-seam body of AttachJWTNamespace, preserved
// verbatim as the oracle for TestResolveTenantIsBitIdentical. It is the thing
// this stage promises not to change; asserting against a copy rather than
// against remembered behavior is what makes that promise checkable.
func oldAttachJWTNamespace(ctx context.Context) context.Context {
	if !WorkerConfig.AclEnabled {
		return AttachNamespace(ctx, RootNamespace)
	}

	ns, err := ExtractNamespaceFrom(ctx)
	if err == nil {
		ctx = AttachNamespace(ctx, ns)
	}
	return ctx
}

// hmacKey is 32 bytes: the FIPS provider rejects an HMAC key shorter than the
// digest size, and this package's tests run in the FIPS build too.
var hmacKey = []byte("tenancy-test-hmac-key-32-bytes!!")

// tokenWith mints a signed ACL-style token from the given claims.
func tokenWith(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(hmacKey)
	require.NoError(t, err)
	return signed
}

// withACL configures WorkerConfig for HS256 ACL tokens and restores it after the
// test. WorkerConfig is global, so every mutation here has to be undone.
func withACL(t *testing.T, enabled bool) {
	t.Helper()
	prevEnabled, prevAlg, prevKey := WorkerConfig.AclEnabled, WorkerConfig.AclJwtAlg, WorkerConfig.AclPublicKey
	t.Cleanup(func() {
		WorkerConfig.AclEnabled, WorkerConfig.AclJwtAlg, WorkerConfig.AclPublicKey = prevEnabled, prevAlg, prevKey
	})
	WorkerConfig.AclEnabled = enabled
	WorkerConfig.AclJwtAlg = jwt.GetSigningMethod("HS256")
	WorkerConfig.AclPublicKey = hmacKey
}

// accessToken mints an ACL-style access JWT carrying the given namespace claim.
// The claim is a float64 on the wire, matching what getAccessJwt produces.
func accessToken(t *testing.T, ns uint64) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"userid":    "alice",
		"groups":    []string{"dev"},
		"namespace": float64(ns),
		"exp":       float64(1 << 40), // far future
	})
	signed, err := tok.SignedString(hmacKey)
	require.NoError(t, err)
	return signed
}

// nsOf reports the namespace metadata value on ctx, or "" when absent, so the
// two implementations can be compared on the one channel that matters.
func nsOf(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	if v := md.Get("namespace"); len(v) > 0 {
		return v[0]
	}
	return ""
}

// TestResolveTenantIsBitIdentical is the Stage 1 gate. ResolveTenant must agree
// with the pre-seam implementation on every input, so introducing the seam is
// provably behavior-preserving rather than believed to be.
func TestResolveTenantIsBitIdentical(t *testing.T) {
	require.False(t, TenantResolverInstalled(), "no resolver may be installed for this comparison")

	valid := func(t *testing.T) string { return accessToken(t, 5) }

	cases := []struct {
		name       string
		aclEnabled bool
		token      func(*testing.T) string // "" for no token
		preAttach  *uint64                 // namespace already on the context
	}{
		{name: "acl off, no token", aclEnabled: false},
		{name: "acl off, valid token", aclEnabled: false, token: valid},
		{name: "acl off, namespace pre-attached", aclEnabled: false, preAttach: ptr(uint64(9))},
		{name: "acl off, malformed token and pre-attached", aclEnabled: false,
			token: func(*testing.T) string { return "not.a.jwt" }, preAttach: ptr(uint64(9))},

		{name: "acl on, no token", aclEnabled: true},
		{name: "acl on, no token but namespace pre-attached", aclEnabled: true, preAttach: ptr(uint64(9))},
		{name: "acl on, valid token", aclEnabled: true, token: valid},
		{name: "acl on, valid token overrides pre-attached", aclEnabled: true, token: valid, preAttach: ptr(uint64(9))},
		{name: "acl on, malformed token", aclEnabled: true,
			token: func(*testing.T) string { return "not.a.jwt" }},
		{name: "acl on, malformed token with pre-attached", aclEnabled: true,
			token: func(*testing.T) string { return "not.a.jwt" }, preAttach: ptr(uint64(9))},
		{name: "acl on, token signed with the wrong key", aclEnabled: true, token: func(t *testing.T) string {
			tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"namespace": float64(5)})
			s, err := tok.SignedString([]byte("a-different-32-byte-hmac-key!!!!!"))
			require.NoError(t, err)
			return s
		}},
		{name: "acl on, token with no namespace claim", aclEnabled: true, token: func(t *testing.T) string {
			tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"userid": "alice"})
			s, err := tok.SignedString(hmacKey)
			require.NoError(t, err)
			return s
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withACL(t, tc.aclEnabled)

			build := func() context.Context {
				md := metadata.New(nil)
				if tc.token != nil {
					md.Set("accessJwt", tc.token(t))
				}
				ctx := metadata.NewIncomingContext(context.Background(), md)
				if tc.preAttach != nil {
					ctx = AttachNamespace(ctx, *tc.preAttach)
				}
				return ctx
			}

			// Separate contexts so neither implementation observes the other's
			// mutations to the shared metadata map.
			wantCtx := oldAttachJWTNamespace(build())
			gotCtx, err := ResolveTenant(build())

			require.NoError(t, err, "the built-in resolver never returns an error")
			require.Equal(t, nsOf(wantCtx), nsOf(gotCtx), "namespace metadata must match the pre-seam behavior")
		})
	}
}

// TestAttachTrustedTenantIsHonored covers the one input the pre-seam code had no
// concept of. In-process callers that hold no request credential mark their
// context, and ResolveTenant must leave that attribution alone — otherwise a
// fail-closed resolver would break schema bootstrap and the in-process Alter path.
func TestAttachTrustedTenantIsHonored(t *testing.T) {
	withACL(t, true)

	// A trusted context naming namespace 9, with a token for namespace 5 also
	// present: the marker wins, so the resolver cannot override a deliberate
	// in-process attribution.
	md := metadata.Pairs("accessJwt", accessToken(t, 5))
	ctx := AttachTrustedTenant(metadata.NewIncomingContext(context.Background(), md), 9)

	got, err := ResolveTenant(ctx)
	require.NoError(t, err)
	require.Equal(t, "9", nsOf(got))

	// And an installed resolver is not consulted at all for a trusted context.
	t.Cleanup(func() { SetTenantResolver(nil) })
	SetTenantResolver(func(context.Context) (context.Context, error) {
		t.Error("resolver must not run for a trusted context")
		return nil, errors.New("unreachable")
	})
	got, err = ResolveTenant(ctx)
	require.NoError(t, err)
	require.Equal(t, "9", nsOf(got))
}

// TestSetTenantResolver covers installation, delegation, error propagation, and
// restoring the built-in.
func TestSetTenantResolver(t *testing.T) {
	withACL(t, false) // built-in would force RootNamespace; the resolver must win
	t.Cleanup(func() { SetTenantResolver(nil) })

	require.False(t, TenantResolverInstalled())

	SetTenantResolver(func(ctx context.Context) (context.Context, error) {
		return AttachNamespace(ctx, 42), nil
	})
	require.True(t, TenantResolverInstalled())

	got, err := ResolveTenant(metadata.NewIncomingContext(context.Background(), metadata.New(nil)))
	require.NoError(t, err)
	require.Equal(t, "42", nsOf(got), "installed resolver must take precedence over the built-in")

	// An error is propagated rather than swallowed. Every call site takes it and
	// rejects, which is the whole reason ResolveTenant replaced the old
	// context-in/context-out signature.
	sentinel := errors.New("cannot attribute request")
	SetTenantResolver(func(context.Context) (context.Context, error) { return nil, sentinel })
	_, err = ResolveTenant(context.Background())
	require.ErrorIs(t, err, sentinel)

	SetTenantResolver(nil)
	require.False(t, TenantResolverInstalled())
	got, err = ResolveTenant(context.Background())
	require.NoError(t, err)
	require.Equal(t, "0", nsOf(got), "built-in resolver restored: ACL off means root namespace")
}

// TestMultiTenancyEnabled pins the truth table. A stock OSS build reports false,
// which is what keeps the Stage 3 substitutions behavior-preserving there.
func TestMultiTenancyEnabled(t *testing.T) {
	t.Cleanup(func() { SetTenantResolver(nil) })

	withACL(t, false)
	require.False(t, MultiTenancyEnabled(), "stock build: ACL off, no resolver")

	withACL(t, true)
	require.True(t, MultiTenancyEnabled(), "ACL carries the namespace claim")

	withACL(t, false)
	SetTenantResolver(func(ctx context.Context) (context.Context, error) { return ctx, nil })
	require.True(t, MultiTenancyEnabled(), "an installed resolver can name a tenant without ACL")
}

// TestResolveTenantHTTP covers the HTTP surface, which previously swallowed the
// parse error and returned namespace 0. That mattered because these call sites
// choose which namespace's GraphQL schema serves a request — so a malformed
// token silently routed the caller to the root namespace's schema instead of
// being rejected.
func TestResolveTenantHTTP(t *testing.T) {
	req := func(token string) *http.Request {
		r := &http.Request{Header: http.Header{}}
		if token != "" {
			r.Header.Set("X-Dgraph-AccessToken", token)
		}
		return r
	}

	t.Run("acl off resolves to the root namespace", func(t *testing.T) {
		withACL(t, false)
		ns, err := ResolveTenantHTTP(req(""))
		require.NoError(t, err)
		require.Equal(t, RootNamespace, ns)
	})

	t.Run("acl on derives the namespace from the token", func(t *testing.T) {
		withACL(t, true)
		ns, err := ResolveTenantHTTP(req(accessToken(t, 5)))
		require.NoError(t, err)
		require.Equal(t, uint64(5), ns)
	})

	// Fail-open is preserved deliberately — see the ResolveTenantHTTP doc comment.
	// Two attempts at making this reject taught the same lesson twice: /admin
	// forces resolver=0 and only needs this for LazyLoadSchema, health probes and
	// the login mutation cannot carry a token at all, and rejecting them deadlocks
	// the cluster because login is how a token is obtained in the first place.
	t.Run("an unresolvable request falls back to the root namespace", func(t *testing.T) {
		withACL(t, true)
		for name, token := range map[string]string{
			"no token":  "",
			"malformed": "not.a.jwt",
			"signed with wrong key": func() string {
				tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"namespace": float64(5)})
				s, err := tok.SignedString([]byte("a-different-32-byte-hmac-key!!!!!"))
				require.NoError(t, err)
				return s
			}(),
		} {
			t.Run(name, func(t *testing.T) {
				ns, err := ResolveTenantHTTP(req(token))
				require.NoError(t, err)
				require.Equal(t, RootNamespace, ns)
			})
		}
	})

	t.Run("an installed resolver's error propagates", func(t *testing.T) {
		withACL(t, false)
		t.Cleanup(func() { SetTenantResolver(nil) })
		sentinel := errors.New("cannot attribute request")
		SetTenantResolver(func(context.Context) (context.Context, error) { return nil, sentinel })

		_, err := ResolveTenantHTTP(req(""))
		require.ErrorIs(t, err, sentinel)
	})
}

// TestResolveTenantHTTPStrict covers the /graphql-only variant. The pair of
// assertions is the whole point: an unusable token is rejected, an absent one is
// not. Two earlier attempts at this hardening failed by conflating them — the
// first rejected both, which broke login and deadlocked the cluster; the second
// still applied the rule to /admin, where the root namespace is the right answer
// and the test harness's own health check presents a token.
func TestResolveTenantHTTPStrict(t *testing.T) {
	req := func(token string) *http.Request {
		r := &http.Request{Header: http.Header{}}
		if token != "" {
			r.Header.Set("X-Dgraph-AccessToken", token)
		}
		return r
	}

	t.Run("no token resolves to root, never rejects", func(t *testing.T) {
		for _, acl := range []bool{false, true} {
			withACL(t, acl)
			ns, err := ResolveTenantHTTPStrict(req(""))
			require.NoErrorf(t, err, "acl=%v: login and probes carry no token by design", acl)
			require.Equal(t, RootNamespace, ns)
		}
	})

	t.Run("valid token resolves to its namespace", func(t *testing.T) {
		withACL(t, true)
		ns, err := ResolveTenantHTTPStrict(req(accessToken(t, 5)))
		require.NoError(t, err)
		require.Equal(t, uint64(5), ns)
	})

	t.Run("unusable token is rejected", func(t *testing.T) {
		withACL(t, true)
		wrongKey, err := jwt.NewWithClaims(jwt.SigningMethodHS256,
			jwt.MapClaims{"namespace": float64(5), "exp": float64(1 << 40)}).
			SignedString([]byte("a-different-32-byte-hmac-key!!!!!"))
		require.NoError(t, err)

		for name, token := range map[string]string{
			"malformed":             "not.a.jwt",
			"signed with wrong key": wrongKey,
			"expired": tokenWith(t, jwt.MapClaims{
				"userid": "alice", "namespace": float64(5), "exp": float64(1)}),
			"no namespace claim": tokenWith(t, jwt.MapClaims{
				"userid": "alice", "exp": float64(1 << 40)}),
		} {
			t.Run(name, func(t *testing.T) {
				_, err := ResolveTenantHTTPStrict(req(token))
				require.Error(t, err,
					"must not serve the root namespace's schema to a caller whose tenant is unknown")
			})
		}
	})

	// With ACL off there is no credential to verify, so even a token that would
	// otherwise be unusable resolves to the root namespace — the single-tenant
	// answer. Strictness must not invent a failure where tenancy cannot vary.
	t.Run("acl off ignores an unusable token", func(t *testing.T) {
		withACL(t, false)
		ns, err := ResolveTenantHTTPStrict(req("not.a.jwt"))
		require.NoError(t, err)
		require.Equal(t, RootNamespace, ns)
	})

	t.Run("lenient variant still admits what strict rejects", func(t *testing.T) {
		withACL(t, true)
		ns, err := ResolveTenantHTTP(req("not.a.jwt"))
		require.NoError(t, err, "/admin and probes must keep working")
		require.Equal(t, RootNamespace, ns)
	})
}

func ptr[T any](v T) *T { return &v }

// TestResolveTenantNeverReturnsNilContext pins the guard against a resolver that
// fails closed by returning only an error.
//
// The hazard is not hypothetical: the deprecated AttachJWTNamespace shim did
// `ctx, _ = ResolveTenant(ctx)`, so a nil context would have reached every caller
// that shim served and panicked on first use — a crash where a rejection was
// intended. The shim is gone, but the guard belongs on the seam rather than on the
// discipline of each caller.
func TestResolveTenantNeverReturnsNilContext(t *testing.T) {
	t.Cleanup(func() { SetTenantResolver(nil) })

	wantErr := errors.New("no credential; refusing to attribute")
	SetTenantResolver(func(context.Context) (context.Context, error) {
		return nil, wantErr
	})

	base := context.Background()
	got, err := ResolveTenant(base)
	require.ErrorIs(t, err, wantErr, "the resolver's error must survive")
	require.NotNil(t, got, "a nil context must never be handed to a caller")
	// Usable, not merely non-nil.
	require.NoError(t, got.Err())
}

// TestClearIncomingNamespaceClosesTheEntryPointHole is the regression test for a
// privilege escalation introduced when the zero-proxy UID lease path was converted
// onto the seam.
//
// Before the conversion the path derived the namespace from the signed access JWT
// and returned that error on failure. After it, the built-in resolver's
// tolerate-a-bad-token branch left the client's own md["namespace"] in place and
// ExtractNamespace read it back, so a caller with no usable credential leased UIDs
// in whichever tenant they named.
func TestClearIncomingNamespaceClosesTheEntryPointHole(t *testing.T) {
	withACL(t, true)

	// A caller asking for namespace 9 while presenting no access token at all.
	md := metadata.New(map[string]string{"namespace": "9"})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	t.Run("without clearing, the client's namespace survives", func(t *testing.T) {
		rctx, err := ResolveTenant(ctx)
		require.NoError(t, err, "the built-in resolver tolerates a missing token")
		ns, err := ExtractNamespace(rctx)
		require.NoError(t, err)
		require.Equal(t, uint64(9), ns,
			"documents the hazard: the value came from the caller, not a credential")
	})

	t.Run("clearing first turns it into a rejection", func(t *testing.T) {
		rctx, err := ResolveTenant(ClearIncomingNamespace(ctx))
		require.NoError(t, err)
		_, err = ExtractNamespace(rctx)
		require.Error(t, err,
			"an unattributable entry-point request must not resolve to any tenant")
	})

	t.Run("a valid token still resolves, and the claim wins over the client's value", func(t *testing.T) {
		token := accessToken(t, 7)
		withTok := metadata.NewIncomingContext(context.Background(),
			metadata.New(map[string]string{"namespace": "9", "accessJwt": token}))
		rctx, err := ResolveTenant(ClearIncomingNamespace(withTok))
		require.NoError(t, err)
		ns, err := ExtractNamespace(rctx)
		require.NoError(t, err)
		require.Equal(t, uint64(7), ns, "the namespace must come from the signed claim")
	})

	t.Run("clearing leaves other metadata alone", func(t *testing.T) {
		in := metadata.NewIncomingContext(context.Background(),
			metadata.New(map[string]string{"namespace": "9", "accessJwt": "keep-me"}))
		out, ok := metadata.FromIncomingContext(ClearIncomingNamespace(in))
		require.True(t, ok)
		require.Empty(t, out.Get("namespace"))
		require.Equal(t, []string{"keep-me"}, out.Get("accessJwt"))
	})

	t.Run("no metadata at all is not a crash", func(t *testing.T) {
		require.NotNil(t, ClearIncomingNamespace(context.Background()))
	})
}
